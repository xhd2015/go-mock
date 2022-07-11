package biz

import (
	"context"
	"fmt"

	"github.com/xhd2015/go-mock/support/xgo/inspect/testdata/mock_aware/biz/dep"
	dep2 "github.com/xhd2015/go-mock/support/xgo/inspect/testdata/mock_aware/biz/dep"
)

func Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}

type Status int

func (c Status) Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}
func (c *Status) RunP(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}

// unexported function
func (c Status) run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.run:status = %v\n", status)
	return 0, nil
}

// unexported type
type aString string

func (c Status) run2(ctx context.Context, status int, _ aString) (int, error) {
	fmt.Printf("main.run:status = %v\n", status)
	return 0, nil
}

// types from other packages type
func (c Status) run3(ctx context.Context, status int, _ dep.DepStatus) (int, error) {
	fmt.Printf("main.run:status = %v\n", status)
	return 0, nil
}
// types from other packages type
func (c Status) run3dep2(ctx context.Context, status int, _ dep2.DepStatus) (int, error) {
	fmt.Printf("main.run:status = %v\n", status)
	return 0, nil
}

// ellipsis
func (c Status) run4(ctx context.Context, status int, ellipsis ...string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}

// duplicate error
func (c Status) run5(ctx context.Context, status int,e error) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}
func (c Status) run6(ctx context.Context, status int,_ error) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}
func (c Status) run7(ctx context.Context, status int,err error) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}

type unexp int
func (c unexp) Run(ctx context.Context, status int,err error) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}
func (c unexp) Run2(ctx context.Context, d Status,status int,err error) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}


// special case: types from internal package
// TODO: this may be supported later

func Run2(ctx context.Context, status int, _ string) int { return 0 }

type S struct {
}

func (s *S) Exec(ctx context.Context, status int) string {
	return ""
}

func Empty(ctx context.Context) {
	Run(ctx, 1, "haha")
}
