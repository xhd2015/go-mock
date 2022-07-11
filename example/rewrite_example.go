package example

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("biz.Run: %v\n", status)
	return 0, nil
}

type Status int

func (c Status) Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("biz.Status.Run: %v\n", status)
	return 0, nil
}

func run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("biz.run: %v\n", status)
	return 0, nil
}
