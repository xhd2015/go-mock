package main

import (
	"context"
	"fmt"

	_mock "github.com/xhd2015/go-mock/support/xgo/inspect/mock"
)

func Run(ctx context.Context, status int, unused_2 string) (Resp_0 int, err error) {_mockstatus := _mock.MockStatus_NormalResp;_mockisPanic := false;var _mockerror error;type _mockReqType struct{Arg_status int `json:"status"`;Arg_unused_2 string `json:"unused_2"`;};type _mockRespType struct{Res_Resp_0 int `json:"Resp_0"`;};var _mockreq = _mockReqType{Arg_status: status,Arg_unused_2: unused_2};var _mockresp _mockRespType;_mockstubInfo := &_mock.StubInfo{PkgName:"github.com/xhd2015/go-mock/support/xgo/inspect/testdata",Owner:"",Name:"Run"};ctx,_mockspan := _mock.StartSpan(ctx,_mockstubInfo,_mockreq);defer func(){var normalPanicErr interface{};if _mockstatus ==  _mock.MockStatus_NormalResp {if normalPanicErr = recover(); normalPanicErr != nil {_mockstatus = _mock.MockStatus_NormalError;_mockisPanic = true;};};_mockspan.SetMockStatus(_mockstatus);_mockspan.SetIsPanic(_mockisPanic);_mockspan.SetError(_mockerror);_mockspan.SetResp(_mockresp);_mockspan.End();if normalPanicErr != nil {panic(normalPanicErr);};}();_mockerror,_mockrespMock := _mock.GetMock(ctx,_mockstubInfo,&_mockreq,&_mockresp);if _mockerror != nil {_mockstatus = _mock.MockStatus_MockError;err = _mockerror;return;};if _mockrespMock {_mockstatus = _mock.MockStatus_MockResp;Resp_0 = _mockresp.Res_Resp_0;return;};Resp_0,err = _mockRun(ctx,status,unused_2);if err != nil {_mockerror = err;_mockstatus = _mock.MockStatus_MockError;};return;}; func _mockRun(ctx context.Context, status int, _ string)(int, error){
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
        Run(context.Background(), 1, "haha")
}
