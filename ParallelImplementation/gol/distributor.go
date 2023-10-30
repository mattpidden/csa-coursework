package gol

import (
	"strconv"
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	//Send command to IO, asking to run readPgmImage function
	c.ioCommand <- 1

	//Construct filename from image height and width
	//Send filename to IO, allowing readPgmImage function to process input of image
	filename := strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.ImageHeight)
	c.ioFilename <- filename

	//Create a 2D slice to store the world.
	golWorld := make([][]uint8, p.ImageHeight)
	for y := range golWorld {
		golWorld[y] = make([]uint8, p.ImageWidth)
	}

	//Loop through 2d slice initializing each cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			//Receive data from channel and assign to 2d slice
			b := <- c.ioInput
			golWorld[y][x] = b
			if b == 255 {
				//Let the event component know which cells start alive
				c.events <- CellFlipped{CompletedTurns: 0, Cell: util.Cell{X: x, Y: y}}
			}
		}
	}

	//Initialize turns to 0
	turn := 0

	//Setting up chan for 2 second updates
	timesUp := make(chan int)
	//Running go routine to be flagging for updates every 2 seconds
	go timer(timesUp)

	//SINGLE THREAD IMPLEMENTATION FOR WHEN THREADS = 1
	if p.Threads == 1 {
		//Execute all turns of the Game of Life.
		for t := 0; t < p.Turns; t++ {

			immutableData := makeImmutableMatrix(golWorld)

			select {
				case <- timesUp:
					//Check if 2 seconds has passed - if so report alive cell count to events
					c.events <- AliveCellsCount{CompletedTurns: t, CellsCount: len(calculateAliveCells(p, golWorld))}
				default:
					//If timer not up, do nothing extra
			}

			golWorld = calculateNextState(0, p.ImageHeight, 0, p.ImageWidth, immutableData, c, t, p)
			turn++
			//Report the completion of each turn
			c.events <- TurnComplete{CompletedTurns: turn}
		}
	} else {
		//PARALLELED MULTIPLE THREAD IMPLEMENTATION
		//Creating slice of channels, initialized with channels, for each worker goroutine
		channels := []chan [][]uint8{}
		for i := 0; i < p.Threads; i++ {
			newChan := make(chan [][]uint8)
			channels = append(channels, newChan)
		}

		//Defining the height of image for each worker
		cuttingHeight := p.ImageHeight/p.Threads

		//Execute all turns of the Game of Life.
		for t := 0; t < p.Turns; t++ {

			//Wrapping starting world in closure
			immutableData := makeImmutableMatrix(golWorld)

			select {
				//Check if 2 seconds has passed - if so report alive cell count to events
				case <-timesUp:
					c.events <- AliveCellsCount{CompletedTurns: t, CellsCount: len(calculateAliveCells(p, golWorld))}
				default:
					//If time not up, do nothing extra
			}

			//Creating var to store new world data in
			var newGolWorld [][]uint8

			//Assigning each goroutine, their slice of the image, and respective channel
			for i := 0; i < p.Threads; i++ {
				startHeight := i * cuttingHeight
				endHeight := (i + 1) * cuttingHeight
				if i == p.Threads-1 {
					endHeight = p.ImageHeight
				}
				go worker(startHeight, endHeight, p, immutableData, c, t, channels[i])
			}

			//Receive all data back from worker goroutines and stitch image back together
			for i := 0; i < p.Threads; i++ {
				newGolWorld = append(newGolWorld, <-channels[i]...)
			}

			golWorld = newGolWorld

			turn++
			//Report the completion of each turn
			c.events <- TurnComplete{CompletedTurns: turn}
		}
	}

	//Report the final state using FinalTurnCompleteEvent.
	aliveCells := calculateAliveCells(p, golWorld)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: aliveCells}
	//Output final state as PGM image
	outputImage(filename + "x" + strconv.Itoa(p.Turns), turn, golWorld, p, c)

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// makeImmutableMatrix takes an existing 2D matrix and wraps it in a getter closure.
func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

func worker(startY int, endY int, p Params, data func(y, x int) uint8, c distributorChannels, turns int, outputChan chan<- [][]uint8) {
	newPixelData := calculateNextState(startY, endY, 0, p.ImageWidth, data, c, turns, p)
	outputChan <- newPixelData
}
//Used to 2 second reporting ticker
//Input: A channel of type int
//No return
func timer(timesUpChan chan int) {
	for {
		timesUpChan <- 1
		time.Sleep(time.Second * 2)

	}
}

//Input: p of type Params containing data about the world
//Input: world of type 2d uint8 slice containing world data
//Input: c of type distributorChannels allowing function to report events
//Input: turn of type int to allow reported events to contain correct turn number
//Returns: world of type 2d uint8 slice containing the updated world data
func calculateNextState(startY, endY, startX, endX int, data func(y, x int) uint8, c distributorChannels, turn int, p Params) [][]uint8 {

	//Create future state of world
	future := make([][]uint8, p.ImageHeight)
	for i := range future {
		future[i] = make([]uint8, p.ImageWidth)
	}

	//Loop through every cell in given range
	for i := startY; i < endY; i++ {
		for j := 0; j < p.ImageWidth; j++ {

			//find number of neighbours alive
			aliveNeighbours := 0
			for n := -1; n < 2; n++ {
				for m := -1; m < 2; m++ {
					// Adjusting for edge cases (closed domain)
					x := (i + n + p.ImageHeight) % p.ImageHeight
					y := (j + m + p.ImageWidth) % p.ImageWidth

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
			if (data(i, j) == 255) && (aliveNeighbours < 2) { 			//cell is alive but lonely and dies
				future[i][j] = 0
				c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			} else if (data(i, j) == 255) && (aliveNeighbours > 3) {     //cell dies due to overpopulation
				future[i][j] = 0
				c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			} else if (data(i, j) == 0) && (aliveNeighbours == 3) {    		//a new cell is born
				future[i][j] = 255
				c.events <- CellFlipped{CompletedTurns: turn, Cell: util.Cell{X: j, Y: i}}
			} else {
				future[i][j] = data(i, j)									//no change
			}
		}
	}
	//trim future world
	future = future[startY:endY]

	return future
}

//Input: p of type Params containing data about the world
//Input: world of type [][]uint8 containing the gol world data
//Returns: slice containing elements of type util.Cell, of all alive cells
func calculateAliveCells(p Params, world [][]uint8) []util.Cell {
	var aliveCells []util.Cell
	//Loops through entire GoL world
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			//If cell is alive, create cell and append to slice
			if world[i][j] == 255 {
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

func calculateAliveCellCount(data func(y, x int) uint8, p Params) int {
	count := 0
	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			if data(i, j) == 255 {
				count++
			}
		}
	}
	return count
}

func outputImage(filename string, t int, world [][]uint8, p Params, c distributorChannels) {
	c.ioCommand <- 0
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
	c.events <- ImageOutputComplete{CompletedTurns: t, Filename: filename}
}