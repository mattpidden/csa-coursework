package gol

import (
	"fmt"
	"strconv"
	"sync"
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
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	var lock sync.Mutex
	var wg sync.WaitGroup

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

	var setUpWaitGroup sync.WaitGroup
	//Loop through 2d slice initializing each cell
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			//Receive data from channel and assign to 2d slice
			b := <- c.ioInput
			golWorld[y][x] = b
			if b == 255 {
				//Let the event component know which cells start alive
				setUpWaitGroup.Add(1)
				go cellFlipped(c, 0, util.Cell{X: x, Y: y}, &setUpWaitGroup)
			}
		}
	}
	setUpWaitGroup.Wait()
	//Initialize turns to 0
	turn := 0

	//Setting up chan for 2 second updates
	timesUp := make(chan int)
	//Running go routine to be flagging for updates every 2 seconds
	go timer(timesUp)

	//PARALLELED MULTIPLE THREAD IMPLEMENTATION MEMORY SHARING

	//Defining the height of image for each worker
	//cuttingHeight := p.ImageHeight/p.Threads
	threadRows := distributeRows(p.ImageHeight, p.Threads)

	//Execute all turns of the Game of Life.
	for t := 0; t < p.Turns; t++ {

		//Wrapping starting world in closure
		immutableData := makeImmutableMatrix(golWorld)

		select {
			//Check if 2 seconds has passed - if so report alive cell count to events
			case <-timesUp:
				go aliveCellsCount(t, len(calculateAliveCells(p, immutableData)), c)
			case key := <- keyPresses:
				handleKeyPress(key, t, filename + "x" + strconv.Itoa(t), immutableData, p, c, keyPresses)
			default:
				//If time not up, or not user input: do nothing extra
		}

		//Creating var to store new world data in (not this is not a reference to gol world so can be edited)
		editableGolWorld := make([][]uint8, len(golWorld))
		for i := range golWorld {
			editableGolWorld[i] = make([]uint8, len(golWorld[i]))
			copy(editableGolWorld[i], golWorld[i])
		}

		//Assigning each goroutine, their slice of the image, and respective channel
		for i := 0; i < p.Threads; i++ {
			startHeight := threadRows[i][0]
			endHeight := threadRows[i][1]

			wg.Add(1)
			go func(startHeight, endHeight int) {
				defer wg.Done()
				calculateNextState(startHeight, endHeight, golWorld, editableGolWorld, c, t, p, lock)
			}(startHeight, endHeight)
		}
		turn++

		wg.Wait() //Wait until the worker go routines have all finished updating the editable gol world

		//Report the completion of each turn
		wg.Add(1)
		go turnComplete(turn, &wg, c)
		wg.Wait()

		golWorld = editableGolWorld

	}


	immutableData := makeImmutableMatrix(golWorld)

	//Output final state as PGM image
	outputImage(filename + "x" + strconv.Itoa(p.Turns), turn, immutableData, p, c)

	//Report the final state using FinalTurnCompleteEvent.
	aliveCells := calculateAliveCells(p, immutableData)
	c.events <- FinalTurnComplete{CompletedTurns: turn, Alive: aliveCells}

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}
	
	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

// Creating a function to distribute rows among threads
func distributeRows(imageHeight, numThreads int) map[int][]int {
	result := make(map[int][]int)
	baseRows := imageHeight / numThreads
	extraRows := imageHeight % numThreads
	currentRow := 0
	for i := 0; i < numThreads; i++ {
		rows := baseRows
		// Distribute one extra row at a time until there are none left
		if extraRows > 0 {
			rows++
			extraRows--
		}
		// Calculate endRow for the current thread
		endRow := currentRow + rows
		// Store the range of rows for the current thread
		result[i] = []int{currentRow, endRow}
		// Update currentRow for the next thread
		currentRow = endRow
	}
	return result
}

// makeImmutableMatrix takes an existing 2D matrix and wraps it in a getter closure.
func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

//Used to 2 second reporting ticker
//Input: A channel of type int
//No return
func timer(timesUpChan chan int) {
	for {
		time.Sleep(time.Second * 2)
		timesUpChan <- 1
	}
}

