package main

import (
	"github.com/xhd2015/go-mock/archtype/stub"
)

func init() {
	stub.RegisterStubs()
	// Do anything shared by test
}

// build command:
//    go run -mod=readonly github.com/xhd2015/go-mock build -v -debug ./test
// debug:
//    dlv exec --api-version=2 --listen=localhost:2345 --accept-multiclient --headless ./debug.bin
func main() {
	// do what main.go of your product code does
}
