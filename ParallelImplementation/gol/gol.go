package gol

import "fmt"

// Params provides the details of how to run the Game of Life and which image to load.
type Params struct {
	Turns       int
	Threads     int
	ImageWidth  int
	ImageHeight int
}

// Run starts the processing of Game of Life. It should initialise channels and goroutines.
func Run(p Params, events chan<- Event, keyPresses <-chan rune) {
	fmt.Println("Started running gol.Run")

	//Put the missing channels in here.
	ioCommand := make(chan ioCommand)
	filename := make(chan string)
	output := make(chan uint8)
	input := make(chan uint8)
	ioIdle := make(chan bool)

	ioChannels := ioChannels{
		command:  ioCommand,
		idle:     ioIdle,
		filename: filename,
		output:   output,
		input:    input,
	}
	go startIo(p, ioChannels)

	distributorChannels := distributorChannels{
		events:     events,
		ioCommand:  ioCommand,
		ioIdle:     ioIdle,
		ioFilename: filename,
		ioOutput:   output,
		ioInput:    input,
	}
	distributor(p, distributorChannels)
}
