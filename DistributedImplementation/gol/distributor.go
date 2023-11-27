package gol

import (
	"fmt"
	"net/rpc"
	"strconv"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)


const (
	Running int = 0
	Pausing int = 1
	Quiting int = 2
	Killing int = 3
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
	ContinuePreviousWorld bool
}

type GetCellsAliveResponse struct {
	Turns int
	CellsAlive int
}

type GetBoardStateResponse struct {
	GolWorld [][]uint8
	Turns int
}

type EngineStateRequest struct {
	State int
}

type EmptyRpcRequest struct {}

type EmptyRpcResponse struct {}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
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
	c.events <- TurnComplete{CompletedTurns: 0}

	//Take input of server:port
	//serveradd := "18.233.91.29:8030"
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
		//Change this variable to control if the local controller takes over a previous controllers processing on the remote engine
		ContinuePreviousWorld: true,
	}
	response := new(SingleThreadExecutionResponse)

	//call server (blocking call) in goroutine with channel to indicate once done
	golWorldProcessed := make(chan bool)
	go func() {
		server.Call("GoLOperations.SingleThreadExecution", request, response)
		golWorldProcessed <- true
	}()

	//Setting up chan for 2 second updates
	timesUp := make(chan int)
	//Running go routine to be flagging for updates every 2 seconds
	go timer(timesUp)

	finish := make(chan bool)
	go handleKeyPress(server, p, c, keyPresses, finish)

	latestGolWorld := golWorld

	doneProcessing := false
	for !doneProcessing {
		select {
		case <-golWorldProcessed:
			doneProcessing = true
		case <-timesUp:
			//make RPC call
			emptyRpcRequest := EmptyRpcRequest{}

			boardStateResponse := new(GetBoardStateResponse)
			server.Call("GoLOperations.GetBoardState", emptyRpcRequest, boardStateResponse)
			immutableData := makeImmutableMatrix(boardStateResponse.GolWorld)

			//report alive cell count to channel
			c.events <- AliveCellsCount{CompletedTurns: boardStateResponse.Turns, CellsCount: len(calculateAliveCells(p, immutableData))}

			//Visualise gol on sdl window
			checkForCellFlips(makeImmutableMatrix(latestGolWorld), makeImmutableMatrix(boardStateResponse.GolWorld), boardStateResponse.Turns, p, c)
			c.events <- TurnComplete{CompletedTurns: boardStateResponse.Turns}
			latestGolWorld = boardStateResponse.GolWorld
		default:
		}
	}

	finish <- true

	//Get server response once gol world done processing on server
	newGolWorld := response.GolWorld
	turn := response.Turns

	// FINISHING UP
	immutableData := makeImmutableMatrix(newGolWorld)

	//Output a PGM image of the final board state
	outputImage(filename + "x" + strconv.Itoa(turn), turn, immutableData, p, c)

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
		time.Sleep(time.Second * 2)
		timesUpChan <- 1
	}
}

func checkForCellFlips(oldGolWorld func(y, x int) uint8, newWorld func(y, x int) uint8, turn int, p Params, c distributorChannels) {
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			//If cell values do not match, send cell flipped event
			if oldGolWorld(i, j) != newWorld(i, j) {
				c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			}
		}
	}
}

func handleKeyPress(server *rpc.Client, p Params, c distributorChannels, keyPresses <-chan rune, finish chan bool) {
	for {
		select {
			case <-finish:
				break
			case key := <-keyPresses:
				switch key {
				case 's':
					//Generate a PGM file with the current state of the board (got with a rpc call)
					fmt.Println("s pressed.")
					emptyRpcRequest := EmptyRpcRequest{}
					boardStateResponse := new(GetBoardStateResponse)
					server.Call("GoLOperations.GetBoardState", emptyRpcRequest, boardStateResponse)
					immutableData := makeImmutableMatrix(boardStateResponse.GolWorld)
					filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(boardStateResponse.Turns)
					outputImage(filename, boardStateResponse.Turns, immutableData, p, c)
				case 'q':
					fmt.Println("q pressed.")
					//Close the controller client program without causing an error on the gol engine server.
					//A new local controller should be able to re-interact with the server
					engineStateRequest := EngineStateRequest{State: Quiting}
					boardStateResponse := new(GetBoardStateResponse)
					server.Call("GoLOperations.SetGolEngineState", engineStateRequest, boardStateResponse)
					fmt.Println("Local Controller Quiting")
				case 'k':
					fmt.Println("k pressed.")
					//All components of the distributed system should be shut down cleanly, and the system should output a PGM image of the latest data
					engineStateRequest := EngineStateRequest{State: Killing}
					boardStateResponse := new(GetBoardStateResponse)
					server.Call("GoLOperations.SetGolEngineState", engineStateRequest, boardStateResponse)
					fmt.Println("Killing Distributed System")
				case 'p':
					fmt.Println("p pressed.")
					//Pause the processing on the gol engine server node and have the controller print the current turn that is being processed
					//If p is pressed again resume the processing and have the controller print "Continuing"
					//It is not necessary for q and s to work while the execution is paused.
					engineStateRequest := EngineStateRequest{State: Pausing}
					boardStateResponse := new(GetBoardStateResponse)
					server.Call("GoLOperations.SetGolEngineState", engineStateRequest, boardStateResponse)
					c.events <- StateChange{
						CompletedTurns: boardStateResponse.Turns,
						NewState:       Paused,
					}

					unpaused := false
					for !unpaused {
						switch <-keyPresses {
						case 'p':
							unpaused = true
							engineStateRequest := EngineStateRequest{State: Running}
							boardStateResponse := new(GetBoardStateResponse)
							server.Call("GoLOperations.SetGolEngineState", engineStateRequest, boardStateResponse)
							fmt.Println("Continuing...")
							c.events <- StateChange{
								CompletedTurns: boardStateResponse.Turns,
								NewState:       Executing,
							}
						default:
							fmt.Println("No other functionality available whilst paused. Press 'p' to resume. ")
						}
					}
				}
		}

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

//Input: filename, Name of output file string
//Input: t, Number of turns completed as an int
//Input: data, a closure getter function of the gol world
//Input: p, the params of this gol world
//Input: c, a structure of distributor channels
//Returns: Nothing, instead sends command in events channels to output an image
func outputImage(filename string, t int, data func(y, x int) uint8, p Params, c distributorChannels) {
	c.ioCommand <- 0
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- data(y, x)
		}
	}
	c.events <- ImageOutputComplete{CompletedTurns: t, Filename: filename}
}