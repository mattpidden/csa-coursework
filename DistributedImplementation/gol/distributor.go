package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

var requestsChannel chan CellsFlippedRequest //YOUR DAMN TESTS LEFT ME WITH NO CHOICE OK

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

type SingleThreadExecutionResponse struct {
	GolWorld [][]uint8
	Turns    int
}

type SingleThreadExecutionRequest struct {
	GolWorld    [][]uint8
	Turns       int
	ImageHeight int
	ImageWidth  int
	Threads     int
}

type GetCellsAliveResponse struct {
	Turns      int
	CellsAlive int
}

type GetCellsAliveRequest struct {
	InitialCellsAlive int
	ImageHeight       int
	ImageWidth        int
}

// HaloExchangeRequest HALO-EXCHANGE
type HaloExchangeRequest struct {
	Section [][]uint8
	Turns   int
}

// HaloExchangeResponse HALO-EXCHANGE
type HaloExchangeResponse struct {
	Section [][]uint8
}

// InitialiseConnectionRequest HALO-EXCHANGE
type InitialiseConnectionRequest struct {
	AboveIP       string
	BelowIP       string
	DistributorIP string
	WorkerID      int
}

// InitialiseConnectionResponse HALO-EXCHANGE
type InitialiseConnectionResponse struct {
	UpperConnection bool
	LowerConnection bool
}
type CellsFlippedRequest struct {
	CellsFlipped []util.Cell
	WorkerID     int
	Turn         int
}
type CellsFlippedResponse struct {
}

type Receiver struct {
	CellsFlippedChan chan<- CellsFlippedRequest
}

func (g *Receiver) CellsFlippedMethod(req CellsFlippedRequest, res *CellsFlippedResponse) error {
	fmt.Printf("CellsFlippedMethod(): req.Turn: %v , WorkerID: %v\n", req.Turn, req.WorkerID)
	(*g).CellsFlippedChan <- req
	return nil
}

func initializeGolMatrix(p Params, c distributorChannels) [][]uint8 {
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioCommand <- ioInput
	c.ioFilename <- filename

	golWorld := make([][]uint8, p.ImageHeight)
	for y := range golWorld {
		golWorld[y] = make([]uint8, p.ImageWidth)
	}

	//Loop through matrix retrieving initial value for each cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			//Receive data from channel and assign to 2d slice
			b := <-c.ioInput
			golWorld[y][x] = b
			if b == 255 {
				//Let the event component know which cells start alive
				c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: x, Y: y}}
			}
		}
	}
	return golWorld
}

func startDistServer(port string, channel chan<- CellsFlippedRequest, shutDownChan, shutDownCompleteChan chan bool) {
	//SETUP DISTRIBUTOR SERVER
	fmt.Println("startDistServer():")
	listener, err := net.Listen("tcp", ":"+port)
	handleError(err)

	receiver := Receiver{channel}
	err = rpc.Register(&receiver)
	handleError(err)

	connChan := make(chan net.Conn)
	connections := make([]net.Conn, 0)

	//All connections will send their CellsFlippedRequest along the same channel
	go func(connChan chan net.Conn) {
		for {
			conn, err := listener.Accept()
			handleError(err)
			if err != nil {
				connChan <- conn
			}

		}
	}(connChan)

	for {
		select {
		case <-shutDownChan:
			//Shut down listener
			err := listener.Close()
			handleError(err)
			//Close all net.Conn connections
			for _, conn := range connections {
				err = conn.Close()
				handleError(err)
			}
			fmt.Println("startDistServer(): DistServer shutting down")
			shutDownCompleteChan <- true

			return
		case conn := <-connChan:
			connections = append(connections, conn)
			go rpc.ServeConn(conn)
		}
	}

}

func handleRequests(reqChan <-chan CellsFlippedRequest, sectionHeight int, workers int, c distributorChannels, shutdownChan chan bool) {
	fmt.Println("handleRequests():")
	var req CellsFlippedRequest
	var turn int
	for {
		for i := 0; i < workers; i++ {
			select {
			case REQ := <-reqChan:
				req = REQ
			case <-shutdownChan:
				fmt.Println("handleRequests(): Shutting down")
				return
			}
			turn = req.Turn
			cellsFlipped := req.CellsFlipped
			for _, cell := range cellsFlipped {
				cell.Y += req.WorkerID * sectionHeight
				c.events <- CellFlipped{CompletedTurns: req.Turn, Cell: cell}
			}
		}
		c.events <- TurnComplete{CompletedTurns: turn}
	}
}

