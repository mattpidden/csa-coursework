package gol

import (
	"net/rpc"
	"strconv"
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
	Turns int
}

type SingleThreadExecutionRequest struct {
	GolWorld    [][]uint8
	Turns       int
	ImageHeight int
	ImageWidth  int
	Threads     int
}

type GetCellsAliveResponse struct {
	Turns int
	CellsAlive int
}

type GetCellsAliveRequest struct {
	InitialCellsAlive int
	ImageHeight int
	ImageWidth int
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	//Send command to IO, asking to run readPgmImage function
	c.ioCommand <- 1

	//Construct filename from image height and width
	//Send filename to IO, allowing readPgmImage function to process input of image
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename

	//Create a 2D slice to store the world.
	golWorld := make([][]uint8, p.ImageHeight)
	for y := range golWorld {
		golWorld[y] = make([]uint8, p.ImageWidth)
	}

	//Loop through 2d slice initializing each cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			//Receive data from channel and assign to 2d slice
			b := <- c.ioInput
			golWorld[y][x] = b
			if b == 255 {
				//Let the event component know which cells start alive
				c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: x, Y: y}}
			}
		}
	}

	//Take input of server:port
	//serveradd := "3.80.78.238:8030"
	serveradd := "127.0.0.1:8030"
	server, _ := rpc.Dial("tcp", serveradd)
	defer server.Close()

	//CALL SINGLE THREAD EXECUTION
	//Create request and response
	request := SingleThreadExecutionRequest{
		GolWorld:    golWorld,
		Turns:       p.Turns,
		ImageHeight: p.ImageHeight,
		ImageWidth: p.ImageWidth,
		Threads:     p.Threads,
	}
	response := new(SingleThreadExecutionResponse)

	//call server (blocking call) in gorountine with channel to indicate once done
	golWorldProcessed := make(chan bool)
	go func() {
		server.Call("GoLOperations.SingleThreadExecution", request, response)
		golWorldProcessed <- true
	}()

	//Setting up chan for 2 second updates
	timesUp := make(chan int)
	//Running go routine to be flagging for updates every 2 seconds
	go timer(timesUp)

	doneProcessing := false
	for !doneProcessing {
		select {
		case <-golWorldProcessed:
			doneProcessing = true
		case <-timesUp:
			//make RPC call
			request := GetCellsAliveRequest{len(calculateAliveCells(p, makeImmutableMatrix(golWorld))), p.ImageHeight, p.ImageWidth}
			response := new(GetCellsAliveResponse)
			server.Call("GoLOperations.GetCellsAlive", request, response)

			//report RPC to channel
			c.events <- AliveCellsCount{CompletedTurns: response.Turns, CellsCount: response.CellsAlive}
		default:
		}
	}


	//Get server response once gol world done processing on server
	newGolWorld := response.GolWorld
	turn := response.Turns




	// FINISHING UP
	immutableData := makeImmutableMatrix(newGolWorld)

	//Report the final state using FinalTurnCompleteEvent.
	aliveCells := calculateAliveCells(p, immutableData)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
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