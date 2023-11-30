package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

// HaloExchangeRequest HALO-EXCHANGE
type HaloExchangeRequest struct {
	Section [][]uint8
	Turns   int
}

// HaloExchangeResponse HALO-EXCHANGE
type HaloExchangeResponse struct {
	Section [][]uint8
}

// InitialiseConnectionRequest HALO EXCHANGE STRUCT
type InitialiseConnectionRequest struct {
	AboveIP      string
	BelowIP      string
	WorkerID     int
	Benchmarking bool
}

// InitialiseConnectionResponse HALO EXCHANGE STRUCT
type InitialiseConnectionResponse struct {
}

// GetRowRequest HALO-EXCHANGE
type GetRowRequest struct {
	RowRequired string //either top or bottom
}

// GetRowResponse HALO-EXCHANGE
type GetRowResponse struct {
	Row []uint8
}

type HaloExchange struct {
	//Clients
	above       *rpc.Client
	below       *rpc.Client
	distributor *rpc.Client

	GetRowLock        sync.Mutex
	AllowGetRow       bool
	AllowGetRowChan   chan bool
	TopSent           chan bool
	BottomSent        chan bool
	AllowGetRowTop    chan bool
	AllowGetRowBottom chan bool

	//Section to simulate gol upon
	section [][]uint8

	aboveIP  string
	belowIP  string
	distIP   string
	WorkerID int

	//Benchmarking
	Benchmarking bool //If true then do not make CellsFlippedRequests back to distributor
}

func (g *HaloExchange) Simulate(req HaloExchangeRequest, res *HaloExchangeResponse) error {
	fmt.Println("Simulate(): HaloExchange.Simulate")
	(*g).section = req.Section
	wg := sync.WaitGroup{}

	//Allow GetRow() calls to complete
	(*g).AllowGetRowChan <- true
	(*g).AllowGetRowTop <- true
	(*g).AllowGetRowBottom <- true

	//Initialise new 2d matrix
	sourceMatrix := make([][]uint8, len((*g).section)+2)
	for y := 0; y < len(sourceMatrix); y++ {
		sourceMatrix[y] = make([]uint8, len((*g).section[0]))
	}
	bottomRow := make([]uint8, len((*g).section[0]))
	topRow := make([]uint8, len((*g).section[0]))

	for turn := 0; turn < req.Turns; turn++ {
		fmt.Printf("Simulate(): Turn: %v \n", turn)

		newSection := make([][]uint8, len((*g).section))
		for y := 0; y < len(newSection); y++ {
			newSection[y] = make([]uint8, len((*g).section[0]))
		}

		//Request top row
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := GetRowRequest{"bottom"}
			res := new(GetRowResponse)
			fmt.Printf("Simulate(): Requesting bottom row from 'above' worker @: %v\n", (*g).aboveIP)
			err := (*g).above.Call("HaloExchange.GetRow", req, res)
			handleError(err)
			fmt.Printf("Simulate(): received bottom row from 'above' worker @: %v\n", (*g).aboveIP)
			topRow = (*res).Row

		}()

		//Request bottom row
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := GetRowRequest{"top"}
			res := new(GetRowResponse)
			fmt.Printf("Simulate(): Requesting top row from 'below' worker @: %v\n", (*g).belowIP)
			err := (*g).below.Call("HaloExchange.GetRow", req, res)
			handleError(err)
			fmt.Printf("Simulate(): received top row from 'below' worker @: %v\n", (*g).belowIP)
			bottomRow = (*res).Row
		}()
		wg.Wait()

		//Wait for both halo rows to have been sent
		_ = <-(*g).TopSent
		_ = <-(*g).BottomSent
		fmt.Println("Simulate(): Both 'halo rows' sent")

		//Initialise source matrix
		sourceMatrix[0] = topRow
		sourceMatrix[len(sourceMatrix)-1] = bottomRow

		for y := 1; y < len(sourceMatrix)-1; y++ {
			copy(sourceMatrix[y], (*g).section[y-1])
		}

		cellsFlipped := make([]util.Cell, 0)
		source := makeImmutableMatrix(sourceMatrix)

		calcNextState(source, &newSection, &cellsFlipped)

		(*g).section = newSection
		(*g).AllowGetRowTop <- true
		(*g).AllowGetRowBottom <- true

	}
	fmt.Println("Simulate(): SIMULATION COMPLETE")
	(*res).Section = (*g).section

	//Clean up such that gol_engine is ready for next Simulate rpc call
	err := (*g).above.Close()
	handleError(err)
	err = (*g).below.Close()
	handleError(err)

	(*g).TopSent = make(chan bool, 1)
	(*g).BottomSent = make(chan bool, 1)
	(*g).AllowGetRowTop = make(chan bool, 1)
	(*g).AllowGetRowChan = make(chan bool, 1)
	(*g).AllowGetRowBottom = make(chan bool, 1)
	return nil
}

