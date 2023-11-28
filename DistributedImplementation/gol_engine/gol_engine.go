package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

const (
	Running int = 0
	Pausing int = 1
	Quiting int = 2
	Killing int = 3
	Waiting int = 4
)





type EngineStateRequest struct {
	State int
}

type StartEngineRequest struct {
	GolWorld [][]uint8
	ImageHeight int
	ImageWidth int
	StartHeight int
	EndHeight int
}

type StartEngineResponse struct {
	GolWorld [][]uint8
}


type EmptyRpcResponse struct {}

func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

func calculateNextState(imageHeight, imageWidth, startY, endY int, data func(y, x int) uint8) [][]uint8 {

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

func (g *GoLOperations) RunEngine(req StartEngineRequest, res *StartEngineResponse) (err error) {
	fmt.Println("GoLOperations.RunEngine")
	//Processing only the strip of the image, then return that strip in the response
	newStripData := calculateNextState(req.ImageHeight, req.ImageWidth, req.StartHeight, req.EndHeight, makeImmutableMatrix(req.GolWorld))
	res.GolWorld = newStripData
	return
}

func (g *GoLOperations) SetGolEngineState(req EngineStateRequest, res *EmptyRpcResponse) (err error) {
	fmt.Println("GoLOperations.SetGolEngineState called")
	if req.State == Killing {
		g.killingChannel <- true
	}
	return
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	killingChannel := make(chan bool)
	golOps := &GoLOperations{killingChannel: killingChannel}
	rpc.Register(golOps)


	listener, _ := net.Listen("tcp", ":"+*pAddr)

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

	fmt.Println("Engine gracefully stopped.")
}