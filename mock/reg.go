package mock

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/xhd2015/go-mock/inspect/serialize"
	"github.com/xhd2015/go-mock/inspect/typeinfo"
)

type MockStubRegistry struct {
	PkgMapping map[string]*PkgRegistry
}
type PkgRegistry struct {
	FuncMapping map[string]map[string]typeinfo.Func
}

type BuildInfo struct {
	MainModule string
}

// mockStubRegistry records all generated mock stubs
// key level: pkgName -> ownerName -> funcName
var mutext sync.Mutex
var mockStubRegistry = &MockStubRegistry{PkgMapping: make(map[string]*PkgRegistry)}
var typesReg = typeinfo.NewGenerator()
var buildInfo = &BuildInfo{}

func GetMockStubs() *MockStubRegistry {
	mutext.Lock()
	defer mutext.Unlock()
	return mockStubRegistry
}
func GetMockTypes() typeinfo.Definitions {
	mutext.Lock()
	defer mutext.Unlock()
	return typesReg.Definitions(nil /*all*/)
}

func GetBuildInfo() *BuildInfo {
	return buildInfo
}
func SetBuildInfo(info *BuildInfo) {
	buildInfo = info
}

// RegisterMockStub
// ownerType always passed as: (*X)(nil)
func RegisterMockStub(pkg string, owner string, ownerType reflect.Type, name string, args []typeinfo.TypeInfo, results []typeinfo.TypeInfo, firstIsCtx bool, lastIsErr bool) {
	mutext.Lock()
	defer mutext.Unlock()
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

	// always prefer $ref
	makeType := func(types []typeinfo.TypeInfo, makeDefault bool) []typeinfo.TypeInfo {
		newTypes := make([]typeinfo.TypeInfo, 0, len(types))
		for _, t := range types {
			typesReg.Gen(t.Type().Reflect()) // register global types
			var defaultVal interface{}
			if makeDefault {
				defV := typeinfo.MakeDefault(t.Type().Reflect(), nil /* no opts*/)
				data, err := json.Marshal(serialize.JSONSerialize(defV))
				if err != nil {
					panic(fmt.Errorf("marshal err:pkg=%v, name=%v T=%v %v", pkg, name, t.Type().Reflect(), err))
				}
				defaultVal = json.RawMessage(data)
			}
			newTypes = append(newTypes, &regType{TypeInfo: t, Default: defaultVal})
		}
		return newTypes
	}

	oreg[name] = typeinfo.NewFunc(makeType(args, false), makeType(results, true))
}

type regType struct {
	typeinfo.TypeInfo
	Default  interface{}
	jsonData []byte
	jsonErr  error
	once     sync.Once
}

func (c *regType) init() {
	c.once.Do(func() {
		mutext.Lock()
		defer mutext.Unlock()

		// NOTE: always prefer $ref
		t := typeinfo.RefOrUse(typesReg.Gen(c.TypeInfo.Type().Reflect()))
		m := map[string]interface{}{
			"Name":    c.Name(),
			"Type":    t,
			"Default": c.Default,
		}
		c.jsonData, c.jsonErr = json.Marshal(m)
	})
}

func (c *regType) MarshalJSON() ([]byte, error) {
	c.init()
	return c.jsonData, c.jsonErr
}
