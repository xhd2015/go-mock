package stub

import (
	"context"
	"fmt"
	"os"

	"github.com/xhd2015/go-mock/generalmock"
	"github.com/xhd2015/go-mock/mock"
)

func RegisterStubs() {
	mock.AddInterceptor(generalmock.GeneralMockInterceptor)
	mock.AddInterceptor(func(ctx context.Context, stubInfo *mock.StubInfo, inst, req, resp interface{}, f mock.Filter, next func(ctx context.Context) error) error {
		if os.Getenv("GO_TEST_DISABLE_MOCK_CALLING_LOG") != "true" {
			fmt.Printf("calling:%s\n", stubInfo)
		}
		return next(ctx)
	})
}
