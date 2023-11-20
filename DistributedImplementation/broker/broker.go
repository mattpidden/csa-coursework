package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"
)

const (
	Running int = 0
	Pausing int = 1
	Quiting int = 2
	Killing int = 3
)

// BROKER REQ/RES STRUCTS

type StartGolExecutionRequest struct {
	GolWorld [][]uint8
	Turns int
	ImageHeight int
	ImageWidth int
	Threads int
	ContinuePreviousWorld bool
}

type StartGolExecutionResponse struct {
	GolWorld [][]uint8
	Turns int
}

// GOL_ENGINE REQ/RES STRUCTS

type StartEngineRequest struct {
	GolWorld [][]uint8
	ImageHeight int
	ImageWidth int
	StartHeight int
	EndHeight int
}

type StartEngineResponse struct {
	GolWorld [][]uint8
}

type BrokerOperations struct {
	state int
	golWorld [][]uint8
	imageHeight int
	imageWidth int
	totalTurns int
	turn int
	lock sync.Mutex
	killingChannel chan bool
}

func (g *BrokerOperations) updateGolWorld(newWorld [][]uint8) {
	g.lock.Lock()
	g.golWorld = newWorld
	g.lock.Unlock()
}

func (g *BrokerOperations) getGolWorld() [][]uint8 {
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.golWorld
}

func (g *BrokerOperations) StartGolExecution(req StartGolExecutionRequest, res *StartGolExecutionResponse) (err error) {
	fmt.Println("BrokerOperations.StartGolExecution called")
	totalTurns := req.Turns
	imageWidth := req.ImageWidth
	imageHeight := req.ImageHeight
	firstTurn := 0
	newGolWorld := req.GolWorld

	//A list of 4 server address, connect to each one and defer the closing of each one
	workerAddresses := []string{"127.0.0.1:8040", "127.0.0.1:8041", "127.0.0.1:8042", "127.0.0.1:8043"}
	// Declare a slice to hold the engines
	var engines []*rpc.Client

	// Loop to dial and defer the closing of each engine
	for _, address := range workerAddresses {
		engine, err := rpc.Dial("tcp", address)
		if err != nil {
			fmt.Println("Error connecting to", address, ":", err)
			g.state = Quiting
		}
		engines = append(engines, engine)
	}
	defer engines[0].Close()
	defer engines[1].Close()
	defer engines[2].Close()
	defer engines[3].Close()

	//Set Up For Iterations
	cuttingHeight := imageHeight/4
	var channels []chan [][]uint8
	for i := 0; i < 4; i++ {
		newChan := make(chan [][]uint8)
		channels = append(channels, newChan)
	}

	//Run each iteration of the GoL on the broker
	for t := firstTurn; t < totalTurns; t++ {
		//Creating var to store new world data in
		var processedGolWorld [][]uint8

		//Assigning each goroutine, their slice of the image, and respective channel
		for i := 0; i < 4; i++ {
			go func(index int) {
				startHeight := index * cuttingHeight
				endHeight := (index + 1) * cuttingHeight
				if index == 3 {
					endHeight = imageHeight
				}
				request := StartEngineRequest{
					GolWorld:    newGolWorld,
					ImageHeight: imageHeight,
					ImageWidth:  imageWidth,
					StartHeight: startHeight,
					EndHeight:   endHeight,
				}
				response := new(StartEngineResponse)
				engines[index].Call("GoLOperations.RunEngine", request, response)
				channels[index] <- response.GolWorld
			}(i)
		}

		//Put the world back together and then repeat
		for i := 0; i < 4; i++ {
			processedGolWorld = append(newGolWorld, <-channels[i]...)
		}

		newGolWorld = processedGolWorld
		g.updateGolWorld(newGolWorld)
		g.turn = t
	}

	//Once all iterations done, return the final gol world
	res.GolWorld = g.getGolWorld()
	res.Turns = g.turn
	return
}

func makeImmutableMatrix(matrix [][]uint8) func(y, x int) uint8 {
	return func(y, x int) uint8 {
		return matrix[y][x]
	}
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	killingChannel := make(chan bool)
	rpc.Register(&BrokerOperations{killingChannel: killingChannel})


	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()

	go func() {
		rpc.Accept(listener)
	}()

	//Waits to receive anything in the killingChannel to kill the server
	<- killingChannel
}