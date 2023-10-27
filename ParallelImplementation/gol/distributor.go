package gol

import (
	"strconv"
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

	//Send command to IO, asking to run readPgmImage function
	c.ioCommand <- 1
	//Construct filename from image height and width
	//Send filename to IO, allowing readPgmImage function to process input of image
	c.ioFilename <- strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)

	//Create a 2D slice to store the world.
	golWorld := make([][]byte, p.ImageHeight)
	for i := range golWorld {
		golWorld[i] = make([]byte, p.ImageWidth)
	}

	//Loop through 2d slice
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			//Receive data from channel and assign to 2d slice
			b := <- c.ioInput
			golWorld[i][j] = b
		}
	}


	//Initialize turns to 0
	turn := 0

	//Execute all turns of the Game of Life.
	for t := 0; t < p.Turns; t++ {
		turn = t
		golWorld = calculateNextState(p, golWorld)
	}

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{CompletedTurns: turn}


	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

//Input: p of type Params containing data about the world
//Input: world of type 2d byte slice containing world data
//Returns: world of type 2d byte slice containing the updated world data
func calculateNextState(p Params, world [][]byte) [][]byte {
	//Create future state of world
	future := make([][]byte, p.ImageHeight)
	for i := range future {
		future[i] = make([]byte, p.ImageWidth)
	}

	//Loop through every cell
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {

			//find number of neighbours alive
			aliveNeighbours := 0
			for n := -1; n < 2; n++ {
				for m := -1; m < 2; m++ {
					// Adjusting for edge cases
					x := (i + n + p.ImageHeight) % p.ImageHeight
					y := (j + m + p.ImageWidth) % p.ImageWidth

					if world[x][y] == byte(255) { // Checks if alive
						aliveNeighbours++
					}
				}
			}

			//Adjusts in case current cell is also alive (it would have got counted in the above calculations)
			if world[i][j] == byte(255) {
				aliveNeighbours -= 1
			}

			//Implement rules of life
			if (world[i][j] == byte(255)) && (aliveNeighbours < 2) { 			//cell is alive but lonely and dies
				future[i][j] = 0
			} else if (world[i][j] == byte(255)) && (aliveNeighbours > 3) {   //cell dies due to overpopulation
				future[i][j] = 0
			} else if (world[i][j] == 0) && (aliveNeighbours == 3) {    //a new cell is born
				future[i][j] = byte(255)
			} else {
				future[i][j] = world[i][j] //no change
			}
		}
	}
	return future
}