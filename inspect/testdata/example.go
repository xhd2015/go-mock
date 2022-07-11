package testdata

import (
	"context"
	"time"
)

func Run(ctx context.Context, status int, _ string) int {
	return 0
}

func Run2(ctx context.Context, status int, _ string) int { return 0 }

type S struct {
}

func (s *S) Exec(ctx context.Context, status int) string {
	return ""
}

func EmptyArg() {}

func OneCtxArg(ctx context.Context)       {}
func OneCtxArgNoName(context.Context)     {}
func OneCtxArgAnonName(_ context.Context) {}

func OneArgNoCtx(a int) {}

func TwoArg(ctx context.Context, a int) int {
	if true {
		return 123
	}
	return 456
}

// when showing span, should provide a filter to filter in or out things visible.
// should filter
//
// what if the signature changed?
// when to detect version information?
func TwoArg_Gen(ctx context.Context, a int) (int, error) {
	ctx, span := _mock.StartSpan()
	defer span.End()
	var _mockStatus MockStatus = MOCK_NORMAL
	type _mockReqType struct{}
	type _mockRespType struct{}
	defer func(begin time.Time) {
		panicErr := recover()
		if panicErr != nil {
			if _mockStatus != MOCK_ERROR {

			}
		}
		Emit(ctx, &StubInfo{}, _mockStatus, _mockIsPanic, _mockReq, _mockResp)
		if panicErr != nil && _mockStatus != MOCK_ERROR {
			panic(panicErr)
		}
	}(time.Now())
	// the get mock have multiple implementation
	// JSON mock, Behavior mock, Expect Mock
	_mockErr, _mockResp := GetMock(ctx, nil, &StubInfo{Pkg: "<Name>", Owner: "<Name>", Name: "TwoArg_Gen"}, _mockReqType{}, &_mockRespType{})
	if _mockErr != nil {
		_mockStatus = MOCK_ERROR
		if hasErr {
			return
		}
		_mockIsPanic = true
		panic(_mockErr)
	}
	if _mockResp {
		_mockStatus = MOCK_RESP
		if _argNamed {
			a, b, c = _mockResp.a, _mockResp.b, _mockResp.c
			return
		}
		// extract resp
		return _mockResp.a, _mockResp.b, _mockResp.c
	}
	if _argNamed {
		a, b, c = _mockTwoArg_Gen(ctx, a)
		return
	} else {
		resp.X, resp.Y, resp.C = _mockTwoArg_Gen(ctx, a)
		return resp.X, resp.Y, resp.C
	}
}

func _mockTwoArg_Gen(ctx context.Context, a int) {
	if true {
		return 123
	}
	return 456
}
