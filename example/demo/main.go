package main

import (
	"context"
	"fmt"

	"github.com/xhd2015/go-mock/example/demo/biz"
	mock_biz "github.com/xhd2015/go-mock/example/demo/test/mock_gen/biz"
	"github.com/xhd2015/go-mock/mock"
)

func main() {
	fmt.Printf("main begin\n")

	// add general function interceptor
	mock.AddInterceptor(func(ctx context.Context, stubInfo *mock.StubInfo, inst, req, resp interface{}, f mock.Filter, next func(ctx context.Context) error) error {
		fmt.Printf("calling: %v\n", stubInfo)
		return next(ctx)
	})

	// associate mock into ctx
	ctx := context.Background()
	ctx = mock_biz.Setup(ctx, func(m *mock_biz.M) {
		m.Run = func(ctx context.Context, status int, _ string) (int, error) {
			fmt.Printf("mock biz.Run\n")
			return 123456, nil
		}
	})

	// call biz.Run
	biz.Run(ctx, 1, "A")

	// call biz.Status.Run
	status := biz.Status(1)
	status.Run(ctx, 2, "B")
}
