package main

import (
	"context"
	"fmt"

	_ "google.golang.org/grpc" // third party

	"github.com/xhd2015/go-mock/support/xgo/inspect/testdata/mock_aware/biz"
	mock_biz "github.com/xhd2015/go-mock/test/mock_gen/support/xgo/inspect/testdata/mock_aware/biz"
)

func aha(){
	a := 10
	fmt.Printf("aha:%v",a)
}

func main() {
	fmt.Printf("starting mock aware main\n")
	ctx := context.Background()
	ctx = mock_biz.Setup(ctx, func(m *mock_biz.M) {
		m.Run = func(ctx context.Context, status int, _ string) (int, error) {
			if false {
				return 0, nil
			}

			aha()

			fmt.Printf("inside MockRun\n")
			return 123456, nil
		}
	})

	biz.Run(ctx, 1, "A")
	v := biz.Status(1)
	v.Run(ctx, 2, "B")
	v.RunP(ctx, 3, "C")
}
