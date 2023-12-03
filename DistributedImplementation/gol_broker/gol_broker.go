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
	WorkerIPs     []string
	WorkerClients []*rpc.Client
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

type InitialiseConnectionResponse struct {
}

type HaloExchangeRequest struct {
	Section [][]uint8
	Turns   int
}

type HaloExchangeResponse struct {
	Section [][]uint8
}

type GetSnapShotRequest struct {
}
type GetSnapShotResponse struct {
	Matrix [][]uint8
}

type GetSnapshotSectionRequest struct {
}
type GetSnapshotSectionResponse struct {
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

	//Initialise connections between workers and broker, and workers and their respective neighbours
	initialiseWorkerConnections(b.WorkerIPs, &(*b).WorkerClients, benchmarking)

	//Begin gol computation
	(*res).FinishedWorld = beginGol((*b).WorkerClients, sectionHeight, world, numWorkers, turns)
	(*res).CompletedTurns = turns

	fmt.Println("Simulation Complete")

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

		//Each worker is sent their section within the request
		request := HaloExchangeRequest{Section: section, Turns: turns}
		response := new(HaloExchangeResponse)

		wg.Add(1)
		go func(I int, req HaloExchangeRequest, res *HaloExchangeResponse) {
			//Make rpc call to worker
			fmt.Println("Making HaloExchange.Simulate rpc call")
			err := servers[I].Call("Worker.Simulate", req, res)
			handleError(err, "Worker.Simulate")
			fmt.Println("Worker.Simulate rpc call response received")
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

	//Return complete matrix
	return newGolWorld
}

func initialiseWorkerConnections(IPs []string, workerClients *[]*rpc.Client, benchmarking bool) {
	var err error
	for i, IP := range IPs {
		(*workerClients)[i], err = rpc.Dial("tcp", IP)
		handleError(err, "rpc.dial")
	}

	//Send InitialiseConnection requests to all workers
	for i, client := range *workerClients {
		uIndex := i - 1 //upper index
		if uIndex == -1 {
			uIndex += len(*workerClients)
		}
		lIndex := (i + 1) % len(*workerClients) //lower index

		request := InitialiseConnectionRequest{
			AboveIP:      IPs[uIndex],
			BelowIP:      IPs[lIndex],
			WorkerID:     i,
			Benchmarking: benchmarking,
		}

		response := new(InitialiseConnectionResponse)

		//Make rpc call to all workers
		fmt.Println("Worker.InitialiseConnection call made")
		err := client.Call("Worker.InitialiseConnection", request, &response)
		handleError(err, "Worker.InitialiseConnection rpc call")
		fmt.Println("Rpc response received")
	}

}

func (b *Broker) GetSnapshot(req GetSnapShotRequest, res *GetSnapShotResponse) error {
	fmt.Println("Broker.GetSnapshot()")
	//Make GetSnapshotSectionReq to all workers
	wg := sync.WaitGroup{}
	sections := make([][][]uint8, len((*b).WorkerClients))
	var matrix [][]uint8

	mu := sync.Mutex{}
	for i, client := range (*b).WorkerClients {
		wg.Add(1)
		go func(Client *rpc.Client, I int) {
			fmt.Printf("I : %v \n", I)
			req := GetSnapshotSectionRequest{}
			res := GetSnapshotSectionResponse{}
			fmt.Println("Making RPC call: worker.GetSnapshotSection")
			err := Client.Call("Worker.GetSnapshotSection", req, &res)
			handleError(err, "Worker.GetSnapshotSection rpc call")
			fmt.Println("Worker.GetSnapshotSection received RPC response received")

			mu.Lock()
			sections[I] = res.Section
			mu.Unlock()
			wg.Done()
		}(client, i)
	}
	wg.Wait()
	fmt.Println("Passed Wait()")

	//Put sections back together
	for _, section := range sections {
		matrix = append(matrix, section...)
	}

	//Give response the snapshot
	res.Matrix = matrix
	return nil
}

func main() {
	pAddr := flag.String("port", "8040", "Port to listen on")
	flag.Parse()
	numWorkers := 4
	workerIPs := make([]string, numWorkers)
	workerClients := make([]*rpc.Client, numWorkers)

	//Private IP addresses - Since workers communicate between themselves on AWS
	workerIPs[0] = "172.31.34.110:8040"
	workerIPs[1] = "172.31.44.175:8040"
	workerIPs[2] = "172.31.42.1:8040"
	workerIPs[3] = "172.31.35.38:8040"

	//Define struct to be registered
	broker := Broker{WorkerIPs: workerIPs, WorkerClients: workerClients}

	//Register struct and its associated methods to default RPC server
	err := rpc.Register(&broker)
	handleError(err, "rpc.register")

	wg := sync.WaitGroup{}
	//Start server such that broker can take rpc calls from the Local Controller
	wg.Add(1)
	go func() {
		fmt.Printf("Main(): Listening on port %v\n", *pAddr)
		listener1, err := net.Listen("tcp", ":"+*pAddr)
		handleError(err, "net.Listen ")
		for {
			fmt.Println("Waiting for connections")
			conn, err := listener1.Accept()
			fmt.Println(conn)
			fmt.Println("Accepted connection on port :8040")
			if err != nil {
				fmt.Println(err)
			}
			go func(Conn net.Conn) {
				rpc.ServeConn(Conn)
			}(conn)
		}
		wg.Done()
	}()

	wg.Wait()
}

func handleError(err error, msg string) {
	if err != nil {
		log.Fatalf("%v : %v \n", msg, err)
	}
}
