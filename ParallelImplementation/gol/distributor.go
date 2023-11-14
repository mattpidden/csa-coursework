package gol

import (
	"fmt"
	"time"
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

type Section struct {
	StartPoint Coord
	EndPoint   Coord //Exclusive
}

type Output struct {
	Reference *[][]byte
	_Section  Section
}

type WorkerIO struct {
	outputChan chan Output
	workChan   chan Section
}
type Coord struct {
	x     int
	y     int
	Pixel int
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	fmt.Printf("\n\n Distributor starting \n\n")

	filename := fmt.Sprintf("%dx%d", p.ImageHeight, p.ImageWidth)
	c.ioCommand <- ioInput
	c.ioFilename <- filename

	//Receive <filename>'s data from IO go routine
	world := receiveInput(p, c) //Working correctly

	//Create a copy of equal size in which to store new values
	newWorld := make([][]byte, p.ImageWidth)
	for y, slice := range world {
		destination := make([]byte, len(slice))
		copy(destination, slice)  //Hard Copy
		newWorld[y] = destination //Soft Copy
	}

	//Start worker threads
	IOs := make([]WorkerIO, p.Threads)
	outputChannel := make(chan Output) //All worker routines are sent the same Output channel

	fmt.Printf("p.Threads: %v\n", p.Threads)
	for worker := 0; worker < p.Threads; worker++ {
		workerIO := WorkerIO{outputChannel, make(chan Section)}
		IOs[worker] = workerIO //Store all channel references in IOs slice

		//Start a workerRoutine for every thread
		go workerRoutine(&world, &newWorld, workerIO)
		fmt.Printf("Worker %v started...\n", worker)
	}

	//Determine section size
	var sectionSize int
	sectionSize = p.ImageWidth * p.ImageWidth / p.Threads //Round up the division
	if (p.ImageWidth * p.ImageWidth % p.Threads) != 0 {
		sectionSize++
	}

	//DEBUG-INFO
	fmt.Printf("sectionSize: %v \n", sectionSize)
	fmt.Printf("p.Turns: %v \n", p.Turns)
	//DEBUG-INFO-END

	timer := time.NewTimer(2 * time.Second)

	//Execute all turns
	done := make(chan bool)
	turn := 0
	go func() {
		for ; turn < p.Turns; turn++ {
			fmt.Printf("Simulating turn: %v \n", turn)

			//Generate sections and send to worker go routines
			pixel := 0
			for i := 0; i < p.Threads-1; i++ {
				section := Section{StartPoint: pixelToCoord(p, pixel), EndPoint: pixelToCoord(p, pixel+sectionSize)}
				fmt.Printf("section: %v \n", section)
				IOs[i].workChan <- section
				pixel += sectionSize
			}

			section := Section{StartPoint: pixelToCoord(p, pixel), EndPoint: pixelToCoord(p, p.ImageWidth*p.ImageHeight)}
			//fmt.Printf("section: %v \n", section)
			IOs[p.Threads-1].workChan <- section

			//Wait for all workers to finished
			for i := 0; i < p.Threads; i++ {
				<-outputChannel
			}
			fmt.Printf("Worker finished...\n")

			//Copy newWorld into world
			for y, slice := range newWorld {
				destination := make([]byte, len(slice))
				copy(destination, slice) //Hard Copy
				world[y] = destination   //Soft Copy
			}
			c.events <- TurnComplete{turn}
		}
		done <- true
	}()

	//generalLock := sync.Mutex{}

	for {
		select {
		case <-done:
			//Report final state using FinalTurnComplete event
			complete := FinalTurnComplete{CompletedTurns: p.Turns, Alive: calculateAliveCells(world)}
			c.events <- complete

			// Make sure that the Io has finished any output before exiting.
			c.ioCommand <- ioCheckIdle
			<-c.ioIdle
			c.events <- StateChange{p.Turns, Quitting}

			// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
			close(c.events)
		case <-timer.C:
			c.events <- AliveCellsCount{CellsCount: calculateNumAliveCells(world), CompletedTurns: turn}
		}
	}

}

//WRONG NEEDS FIXING
func pixelToCoord(p Params, pixel int) Coord {
	return Coord{
		x:     pixel % p.ImageWidth,
		y:     pixel / p.ImageWidth,
		Pixel: pixel,
	}
}

func workerRoutine(world *[][]byte, newWorld *[][]byte, io WorkerIO) {
	for {
		section := <-io.workChan
		calculateNextState(world, newWorld, section)
		io.outputChan <- Output{newWorld, section}
	}

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

func calculateNextState(world *[][]byte, newWorld *[][]byte, section Section) *[][]byte {
	//height := len(*world)
	width := len((*world)[0])

	//fmt.Printf("height: %v, width: %v \n", height, width)
	//fmt.Printf("calculateNextState for section: %v \n", section)

	start := section.StartPoint
	end := section.EndPoint

	if start.x != 0 {
		//Simulate top row
		fmt.Println("partial top row")
		slice := (*world)[start.y]
		for X := start.x; X < width; X++ {
			simulateCell(X, start.y, slice[X], world, &slice, newWorld, section)
		}
		start.y += 1
		start.x = 0
	}

	if end.x != 0 {
		//Simulate bottom row
		fmt.Println("partial bottom row")
		slice := (*world)[end.y]
		for X := 0; X < end.x; X++ {
			simulateCell(X, end.y, slice[X], world, &slice, newWorld, section)
		}
		end.x = 0
	}

	//Simulate core
	for Y := start.y; Y < end.y; Y++ {
		slice := (*world)[Y]
		for X, val := range slice {
			simulateCell(X, Y, val, world, &slice, newWorld, section)
		}
	}

	return newWorld
}

func simulateCell(X, Y int, val byte, world *[][]byte, slice *[]byte, newWorld *[][]byte, section Section) {
	liveNeighbours := 0

	//fmt.Printf("simulateCell() on  X: %v , Y: %v \n", X, Y)
	//Iterate over the surrounding 8 cells
	for y := Y - 1; y < Y-1+3; y++ {
		for x := X - 1; x < X-1+3; x++ {
			xIndex := x
			yIndex := y

			//Check it is within bounds, adjust if needed
			if x < 0 {
				xIndex = x + len(*slice)
			} else if x == len(*slice) {
				xIndex = x - len(*slice)
			}
			if y < 0 {
				yIndex = y + len(*world)
			} else if y == len(*world) {
				yIndex = y - len(*world)
			}
			//Check if cell is alive
			if (*world)[yIndex][xIndex] == byte(255) && !(xIndex == X && yIndex == Y) {
				liveNeighbours += 1
			}
		}
	}
	(*newWorld)[Y][X] = determineVal(liveNeighbours, val)
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

func calculateNumAliveCells(world [][]byte) int {
	numAliveCells := 0
	for _, slice := range world {
		for _, val := range slice {
			if val == byte(255) {
				numAliveCells++
			}
		}
	}
	return numAliveCells
}
