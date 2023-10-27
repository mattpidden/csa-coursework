package gol

import "strconv"

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
	imageData := make([][]int, p.ImageHeight)
	for i := range imageData {
		imageData[i] = make([]int, p.ImageWidth)
	}

	//Loop through 2d slice
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			//Receive data from channel and assign to 2d slice
			b := <- c.ioInput
			imageData[i][j] = int(b)
		}
	}

	//Initialize turns to 0
	turn := 0


	// TODO: Execute all turns of the Game of Life.

	// TODO: Report the final state using FinalTurnCompleteEvent.


	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
