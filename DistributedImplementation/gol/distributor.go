package gol

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

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
	Benchmarking bool
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

func (g *Receiver) CellsFlippedMethod(req CellsFlippedRequest, res *CellsFlippedResponse) error {
	fmt.Printf("CellsFlippedMethod(): req.Turn: %v , WorkerID: %v\n", req.Turn, req.WorkerID)
	(*g).CellsFlippedChan <- req
	return nil
}

func startDistServer(server *rpc.Server, DistributorIP string, shutDownDist chan bool) {
	//Local channels
	connChan := make(chan net.Conn)
	wg := sync.WaitGroup{}

	//SETUP DISTRIBUTOR SERVER
	listener, err := net.Listen("tcp", DistributorIP)
	handleError(err)

	ctx, cancel := context.WithCancel(context.Background())

	//All connections will send their CellsFlippedRequest along the same channel
	go func(connChan chan<- net.Conn, ctx context.Context) {
		for {
			conn, err := listener.Accept() //Blocking call

			if err != nil {
				select {
				case <-ctx.Done():
					fmt.Println("Error due to ctx cancellation")
					fmt.Println("Shutting down cleanly...")
				default:
					log.Fatal(err)
				}
			}
			connChan <- conn
		}
	}(connChan, ctx)

	for {
		select {
		case conn := <-connChan:
			wg.Add(1)
			go func() {
				server.ServeConn(conn)
				wg.Done()
			}()
		case <-shutDownDist:
			cancel()
			//Closes listener such that no new connections are received
			err := listener.Close()
			handleError(err)
			//Waits for all rpc.ServeConn() to finish
			wg.Wait()
			//Let Distributor() know that distServer is shutdown
			shutDownDist <- true
			return
		}
	}
}

func handleRequests(reqChan <-chan CellsFlippedRequest, sectionHeight int, c distributorChannels, workers int, shutDownReqHandler chan bool) {
	var req CellsFlippedRequest
	for {
		turn := 0
		for i := 0; i < workers; i++ {

			select {
			case req = <-reqChan:
			case <-shutDownReqHandler:
				fmt.Println("Handle requests shutting down...")
				return
			}

			fmt.Println("handleRequest(): request received from reqChan")

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
		log.Fatal(err)
	}
}
func initialiseWorkerConnections(IPAddresses []string, DistributorIP string, servers []*rpc.Client, b bool) {
	fmt.Println("initialiseWorkerConnections():")

	var err error
	for i, IPAddress := range IPAddresses {
		fmt.Printf("initialiseWorkerConnections(): IP: %v \n", IPAddress)
		servers[i], err = rpc.Dial("tcp", IPAddress)
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
			AboveIP:       IPAddresses[uIndex],
			BelowIP:       IPAddresses[lIndex],
			DistributorIP: DistributorIP,
			WorkerID:      i,
			Benchmarking:  b,
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
	var newGolWorld [][]uint8

	completeSections := make([][][]uint8, workers)
	for i := 0; i < len(servers); i++ {
		section := make([][]uint8, sectionHeight)
		copy(section, (golWorld)[y:y+sectionHeight]) //Shallow copy
		y += sectionHeight

		request := HaloExchangeRequest{Section: section, Turns: p.Turns}
		response := new(HaloExchangeResponse)

		wg.Add(1)
		go func(I int, req HaloExchangeRequest, res *HaloExchangeResponse) {
			fmt.Println("Making HaloExchange.Simulate rpc call")
			err := servers[I].Call("HaloExchange.Simulate", req, res)
			handleError(err)
			fmt.Println("HaloExchange.Simulate rpc call response received")
			completeSections[I] = res.Section
			wg.Done()
		}(i, request, response)
	}
	wg.Wait()
	fmt.Println("All workers have finished computation")

	for _, section := range completeSections {
		newGolWorld = append(newGolWorld, section...)
	}

	return newGolWorld
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	fmt.Println("Distributor(): ")

	//VARIABLE-INIT
	workers := 4 //Hardcoding no. workers to 4

	//BENCHMARKING
	benchmarking := false

	IPAddresses := make([]string, workers)
	servers := make([]*rpc.Client, workers)
	sectionHeight := p.ImageHeight / workers
	reqChan := make(chan CellsFlippedRequest)
	var golWorld [][]uint8

	server := rpc.NewServer()
	receiver := Receiver{reqChan}
	err := server.Register(&receiver)
	handleError(err)

	//Configuration for running 4 workers on local system
	IPAddresses[0] = "52.54.247.234:8040"
	IPAddresses[1] = "100.24.91.201:8040"
	IPAddresses[2] = "3.225.79.228:8040"
	IPAddresses[3] = "54.86.16.188:8040"
	DistributorIP := "172.23.182.92:8040"

	//Receive initial gol matrix from io
	golWorld = initializeGolMatrix(p, c)

	//Start distributor server
	shutDownDist := make(chan bool)
	go startDistServer(server, DistributorIP, shutDownDist)

	//Start go routine to handle CellsFlippedRequests
	shutDownReqHandler := make(chan bool)
	go handleRequests(reqChan, sectionHeight, c, workers, shutDownReqHandler)

	//Make connection for every worker
	initialiseWorkerConnections(IPAddresses, DistributorIP, servers, benchmarking)

	//Begin gol computation
	golWorld = beginGol(servers, sectionHeight, golWorld, p, workers)

	//Clean up before next distributor call
	//Close all connections with workers
	fmt.Println("Beginning clean up...")
	for _, server := range servers {
		err := server.Close()
		handleError(err)
	}
	//Stop distServer listener
	shutDownDist <- true
	<-shutDownDist

	//Stop handleRequest go routine
	shutDownReqHandler <- true

	aliveCells := calculateAliveCells(p, makeImmutableMatrix(golWorld))
	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}
	close(c.events)
	fmt.Println("Clean up done...")

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

