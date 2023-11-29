package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

type Broker struct {
	WorkerIPs []string
}

type BeginGolReq struct {
	World        [][]uint8
	Turns        int
	Benchmarking bool
}

type BeginGolRes struct {
	FinishedWorld  [][]uint8
	CompletedTurns int
}

type InitialiseConnectionRequest struct {
	AboveIP      string
	BelowIP      string
	WorkerID     int
	Benchmarking bool
}

// InitialiseConnectionResponse HALO-EXCHANGE
type InitialiseConnectionResponse struct {
}

// HaloExchangeRequest HALO-EXCHANGE
type HaloExchangeRequest struct {
	Section [][]uint8
	Turns   int
}

// HaloExchangeResponse HALO-EXCHANGE
type HaloExchangeResponse struct {
	Section [][]uint8
}

func (b *Broker) BeginSimulation(req BeginGolReq, res *BeginGolRes) error {
	fmt.Println("BeginSimulation():")

	//Variable initialisation
	numWorkers := 4
	world := req.World
	turns := req.Turns
	benchmarking := req.Benchmarking

	height := len(world)

	sectionHeight := height / numWorkers

	//Hardcoded IPs of all workers
	servers := make([]*rpc.Client, numWorkers)
	workerIPs := make([]string, numWorkers)

	/*
		workerIPs[0] = "100.24.91.201:8040"
		workerIPs[1] = "3.225.79.228:8040"
		workerIPs[2] = "52.54.247.23:8040"
		workerIPs[3] = "54.86.16.188:8040"
	*/

	//Private IP addresses
	workerIPs[0] = "172.31.34.110:8040"
	workerIPs[1] = "172.31.44.175:8040"
	workerIPs[2] = "172.31.42.1:8040"
	workerIPs[3] = "172.31.35.38:8040"
	//Initialise connections between workers and broker, and workers and their respective neighbours
	initialiseWorkerConnections(workerIPs, servers, benchmarking)

	//Begin gol computation
	(*res).FinishedWorld = beginGol(servers, sectionHeight, world, numWorkers, turns)
	(*res).CompletedTurns = turns

	return nil
}

func beginGol(servers []*rpc.Client, sectionHeight int, golWorld [][]uint8, numWorkers int, turns int) [][]uint8 {
	fmt.Println("beginGol():")
	wg := sync.WaitGroup{}
	y := 0
	var newGolWorld [][]uint8

	completeSections := make([][][]uint8, numWorkers)
	for i := 0; i < len(servers); i++ {
		section := make([][]uint8, sectionHeight)
		copy(section, (golWorld)[y:y+sectionHeight]) //Shallow copy
		y += sectionHeight

		request := HaloExchangeRequest{Section: section, Turns: turns}
		response := new(HaloExchangeResponse)

		wg.Add(1)
		go func(I int, req HaloExchangeRequest, res *HaloExchangeResponse) {
			fmt.Println("Making HaloExchange.Simulate rpc call")
			err := servers[I].Call("HaloExchange.Simulate", req, res)
			handleError(err)
			fmt.Println("HaloExchange.Simulate rpc call response received")
			completeSections[I] = res.Section
			wg.Done()
		}(i, request, response)
	}
	wg.Wait()
	fmt.Println("All numWorkers have finished computation")

	//Wait for all workers to finish respective simulation, then piece together world from received sections
	for _, section := range completeSections {
		newGolWorld = append(newGolWorld, section...)
	}

	return newGolWorld
}

func initialiseWorkerConnections(IPs []string, servers []*rpc.Client, benchmarking bool) {
	var err error
	for i, IP := range IPs {
		servers[i], err = rpc.Dial("tcp", IP)
		handleError(err)
	}

	//Send InitialiseConnection requests to all workers
	for i, server := range servers {
		uIndex := i - 1 //upper index
		if uIndex == -1 {
			uIndex += len(servers)
		}
		lIndex := (i + 1) % len(servers) //lower index

		request := InitialiseConnectionRequest{
			AboveIP:      IPs[uIndex],
			BelowIP:      IPs[lIndex],
			WorkerID:     i,
			Benchmarking: benchmarking,
		}

		response := new(InitialiseConnectionResponse)

		//Make rpc call to all workers
		fmt.Println("HaloExchange.InitialiseConnection call made")
		err := server.Call("HaloExchange.InitialiseConnection", request, &response)
		handleError(err)
		fmt.Println("Rpc response received")
	}
}

func main() {
	pAddr := flag.String("port", "8040", "Port to listen on")
	flag.Parse()
	fmt.Printf("Main(): Listening on port %v\n", *pAddr)

	numWorkers := 4
	workerIPs := make([]string, numWorkers)
	workerIPs[0] = "100.24.91.201:8040"
	workerIPs[1] = "3.225.79.228:8040"
	workerIPs[2] = "52.54.247.23:80404"
	workerIPs[3] = "54.86.16.188:8040"

	//Define struct
	broker := Broker{WorkerIPs: workerIPs}

	//Register struct and its associated methods to default RPC server
	err := rpc.Register(&broker)
	handleError(err)

	//Start server such that broker can take rpc calls from the Local Controller
	listener, err := net.Listen("tcp", ":"+*pAddr)
	handleError(err)

	for {
		conn, err := listener.Accept()

		if err != nil {
			fmt.Println(err)
		}
		go rpc.ServeConn(conn)
	}

}

func handleError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
