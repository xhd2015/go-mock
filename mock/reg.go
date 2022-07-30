package mock

import (
	"fmt"
	"reflect"

	"github.com/xhd2015/go-mock/inspect/typeinfo"
)

type MockStubRegistry struct {
	PkgMapping map[string]*PkgRegistry
}
type PkgRegistry struct {
	FuncMapping map[string]map[string]typeinfo.Func
}

// mockStubRegistry records all generated mock stubs
// key level: pkgName -> ownerName -> funcName
var mockStubRegistry = &MockStubRegistry{PkgMapping: make(map[string]*PkgRegistry)}

func GetMockStubs() *MockStubRegistry {
	return mockStubRegistry
}

// RegisterMockStub
// ownerType always passed as: (*X)(nil)
func RegisterMockStub(pkg string, owner string, ownerType reflect.Type, name string, args []typeinfo.TypeInfo, results []typeinfo.TypeInfo, firstIsCtx bool, lastIsErr bool) {
	preg := mockStubRegistry.PkgMapping[pkg]
	if preg == nil {
		preg = &PkgRegistry{FuncMapping: make(map[string]map[string]typeinfo.Func)}
		mockStubRegistry.PkgMapping[pkg] = preg
	}

	oreg := preg.FuncMapping[owner]
	if oreg == nil {
		oreg = make(map[string]typeinfo.Func)
		preg.FuncMapping[owner] = oreg
	}
	if _, ok := oreg[name]; ok {
		panic(fmt.Errorf("duplicate register:pkg=%s, type=%s, func=%s", pkg, owner, name))
	}
	oreg[name] = typeinfo.NewFunc(args, results)
}
