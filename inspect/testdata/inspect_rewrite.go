package main2

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}

type Status int
func (c Status)  Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}
func (c *Status)  RunP(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("main.Run:status = %v\n", status)
	return 0, nil
}

func Run2(ctx context.Context, status int, _ string) int { return 0 }

type S struct {
}

func (s *S) Exec(ctx context.Context, status int) string {
	return ""
}

func main() {
	Run(context.Background(), 1, "A")
	v := Status(1)
	v.Run(context.Background(), 2, "B")
	v.RunP(context.Background(), 3, "C")
}

func Empty(ctx context.Context) {
	Run(ctx, 1, "haha")
}
