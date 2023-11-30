package gol

import (
	"fmt"
	"log"
	"net/rpc"
	"strconv"
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	fmt.Println("Distributor(): ")

	//BENCHMARKING
	var golWorld [][]uint8

	//Get image data from io
	golWorld = initializeGolMatrix(p, c)

	//Make connection with broker
	brokerIP := "54.175.85.139:8040"
	broker, err := rpc.Dial("tcp", brokerIP)
	handleError(err)

	//Make rpc call to broker
	req := BeginGolReq{
		World: golWorld,
		Turns: p.Turns,
	}
	res := BeginGolRes{}
	broker.Call("Broker.BeginSimulation", req, &res) //Blocking rpc call

	golWorld = res.FinishedWorld

	//Clean up before next distributor call
	fmt.Println("Beginning clean up...")

	aliveCells := calculateAliveCells(p, makeImmutableMatrix(golWorld))
	c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: aliveCells}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{p.Turns, Quitting}
	close(c.events)

	broker.Close()
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

func handleError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
