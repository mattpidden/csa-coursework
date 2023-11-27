package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)
// COMMANDS
// go run gol_engine/gol_engine.go
const (
	Running int = 0
	Pausing int = 1
	Quiting int = 2
	Killing int = 3
)

type SingleThreadExecutionResponse struct {
	GolWorld [][]uint8
	Turns int
}

type SingleThreadExecutionRequest struct {
	GolWorld [][]uint8
	Turns int
	ImageHeight int
	ImageWidth int
	Threads int
	ContinuePreviousWorld bool
}


type GetBoardStateResponse struct {
	GolWorld [][]uint8
	Turns int
}

type EngineStateRequest struct {
	State int
}

type EmptyRpcRequest struct {}


func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}


func calculateNextState(imageHeight, imageWidth, turn, startY, endY, startX, endX int, data func(y, x int) uint8) [][]uint8 {

	//Create future state of world
	future := make([][]uint8, imageHeight)
	for i := range future {
		future[i] = make([]uint8, imageWidth)
	}

	//Loop through every cell in given range
	for i := startY; i < endY; i++ {
		for j := 0; j < imageWidth; j++ {

			//find number of neighbours alive
			aliveNeighbours := 0
			for n := -1; n < 2; n++ {
				for m := -1; m < 2; m++ {
					// Adjusting for edge cases (closed domain)
					x := (i + n + imageHeight) % imageHeight
					y := (j + m + imageWidth) % imageWidth

					if data(x,y) == 255 { // Checks if each neighbour cell is alive
						aliveNeighbours++
					}
				}
			}

			//Adjusts in case current cell is also alive (it would have got counted in the above calculations but is not a neighbour)
			if data(i, j) == 255 {
				aliveNeighbours -= 1
			}

			//Implement rules of life
			if (data(i, j) == 255) && (aliveNeighbours < 2) { 				//cell is alive but lonely and dies
				future[i][j] = 0
				//c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			} else if (data(i, j) == 255) && (aliveNeighbours > 3) {     	//cell dies due to overpopulation
				future[i][j] = 0
				//c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			} else if (data(i, j) == 0) && (aliveNeighbours == 3) {    		//a new cell is born
				future[i][j] = 255
				//c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			} else {
				future[i][j] = data(i, j)									//no change
			}
		}
	}
	//trim future world
	future = future[startY:endY]

	return future
}

type GoLOperations struct {
	state int
	golWorld [][]uint8
	imageHeight int
	imageWidth int
	totalTurns int
	turn int
	lock sync.Mutex
	killingChannel chan bool
	wg sync.WaitGroup
}

func (g *GoLOperations) updateGolWorld(newWorld [][]uint8) {
	g.lock.Lock()
	g.golWorld = newWorld
	g.lock.Unlock()
}

func (g *GoLOperations) getGolWorld() [][]uint8 {
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.golWorld
}

func (g *GoLOperations) SingleThreadExecution(req SingleThreadExecutionRequest, res *SingleThreadExecutionResponse) (err error) {
	fmt.Println("GoLOperations.SingleThreadExecution called")
	totalTurns := req.Turns
	imageWidth := req.ImageWidth
	imageHeight := req.ImageHeight
	firstTurn := 0
	newGolWorld := req.GolWorld

	//If a previous world was quite and the new controller would like to continue processing that world...
	if g.state == Quiting && req.ContinuePreviousWorld {
		//Then set all the values to that of the last saved state of previous world
		fmt.Println("Continuing execution of previous world")
		totalTurns = g.totalTurns
		imageWidth = g.imageWidth
		imageHeight = g.imageHeight
		firstTurn = g.turn
		newGolWorld = g.getGolWorld()
	} else {
		//Otherwise set some values in the structure, so that other functions can access them
		g.totalTurns = req.Turns
		g.imageWidth = req.ImageWidth
		g.imageHeight = req.ImageHeight
		g.turn = 0
		g.updateGolWorld(req.GolWorld)
	}

	g.state = Running

	for t := firstTurn; t < totalTurns; t++ {

		//On each iteration, check the state and act accordingly
		currentState := g.state
		if currentState == Quiting {
			fmt.Println("Local Controller Quit")
			break
		} else if currentState == Killing {
			fmt.Println("Killing Distributed System")
			break
		} else if currentState == Pausing {
			fmt.Println("Running Paused")
			for g.state != Running {}
			fmt.Println("Running Resumed")
		}

		//Do the iterations computation
		g.turn = t
		immutableData := makeImmutableMatrix(newGolWorld)
		newGolWorld = calculateNextState(imageHeight, imageWidth, t, 0, imageHeight, 0, imageWidth, immutableData)
		g.updateGolWorld(newGolWorld)
	}

	res.Turns = g.turn
	res.GolWorld = g.getGolWorld()
	fmt.Println("Finished Running SingleThreadExecution ")

	//If killing selected, let main function know to end it all
	if g.state == Killing {
		g.killingChannel <- true
	}

	return
}

func (g *GoLOperations) SetGolEngineState(req EngineStateRequest, res *GetBoardStateResponse) (err error) {
	fmt.Println("GoLOperations.SetGolEngineState called")
	g.state = req.State
	res.Turns = g.turn
	res.GolWorld = g.getGolWorld()
	return
}
func (g *GoLOperations) GetBoardState(req EmptyRpcRequest, res *GetBoardStateResponse) (err error) {
	fmt.Println("GoLOperations.GetBoardState called")
	res.Turns = g.turn
	res.GolWorld = g.getGolWorld()
	return
}


func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	killingChannel := make(chan bool)
	golOps := &GoLOperations{killingChannel: killingChannel}
	rpc.Register(golOps)


	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				// Check if the listener is closed intentionally
				select {
				case <-killingChannel:
					return
				default:
					log.Fatal(err)
				}
			}

			golOps.wg.Add(1)
			go func() {
				defer golOps.wg.Done()
				rpc.ServeConn(conn)
			}()
		}
	}()

	fmt.Println("GolEngine server started on port:", *pAddr)

	// Wait for the server to be signaled to stop
	<-killingChannel

	//Wait for ongoing RPC calls to complete gracefully
	golOps.wg.Wait()

	fmt.Println("Server gracefully stopped.")
}