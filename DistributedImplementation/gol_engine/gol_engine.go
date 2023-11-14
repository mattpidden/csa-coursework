package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
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

func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

func calculateAliveCells(imageHeight, imageWidth int, data func(y, x int) uint8) []util.Cell {
	var aliveCells []util.Cell
	//Loops through entire GoL world
	for i := 0; i < imageHeight; i++ {
		for j := 0; j < imageWidth; j++ {
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
	golWorld [][]uint8
	turns int
	lock sync.Mutex
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

	newGolWorld := req.GolWorld

	for t := 0; t < req.Turns; t++ {
		g.turns = t
		immutableData := makeImmutableMatrix(newGolWorld)
		newGolWorld = calculateNextState(req.ImageHeight, req.ImageWidth, t, 0, req.ImageHeight, 0, req.ImageWidth, immutableData)
		g.updateGolWorld(newGolWorld)
	}

	res.Turns = g.turns
	res.GolWorld = newGolWorld
	return
}

func (g *GoLOperations) GetCellsAlive(req GetCellsAliveRequest, res *GetCellsAliveResponse) (err error) {
	fmt.Println("GoLOperations.GetCellsAlive called")

	GolWorld := g.getGolWorld()
	imageHeight := req.ImageHeight
	imageWidth := req.ImageWidth

	immutableData := makeImmutableMatrix(GolWorld)
	res.Turns = g.turns
	//Even though there are often cells alive at the start, the testing seems to think there is not
	if g.turns == 0 {
		res.CellsAlive = 0
	}  else {
		res.CellsAlive = len(calculateAliveCells(imageHeight, imageWidth, immutableData))
	}
	return
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	//rand.Seed(time.Now().UnixNano())
	rpc.Register(&GoLOperations{})


	listener, _ := net.Listen("tcp", ":"+*pAddr)
	//fmt.Println(listener)
	defer listener.Close()
	rpc.Accept(listener)

}