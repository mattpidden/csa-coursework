package gol

import (
	"fmt"
	"log"
	"net/rpc"
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

type BeginGolReq struct {
	World [][]uint8
	Turns int
}

type BeginGolRes struct {
	FinishedWorld  [][]uint8
	CompletedTurns int
}

type GetSnapShotRequest struct {
}
type GetSnapShotResponse struct {
	matrix [][]uint8
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
	c.events <- TurnComplete{CompletedTurns: 0}
	return golWorld
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	fmt.Println("Distributor(): ")
	var golWorld [][]uint8

	//Get image data from io
	golWorld = initializeGolMatrix(p, c)
	shutDownChan := make(chan bool)

	//Make connection with broker
	brokerIP := "54.175.85.139:8040"
	broker, err := rpc.Dial("tcp", brokerIP)
	handleError(err, "rpc.Dial")

	//Make rpc call to broker
	req := BeginGolReq{
		World: golWorld,
		Turns: p.Turns,
	}
	res := BeginGolRes{}

	//Begin simulation
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		call1 := &rpc.Call{
			ServiceMethod: "Broker.BeginSimulation",
			Args:          req,
			Reply:         &res,
			Done:          make(chan *rpc.Call, 1),
		}
		call1 = broker.Go(call1.ServiceMethod, call1.Args, call1.Reply, call1.Done) //Blocking rpc call
		<-call1.Done
		handleError(call1.Error, "Broker.BeginSimulation rpc call")
		shutDownChan <- true //ShutDown Graphics
		wg.Done()
	}()

	//Start Snapshot Graphics requests
	wg.Add(1)
	go Graphics(c, shutDownChan, golWorld, &wg)

	wg.Wait()

	//Get finished world
	golWorld = res.FinishedWorld

	//Clean up before next distributor call
	fmt.Println("Beginning clean up...")

	//Report FinalTurnComplete
	aliveCells := calculateAliveCells(p, makeImmutableMatrix(golWorld))
	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}
	close(c.events)

	err = broker.Close()
	handleError(err, "broker.Close()")
	fmt.Println("Clean up done...")
}

func Graphics(c distributorChannels, shutDownChan chan bool, world [][]uint8, wg *sync.WaitGroup) {
	fmt.Println("Graphics():")

	ticker := time.NewTicker(1 * time.Second)
	/*conn, err := net.Dial("tcp", "54.175.85.139:8040")
	brokerG := rpc.NewClient(conn)
	handleError(err, "rpc.Dial 2")

	b

	//Make copy such that no underlying slices are shared
	currentWorld := make([][]uint8, len(world))
	for y := 0; y < len(world); y++ {
		currentWorld[0] = make([]uint8, len(world[0]))
		copy(currentWorld[0], world[0])
	}

	turn := 0
	*/
	broker, err := rpc.Dial("tcp", "54.175.85.139:8040")
	handleError(err, "rpc.Dial")
	req := GetSnapShotRequest{}
	res := GetSnapShotResponse{}
	err = broker.Call("Broker.GetSnapshot", req, &res)
	handleError(err, "Broker.GetSnapshot")
	for {
		select {
		case <-ticker.C:
			fmt.Println("Sending GetSnapShotRequest()")
			/*turn++

			//send getSnapshotRequest
			req := GetSnapShotRequest{}
			res := GetSnapShotResponse{}

			call2 := rpc.Call{
				ServiceMethod: "Broker.GetSnapshot",
				Args:          req,
				Reply:         res,
				Done:          make(chan *rpc.Call, 1),
			}

			//Blocking
			brokerG.Go(call2.ServiceMethod, call2.Args, &call2.Reply, call2.Done)
			<-call2.Done

			//Needs removing
			channel := make(chan bool)
			<-channel

			handleError(call2.Error, "Broker.GetSnapshot rpc call")

			//Determine what cells to flip
			for y, row := range currentWorld {
				for x, val := range row {
					if val != res.matrix[y][x] {
						c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: x, Y: y}}
					}
				}
			}
			c.events <- TurnComplete{CompletedTurns: turn}
			currentWorld = res.matrix*/

		case <-shutDownChan:
			(*wg).Done()
			return
		}
	}
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

func handleError(err error, msg string) {
	if err != nil {
		log.Fatalf("%v : %v \n", msg, err)
	}
}
