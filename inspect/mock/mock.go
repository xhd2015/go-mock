package mock

import (
	"github.com/xhd2015/go-mock/inspect/typeinfo"
	"github.com/xhd2015/go-mock/mock"
)

// This file defines a direct dependency of the generated code.
// All public names here(should here only have public names) can be accessed by dependency code.

const _SKIP_MOCK = true

type StubInfo = mock.StubInfo

var TrapFunc = mock.TrapFunc
var WithMockSetup = mock.WithMockSetup

var RegisterMockStub = mock.RegisterMockStub

type TypeInfo = typeinfo.TypeInfo

var NewTypeInfo = typeinfo.NewTypeInfo

type BuildInfo = mock.BuildInfo

var SetBuildInfo = mock.SetBuildInfo