//Input: p of type Params containing data about the world
//Input: world of type 2d uint8 slice containing world data
//Input: c of type distributorChannels allowing function to report events
//Input: turn of type int to allow reported events to contain correct turn number
//Returns: world of type 2d uint8 slice containing the updated world data
func calculateNextState(startY, endY int, golWorld, editableGolWorld [][]uint8, c distributorChannels, turn int, p Params, mu sync.Mutex) {
	var cellsFlippedWaitGroup sync.WaitGroup
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

					if golWorld[x][y] == 255 { // Checks if each neighbour cell is alive
						aliveNeighbours++
					}
				}
			}

			//Adjusts in case current cell is also alive (it would have got counted in the above calculations but is not a neighbour)
			if golWorld[i][j] == 255 {
				aliveNeighbours -= 1
			}

			//Implement rules of life
			if (golWorld[i][j] == 255) && (aliveNeighbours < 2) { 			//cell is alive but lonely and dies
				mu.Lock()
				editableGolWorld[i][j] = 0
				mu.Unlock()
				cellsFlippedWaitGroup.Add(1)
				go cellFlipped(c, turn, util.Cell{X: j, Y: i}, &cellsFlippedWaitGroup)
			} else if (golWorld[i][j] == 255) && (aliveNeighbours > 3) {     //cell dies due to overpopulation
				mu.Lock()
				editableGolWorld[i][j] = 0
				mu.Unlock()
				cellsFlippedWaitGroup.Add(1)
				go cellFlipped(c, turn, util.Cell{X: j, Y: i}, &cellsFlippedWaitGroup)
			} else if (golWorld[i][j] == 0) && (aliveNeighbours == 3) {    		//a new cell is born
				mu.Lock()
				editableGolWorld[i][j] = 255
				mu.Unlock()
				cellsFlippedWaitGroup.Add(1)
				go cellFlipped(c, turn, util.Cell{X: j, Y: i}, &cellsFlippedWaitGroup)
			} else {
				//no change
			}
		}
	}

	//cellsFlippedWaitGroup.Add(1)
	//go turnComplete(turn+1, &cellsFlippedWaitGroup, c)
	cellsFlippedWaitGroup.Wait()
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


func outputImage(filename string, t int, data func(y, x int) uint8, p Params, c distributorChannels) {
	c.ioCommand <- 0
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- data(y, x)
		}
	}
	c.events <- ImageOutputComplete{CompletedTurns: t, Filename: filename}
}

func handleKeyPress(key rune, t int, filename string, data func(y, x int) uint8, p Params, c distributorChannels, keyPresses <-chan rune) {
	switch key {
	case 's':
		outputImage(filename, t, data, p, c)
	case 'q':
		//Output final state as PGM image
		outputImage(filename, t, data, p, c)

		//Report the final state using FinalTurnCompleteEvent.
		aliveCells := calculateAliveCells(p, data)
		c.events <- FinalTurnComplete{CompletedTurns: t, Alive: aliveCells}

		// Make sure that the Io has finished any output before exiting.
		c.ioCommand <- ioCheckIdle
		<-c.ioIdle

		c.events <- StateChange{t, Quitting}

		// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
		close(c.events)
	case 'p':
		c.events <- StateChange{
			CompletedTurns: t,
			NewState:       Paused,
		}
		unpaused := false
		for !unpaused {
			switch <-keyPresses {
			case 'p':
				unpaused = true
				fmt.Println("Continuing...")
			default:
				fmt.Println("Press 'p' to resume. No other functionality available whilst paused.")
			}
		}
	}
}

// DOWN HERE ARE ALL THE FUNCTIONS FOR USE OF IO CHANNELS

func cellFlipped(c distributorChannels, turn int, cell util.Cell, wg *sync.WaitGroup) {
	defer wg.Done()
	c.events <- CellFlipped{CompletedTurns: turn, Cell: cell}
}

func turnComplete(turn int, wg *sync.WaitGroup, c distributorChannels) {
	defer wg.Done()
	c.events <- TurnComplete{CompletedTurns: turn}
}

func aliveCellsCount(turn int, cellsCount int, c distributorChannels) {
	c.events <- AliveCellsCount{CompletedTurns: turn, CellsCount: cellsCount}
}