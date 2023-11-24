package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"sync"
	"time"
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

func startDistServer(port string, channel chan<- CellsFlippedRequest) {
	//SETUP DISTRIBUTOR SERVER
	listener, _ := net.Listen("tcp", ":"+port)
	receiver := Receiver{channel}
	rpc.Register(&receiver)

	//All connections will send their CellsFlippedRequest along the same channel
	for {
		conn, err := listener.Accept()
		handleError(err)
		go rpc.ServeConn(conn)
	}
}

//WILL NEED IMPROVEMENT TO WORK WITH INCONVENIENT p.threads (Assumes all sections are same height)
func handleRequests(reqChan <-chan CellsFlippedRequest, sectionHeight int, p Params, c distributorChannels) {
	for {
		turn := 0
		for i := 0; i < p.Threads; i++ {
			req := <-reqChan
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
		server.Call("HaloExchange.InitialiseConnection", request, &response)
		fmt.Println("Rpc response received")

		if response.LowerConnection && response.UpperConnection {
			fmt.Printf("Error occured in worker: %v when attempting to connect to above and below worker\n", i)
			os.Exit(-1)
		}
	}
}

func beginGol(servers []*rpc.Client, sectionHeight int, golWorld [][]uint8, p Params) {
	fmt.Println("beginGol():")
	wg := sync.WaitGroup{}
	y := 0
	completeSections := make([][][]uint8, p.Threads)
	for i := 0; i < len(servers); i++ {
		section := make([][]uint8, sectionHeight)
		copy(section, golWorld[y:y+sectionHeight]) //Shallow copy
		y += sectionHeight
		request := HaloExchangeRequest{Section: section, Turns: p.Turns}
		response := new(HaloExchangeResponse)
		wg.Add(1)
		go func(I int, req HaloExchangeRequest, res *HaloExchangeResponse) {
			fmt.Printf("i: %v\n", I)
			fmt.Println("Making HaloExchange.Simulate rpc call")
			servers[I].Call("HaloExchange.Simulate", req, res)
			fmt.Println("HaloExchange.Simulate rpc call response received")
			completeSections[I] = res.Section
			wg.Done()
		}(i, request, response)
	}
	wg.Wait()
	fmt.Println("All workers have finished computation")

}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	fmt.Println("Distributor(): ")
	//VARIABLE-INIT
	ports := make([]string, p.Threads)
	servers := make([]*rpc.Client, p.Threads)
	sectionHeight := p.ImageHeight / p.Threads
	reqChan := make(chan CellsFlippedRequest)
	var golWorld [][]uint8

	//Configuration for running 4 workers on local system
	distPort := "8040"
	ports[0] = "8050"
	ports[1] = "8060"
	ports[2] = "8070"
	ports[3] = "8080"
	localHost := "127.0.0.1:"

	//Start distributor server
	golWorld = initializeGolMatrix(p, c)

	go startDistServer(distPort, reqChan)

	//Start go routine to handle CellsFlippedRequests
	go handleRequests(reqChan, sectionHeight, p, c)

	//Make connection for every worker
	initialiseWorkerConnections(ports, localHost, distPort, servers)

	//Begin gol computation
	beginGol(servers, sectionHeight, golWorld, p)

	/*y := 0
	//Split initial world into sections of equal height and send sections to workers
	wg := sync.WaitGroup{}
	completeSections := make([][][]uint8, p.Threads)
	go func() {
		for i := 0; i < len(servers); i++ {
			section := make([][]uint8, sectionHeight)
			copy(section, golWorld[y:y+sectionHeight]) //Shallow copy
			y += sectionHeight
			request := HaloExchangeRequest{Section: section, Turns: p.Turns}
			response := new(HaloExchangeResponse)
			wg.Add(1)
			go func(I int, req HaloExchangeRequest, res *HaloExchangeResponse) {
				fmt.Printf("i: %v\n", I)
				fmt.Println("Making HaloExchange.Simulate rpc call")
				servers[I].Call("HaloExchange.Simulate", req, res)
				fmt.Println("HaloExchange.Simulate rpc call response received")
				completeSections[I] = res.Section
				wg.Done()
			}(i, request, response)
		}
		wg.Wait()
		fmt.Println("All final sections received")

		completeWorld := make([][]uint8, p.ImageHeight)
		for y := 0; y < len(completeWorld); y++ {
			completeWorld[y] = make([]uint8, p.ImageWidth)
		}
		y = 0
		for _, section := range completeSections {
			for _, row := range section {
				completeWorld[y] = row
				y++
			}
		}
		//Output completeWorld as pgm file
		c.ioCommand <- ioOutput
		c.ioFilename <- "completeWorld"
		for _, row := range completeWorld {
			for _, val := range row {
				c.ioOutput <- val
			}
		}
	}()*/

}

// makeImmutableMatrix takes an existing 2D matrix and wraps it in a getter closure.
func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

//Go routine used to send a notification every time 2 seconds has passed

func timer(timesUpChan chan int) {
	for {
		timesUpChan <- 1
		time.Sleep(time.Second * 2)

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