func calcNextState(source func(y, x int) uint8, newSection *[][]uint8, cellsFlipped *[]util.Cell) {
	for Y, row := range *newSection {
		for X := range row {
			liveNeighbours := 0
			val := source(Y+1, X)
			//Iterate over the surrounding 8 cells
			for y := Y - 1; y < Y-1+3; y++ {
				for x := X - 1; x < X-1+3; x++ {
					//Skip central cell
					if x == X && y == Y {
						continue
					}

					//(x,y) -> (x,y+1) adjustment
					sourceX := x
					sourceY := y + 1

					//"Wrap around" on x-axis but not on y-axis
					if sourceX < 0 {
						sourceX += len(row) //+ width
					} else if sourceX == len(row) { //==width
						sourceX -= len(row)
					}

					//Check if cell is alive
					if source(sourceY, sourceX) == 255 {
						liveNeighbours++
					}
				}
			}

			(*newSection)[Y][X] = determineVal(liveNeighbours, val, cellsFlipped, Y, X)
		}
	}
}

func (g *HaloExchange) GetRow(req GetRowRequest, res *GetRowResponse) error {
	fmt.Printf("GetRow(): HaloExchange.GetRow: %v\n", req.RowRequired)
	(*g).GetRowLock.Lock()
	if !(*g).AllowGetRow {
		(*g).AllowGetRow = <-(*g).AllowGetRowChan
	}
	(*g).GetRowLock.Unlock()

	if req.RowRequired == "top" {
		<-(*g).AllowGetRowTop
		(*res).Row = (*g).section[0]
		(*g).TopSent <- true
		return nil

	} else if req.RowRequired == "bottom" {
		<-(*g).AllowGetRowBottom
		(*res).Row = (*g).section[len((*g).section)-1]
		(*g).BottomSent <- true
		return nil
	}

	fmt.Printf("GetRow: Unrecognised row required: %v \n", req.RowRequired)
	os.Exit(1)
	return nil
}

func (g *HaloExchange) InitialiseConnection(req InitialiseConnectionRequest, res *InitialiseConnectionResponse) error {
	fmt.Println("InitialiseConnection(): HaloExchange.InitialiseConnection")
	var err error

	(*g).WorkerID = req.WorkerID
	(*g).above, err = rpc.Dial("tcp", req.AboveIP)
	(*g).aboveIP = req.AboveIP
	if err != nil {
		fmt.Println("InitialiseConnection(): Error occurred whilst attempting to connect to 'above' worker ")
		handleError(err)
	}
	fmt.Printf("InitialiseConnection(): g.above connected to %v\n", req.AboveIP)

	(*g).below, err = rpc.Dial("tcp", req.BelowIP)
	(*g).belowIP = req.BelowIP
	if err != nil {
		fmt.Println("InitialiseConnection(): Error occurred whilst attempting to connect to 'above' worker ")
		handleError(err)
	}
	fmt.Printf("InitialiseConnection(): g.below connected to %v\n", req.BelowIP)

	(*g).Benchmarking = req.Benchmarking
	return err
}

func determineVal(LN int, currentVal uint8, cellsFlipped *[]util.Cell, y, x int) uint8 {
	//LN : LiveNeighbours
	if currentVal == 255 {
		if LN < 2 {
			//fmt.Println("Alive & LN < 2 : appending death to cellsFlipped")
			*cellsFlipped = append(*cellsFlipped, util.Cell{X: x, Y: y})
			return 0 //dies by under-population
		}
		if LN == 2 || LN == 3 {
			return currentVal //unaffected
		}
		if LN > 3 {
			//fmt.Println("Alive & LN > 3 : appending  death to cellsFlipped")

			*cellsFlipped = append(*cellsFlipped, util.Cell{X: x, Y: y})
			return 0 //dies by over population
		}
	} else if currentVal == 0 {
		//fmt.Println("Dead & LN == 3 : appending  life to cellsFlipped")
		if LN == 3 {
			*cellsFlipped = append(*cellsFlipped, util.Cell{X: x, Y: y})

			return 255 //lives
		}
	}
	return currentVal
}

func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

func handleError(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	pAddr := flag.String("port", "8040", "Port to listen on")
	flag.Parse()
	fmt.Printf("Main(): Listening on port %v\n", *pAddr)

	g := HaloExchange{
		TopSent:           make(chan bool, 1),
		BottomSent:        make(chan bool, 1),
		AllowGetRowChan:   make(chan bool, 1),
		AllowGetRowTop:    make(chan bool, 1),
		AllowGetRowBottom: make(chan bool, 1),
	}
	err := rpc.Register(&g)
	handleError(err)

	listener, _ := net.Listen("tcp", ":"+*pAddr)

	defer func() {
		err = listener.Close()
		handleError(err)
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println(err)
		}
		go rpc.ServeConn(conn)
	}

}

/*
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
}*/
