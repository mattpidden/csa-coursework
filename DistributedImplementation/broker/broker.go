package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"strconv"
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

type GetBoardStateResponse struct {
	GolWorld [][]uint8
	Turns int
}

type EngineStateRequest struct {
	State int
}

type EmptyRpcRequest struct {}

type EmptyRpcResponse struct {}

// GOL_ENGINE REQ/RES STRUCTS

type StartEngineRequest struct {
	Threads int
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
	wg sync.WaitGroup

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

func (g *BrokerOperations) killBroker() {
	g.killingChannel <- true
}


func (g *BrokerOperations) StartGolExecution(req StartGolExecutionRequest, res *StartGolExecutionResponse) (err error) {
	fmt.Println("BrokerOperations.StartGolExecution called on " + strconv.Itoa(req.Turns) + " turns")

	if req.Turns == 0 {
		res.Turns = 0
		res.GolWorld = req.GolWorld
		return
	}

	totalTurns := req.Turns
	imageWidth := req.ImageWidth
	imageHeight := req.ImageHeight
	firstTurn := 0
	newGolWorld := req.GolWorld

	//If a previous world was quit and the new controller would like to continue processing that world...
	if g.state == Quiting && req.ContinuePreviousWorld {
		//Then set all the values to that of the last saved state of previous world
		fmt.Println("Continuing execution of previous world")
		totalTurns = g.totalTurns
		imageWidth = g.imageWidth
		imageHeight = g.imageHeight
		firstTurn = g.turn
		newGolWorld = g.getGolWorld()
	} else {
		//Otherwise set some values in the structure, so that other functions can access them
		g.totalTurns = req.Turns
		g.imageWidth = req.ImageWidth
		g.imageHeight = req.ImageHeight
		g.turn = 0
		g.updateGolWorld(req.GolWorld)
	}

	g.state = Running

	//A list of 4 server address, connect to each one and defer the closing of each one
	workerAddresses := []string{"34.196.66.76:8030", "52.72.215.26:8030", "54.173.105.245:8030", "44.210.32.83:8030"}
	// Declare a slice to hold the engines
	var engines []*rpc.Client

	// Loop to dial and defer the closing of each engine
	for _, address := range workerAddresses {
		engine, err := rpc.Dial("tcp", address)
		if err == nil {
			//fmt.Println("Dialed:", address)
		} else {
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
		//On each iteration, check the state and act accordingly
		currentState := g.state
		if currentState == Quiting {
			fmt.Println("Local Controller Quit")
			break
		} else if currentState == Killing {
			fmt.Println("Killing Distributed System")
			break
		} else if currentState == Pausing {
			fmt.Println("Running Paused")
			for g.state != Running {}
			fmt.Println("Running Resumed")
		}

		g.turn = t
		//Creating var to store new world data in
		var processedGolWorld [][]uint8

		//Assigning each goroutine, their range of the image, and respective channel
		for i := 0; i < 4; i++ {
			go func(index int) {
				startHeight := index * cuttingHeight
				endHeight := (index + 1) * cuttingHeight

				request := StartEngineRequest{
					Threads: req.Threads,
					GolWorld:    newGolWorld,
					ImageHeight: imageHeight,
					ImageWidth:  imageWidth,
					StartHeight: startHeight,
					EndHeight:   endHeight,
				}
				response := new(StartEngineResponse)
				engines[index].Call("GoLOperations.RunParallelEngine", request, response)
				channels[index] <- response.GolWorld
			}(i)
		}


		//Put the world back together and then repeat
		for i := 0; i < 4; i++ {
			processedGolWorld = append(processedGolWorld, <-channels[i]...)
		}

		newGolWorld = processedGolWorld
		g.updateGolWorld(processedGolWorld)
	}

	//Once all iterations done, return the final gol world
	res.GolWorld = g.getGolWorld()
	res.Turns = g.turn
	fmt.Println("Finished Running StartGolExecution")

	//If killing selected, send request to kill all the gol worker engines
	if g.state == Killing {
		for _, engine := range engines {
			request := EngineStateRequest{State: Killing}
			response := new(EmptyRpcResponse)
			engine.Call("GoLOperations.SetGolEngineState", request, response)
		}

	}

	//Then shutdown the broker :(
	if g.state == Killing {
		g.killingChannel <- true
	}
	return
}

func (g *BrokerOperations) GetBoardState(req EmptyRpcRequest, res *GetBoardStateResponse) (err error) {
	fmt.Println("BrokerOperations.GetBoardState called")
	res.Turns = g.turn
	res.GolWorld = g.getGolWorld()
	return
}

func (g *BrokerOperations) SetGolEngineState(req EngineStateRequest, res *GetBoardStateResponse) (err error) {
	fmt.Println("BrokerOperations.SetGolEngineState called")
	g.state = req.State
	res.Turns = g.turn
	res.GolWorld = g.getGolWorld()
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
	brokerOps := &BrokerOperations{killingChannel: killingChannel}
	rpc.Register(brokerOps)


	listener, _ := net.Listen("tcp", ":"+*pAddr)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				// Check if the listener is closed intentionally
				select {
				case <-killingChannel:
					return
				default:
					log.Fatal(err)
				}
			}

			brokerOps.wg.Add(1)
			go func() {
				defer brokerOps.wg.Done()
				rpc.ServeConn(conn)
			}()
		}
	}()

	fmt.Println("Broker server started on port:", *pAddr)

	// Wait for the server to be signaled to stop
	<-killingChannel

	//Wait for ongoing RPC calls to complete gracefully
	brokerOps.wg.Wait()

	fmt.Println("Server gracefully stopped.")
}