func handleError(err error) {
	if err != nil {
		fmt.Println(err)
	}
}
func initialiseWorkerConnections(ports []string, localHost string, distPort string, servers []*rpc.Client) {
	fmt.Println("initialiseWorkerConnections():")
	var err error
	for i, port := range ports {
		servers[i], err = rpc.Dial("tcp", localHost+port)
		handleError(err)
	}

	//Send InitialiseConnection requests to all workers
	for i, server := range servers {
		uIndex := i - 1 //upper index
		if uIndex == -1 {
			uIndex += len(servers)
		}
		lIndex := (i + 1) % len(servers) //lower index

		request := InitialiseConnectionRequest{
			AboveIP:       localHost + ports[uIndex],
			BelowIP:       localHost + ports[lIndex],
			DistributorIP: localHost + distPort,
			WorkerID:      i,
		}

		response := new(InitialiseConnectionResponse)

		//Blocking call
		fmt.Println("HaloExchange.InitialiseConnection call made")
		err := server.Call("HaloExchange.InitialiseConnection", request, &response)
		handleError(err)
		fmt.Println("Rpc response received")

		if response.LowerConnection && response.UpperConnection {
			fmt.Printf("Error occured in worker: %v when attempting to connect to above and below worker\n", i)
			os.Exit(-1)
		}
	}
}

func beginGol(servers []*rpc.Client, sectionHeight int, golWorld [][]uint8, p Params, workers int) [][]uint8 {
	fmt.Println("beginGol():")
	wg := sync.WaitGroup{}
	y := 0
	completeSections := make([][][]uint8, workers)
	for i := 0; i < len(servers); i++ {
		section := make([][]uint8, sectionHeight)
		copy(section, golWorld[y:y+sectionHeight]) //Shallow copy
		y += sectionHeight
		request := HaloExchangeRequest{Section: section, Turns: p.Turns}
		response := new(HaloExchangeResponse)
		wg.Add(1)
		go func(I int, req HaloExchangeRequest, res *HaloExchangeResponse) {
			fmt.Println("Making HaloExchange.Simulate rpc call")

			err := servers[I].Call("HaloExchange.Simulate", req, res)
			fmt.Println("HaloExchange.Simulate rpc call response received")
			handleError(err)

			completeSections[I] = res.Section
			wg.Done()
		}(i, request, response)
	}
	wg.Wait()

	var newGolWorld [][]uint8
	for i := 0; i < len(completeSections); i++ {
		newGolWorld = append(newGolWorld, completeSections[i]...)
	}

	fmt.Println("All workers have finished computation")
	return newGolWorld
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	fmt.Println("Distributor(): ")
	//VARIABLE-INIT
	//Hard-coded configuration for 4 workers running on local system
	distPort := "8040"
	workers := 4
	ports := make([]string, workers)
	ports[0] = "8050"
	ports[1] = "8060"
	ports[2] = "8070"
	ports[3] = "8080"
	localHost := "127.0.0.1:"

	servers := make([]*rpc.Client, workers)
	sectionHeight := p.ImageHeight / workers
	reqChan := make(chan CellsFlippedRequest)
	var golWorld [][]uint8

	//Initialize golWorld
	golWorld = initializeGolMatrix(p, c)

	//Start distributor server
	shutDownChan := make(chan bool)
	shutDownCompleteChan := make(chan bool)
	go startDistServer(distPort, reqChan, shutDownChan, shutDownCompleteChan)

	//Start go routine to handle CellsFlippedRequests
	shutDownChan2 := make(chan bool)
	go handleRequests(reqChan, sectionHeight, workers, c, shutDownChan2)

	//Make connection for every worker
	initialiseWorkerConnections(ports, localHost, distPort, servers)

	//Begin gol computation
	finishedWorld := beginGol(servers, sectionHeight, golWorld, p, workers)
	finishedWorldImmutable := makeImmutableMatrix(finishedWorld)

	aliveCells := FinalTurnComplete{CompletedTurns: p.Turns, Alive: calculateAliveCells(p, finishedWorldImmutable)}
	c.events <- aliveCells

	//Cleaning up
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}

	close(c.events)

	shutDownChan <- true
	shutDownChan2 <- true
	<-shutDownCompleteChan

}

// makeImmutableMatrix takes an existing 2D matrix and wraps it in a getter closure.
func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

//Input: p of type Params containing data about the world
//Input: world of type [][]uint8 containing the gol world data
//Returns: slice containing elements of type util.Cell, of all alive cells
func calculateAliveCells(p Params, data func(y, x int) uint8) []util.Cell {
	var aliveCells []util.Cell
	//Loops through entire GoL world
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			//If cell is alive, create cell and append to slice
			if data(i, j) == 255 {
				newCell := util.Cell{
					X: j,
					Y: i,
				}
				aliveCells = append(aliveCells, newCell)
			}
		}
	}
	return aliveCells
}
