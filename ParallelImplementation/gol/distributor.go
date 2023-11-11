package gol

import (
	"fmt"
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	//Send filename to IO and instruct it to read the file c.ioCommand channel
	filename := fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	c.ioCommand <- ioInput
	c.ioFilename <- filename

	//Receive <filename>'s data from IO go routine
	world := receiveInput(p, c)

	//Execute all turns of gol
	for turn := 0; turn < p.Turns; turn++ {
		world = calculateNextState(world)
	}

	//Report final state using FinalTurnComplete event
	complete := FinalTurnComplete{CompletedTurns: p.Turns, Alive: calculateAliveCells(world)}
	c.events <- complete

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- StateChange{p.Turns, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func receiveInput(p Params, c distributorChannels) [][]byte {
	world := make([][]byte, p.ImageHeight)
	for y, slice := range world {
		slice = make([]byte, p.ImageWidth)
		for x, _ := range slice {
			slice[x] = <-c.ioInput
		}
		world[y] = slice
	}
	return world
}

func calculateNextState(world [][]byte) [][]byte {
	//Copying the 2D slice 'world' into 'newWorld':
	//The backing arrays must be different
	newWorld := make([][]byte, len(world))
	for y, slice := range world {
		destination := make([]byte, len(slice))
		copy(destination, slice)  //Hard Copy
		newWorld[y] = destination //Soft Copy
	}

	for Y, slice := range world {
		for X, val := range slice {
			liveNeighbours := 0
			//Iterate over the surrounding 8 cells
			for y := Y - 1; y < Y-1+3; y++ {
				for x := X - 1; x < X-1+3; x++ {
					xIndex := x
					yIndex := y

					//Check it is within bounds, adjust if needed
					if x < 0 {
						xIndex = x + len(slice)
					} else if x == len(slice) {
						xIndex = x - len(slice)
					}
					if y < 0 {
						yIndex = y + len(world)
					} else if y == len(world) {
						yIndex = y - len(world)
					}
					//Check if cell is alive
					if world[yIndex][xIndex] == byte(255) && !(xIndex == X && yIndex == Y) {
						liveNeighbours += 1
					}
				}
			}
			newWorld[Y][X] = determineVal(liveNeighbours, val)
		}
	}
	//fmt.Println(fmt.Sprintf("Number of alive cells: %d", calculateNoLiveCells(newWorld)))
	return newWorld
}

func determineVal(LN int, currentVal byte) byte {
	//LN : LiveNeighbours
	//If cell is alive
	if currentVal == byte(255) {
		if LN < 2 {
			return byte(0) //dies
		}
		if LN == 2 || LN == 3 {
			return currentVal //unaffected
		}
		if LN > 3 {
			return byte(0) //dies
		}
	}
	//If cell is dead
	if currentVal == byte(0) {
		if LN == 3 {
			return byte(255) //lives
		}
	}
	return byte(currentVal)
}

func calculateAliveCells(world [][]byte) []util.Cell {
	var cells []util.Cell
	for y, slice := range world {
		for x, val := range slice {
			if val == byte(255) {
				cells = append(cells, util.Cell{X: x, Y: y})
			}
		}
	}
	return cells
}
