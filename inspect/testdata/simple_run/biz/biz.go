package biz

import (
	"context"
	"fmt"
)

func Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("Run:status = %v\n", status)
	return 0, nil
}

type Status int

func (c Status) Run(ctx context.Context, status int, _ string) (int, error) {
	fmt.Printf("Status Run:status = %v\n", status)
	return 0, nil
}