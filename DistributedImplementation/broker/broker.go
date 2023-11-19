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

type SingleThreadExecutionResponse struct {
	GolWorld [][]uint8
	Turns int
}

type SingleThreadExecutionRequest struct {
	GolWorld [][]uint8
	Turns int
	ImageHeight int
	ImageWidth int
	Threads int
	ContinuePreviousWorld bool
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

func (g *BrokerOperations) StartGolExecution(req StartGolExecutionRequest, res *StartGolExecutionResponse) (err error) {
	fmt.Println("BrokerOperations.StartGolExecution called")

	//TODO have a list of 4 server address, connect to each one
	//TODO run each iteration on this broker
	//TODO split the image up into 4 sections with the extra boundaries
	//TODO send each split to the 4 different servers
	//TODO wait for each server to return the new strip
	//TODO put the world back together and then repeat
	//TODO once all iterations done, return the final gol world

	//Take input of server:port
	//serveradd := "18.233.91.29:8030"
	serveradd := "127.0.0.1:8040"
	server, _ := rpc.Dial("tcp", serveradd)
	defer server.Close()

	//Create request and response
	request := SingleThreadExecutionRequest{
		GolWorld:    req.GolWorld,
		Turns:       req.Turns,
		ImageHeight: req.ImageHeight,
		ImageWidth: req.ImageWidth,
		Threads:     req.Threads,
		ContinuePreviousWorld: req.ContinuePreviousWorld,
	}
	response := new(SingleThreadExecutionResponse)

	//call server (blocking call) in goroutine with channel to indicate once done
	server.Call("GoLOperations.SingleThreadExecution", request, response)

	res.GolWorld = response.GolWorld
	res.Turns = response.Turns
	return
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