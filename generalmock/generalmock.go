package generalmock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/xhd2015/go-mock/mock"
)

// PutLocal defines how to associate mockData with current goroutine
var PutLocal func(mockData *MockData)

// GetLocal defines how to get mockData associated with current goroutine
var GetLocal func() *MockData

// Unmarshal defines how to unmarshal send-in data into memory structs
var Unmarshal = func(data []byte, dst interface{}) error {
	return json.Unmarshal(data, dst)
}

type RespErr struct {
	Resp  json.RawMessage
	Error string
}
type MockData struct {
	Mapping        map[string]map[string]*RespErr   `json:",omitempty"`
	MappingList    map[string]map[string][]*RespErr `json:",omitempty"` // if multiple
	requestCounter map[string]map[string]int
}

func GeneralMockInterceptor(ctx context.Context, stubInfo *mock.StubInfo, inst, req, resp interface{}, f mock.Filter, next func(ctx context.Context) error) error {
	mockVal := GetGeneralMockData(ctx)
	if mockVal != nil {
		fnKey := stubInfo.Name
		if stubInfo.Owner != "" {
			fnKey = stubInfo.Owner + "." + stubInfo.Name
		}

		var mockRes *RespErr
		if respErrList, ok := mockVal.MappingList[stubInfo.PkgName][fnKey]; ok && len(respErrList) > 0 {
			if mockVal.requestCounter == nil {
				mockVal.requestCounter = make(map[string]map[string]int, 1)
			}
			if mockVal.requestCounter[stubInfo.PkgName] == nil {
				mockVal.requestCounter[stubInfo.PkgName] = make(map[string]int, 1)
			}
			cnt := mockVal.requestCounter[stubInfo.PkgName][fnKey]
			if cnt >= len(respErrList) {
				if len(respErrList) > 0 {
					// take the last
					mockRes = respErrList[len(respErrList)-1]
				}
			} else {
				mockRes = respErrList[cnt]
			}
			mockVal.requestCounter[stubInfo.PkgName][fnKey] = cnt + 1
		} else {
			mockRes = mockVal.Mapping[stubInfo.PkgName][fnKey]
		}
		if mockRes != nil {
			if mockRes.Error != "" {
				return errors.New(mockRes.Error)
			}
			if len(mockRes.Resp) > 0 {
				// err := mocker.Copy(mockRes.Resp, resp)
				err := AsGeneral(resp).UnmarshalJSON([]byte(mockRes.Resp))
				if err != nil {
					panic(fmt.Errorf("copy mock data error:%v", err))
				}
				return nil
			}
		}
	}
	return next(ctx)
}

type generalMockKeyType string

const (
	generalMockKey generalMockKeyType = "generalMock"
)

func (c *MockData) Setup(ctx context.Context) context.Context {
	if PutLocal != nil {
		// aways put into local if given
		PutLocal(c)
	}

	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, generalMockKey, c)
}

func GetGeneralMockData(ctx context.Context) *MockData {
	if ctx == nil {
		// fallback to ctx
		if GetLocal != nil {
			return GetLocal()
		}
		return nil
	}
	mockData, _ := ctx.Value(generalMockKey).(*MockData)
	return mockData
}

type GeneralData struct {
	ptr reflect.Value
}

func AsGeneral(v interface{}) *GeneralData {
	if v == nil {
		panic(fmt.Errorf("val is nil"))
	}
	ptr := reflect.ValueOf(v)
	if ptr.Kind() != reflect.Ptr {
		panic(fmt.Errorf("val must be pointer"))
	}
	if ptr.IsNil() {
		panic(fmt.Errorf("val must be not be nil"))
	}
	if ptr.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("val must be not struct type,found:%v", ptr.Type()))
	}
	return &GeneralData{ptr: ptr}
}
func (c *GeneralData) UnmarshalJSON(data []byte) error {
	v := c.ptr.Elem()
	t := v.Type()
	n := t.NumField()
	if n == 0 {
		return nil
	}
	dst := c.ptr.Interface()
	if n == 1 {
		dst = v.Field(0).Addr().Interface()
	}

	return Unmarshal(data, dst)
}

func (c *GeneralData) Marshal() ([]byte, error) {
	v := c.ptr.Elem()
	t := v.Type()
	n := t.NumField()
	if n == 0 {
		return nil, nil
	}
	src := c.ptr.Interface()
	if n == 1 {
		src = v.Field(0).Interface()
	}
	return json.Marshal(src)
}
