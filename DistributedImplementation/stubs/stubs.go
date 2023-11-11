package stubs

var SingleThreadExecution = "GoLOperations.SingleThreadExecution"

type Response struct {
	GolWorld [][]uint8
	Turns int
}

type Request struct {
	GolWorld [][]uint8
	Turns int
	ImageHeight int
	ImageWidth int
	Threads int
}