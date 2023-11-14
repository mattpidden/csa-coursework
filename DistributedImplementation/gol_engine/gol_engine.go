package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
)

type Response struct {
	GolWorld [][]uint8
	Turns int
}

type Request struct {
	GolWorld [][]uint8
	Turns int
	ImageHeight int
	ImageWidth int
	Threads int
}

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

func CalculateNewWorld(golWorld [][]uint8, turns , imageHeight, imageWidth, startX, startY int) [][]uint8 {

	newGolWorld := golWorld

	for t := 0; t < turns; t++ {
		immutableData := makeImmutableMatrix(newGolWorld)
		newGolWorld = calculateNextState(imageHeight, imageWidth, t, startY, imageHeight, startX, imageWidth, immutableData)
	}
	return newGolWorld
}

type GoLOperations struct {}

func (g *GoLOperations) SingleThreadExecution(req Request, res *Response) (err error) {
	fmt.Println("RPC called")
	res.Turns = req.Turns
	res.GolWorld = CalculateNewWorld(req.GolWorld, req.Turns, req.ImageHeight, req.ImageWidth, 0, 0)
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