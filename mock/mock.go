package mock

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

const _SKIP_MOCK = true

type MockStatus string

const (
	MockStatus_NormalResp  MockStatus = "normal_resp"
	MockStatus_NormalError MockStatus = "normal_error"
	MockStatus_MockResp    MockStatus = "mock_resp"
	MockStatus_MockError   MockStatus = "mock_error"
)

type Logger interface {
	SetMockStatus(mockStatus MockStatus)
	SetIsPanic(isPanic bool)
	SetError(err error)
	SetRequest(req interface{})
	SetResp(resp interface{})
}

type StubInfo struct {
	PkgName  string
	Owner    string
	OwnerPtr bool
	Name     string
}

func (c *StubInfo) String() string {
	ownerPrefix := ""
	owner := ""
	ownerCC := ""
	if c.Owner != "" {
		owner = c.Owner
		ownerCC = "."
		if c.OwnerPtr {
			ownerPrefix = "*"
		}
	}
	return fmt.Sprintf("%s%s%s%s.%s", c.PkgName, ownerCC, ownerPrefix, owner, c.Name)
}

type Filter interface {
	NeedTrace() bool
	SetNeedTrace(v bool)

	IsForceUseOld() bool
	SetForceUseOld(force bool)
}

type Interceptor func(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}, f Filter, next func(ctx context.Context) error) error

// GetContext defines extension point where
// when Trap encounters nil context, user can
// to get a context.
var GetContext = func(ctx context.Context) context.Context {
	return ctx
}

// fnMockKey a universal id of a function
type fnMockKey struct {
	pkg   string
	owner string
	name  string
}

// WithMock inject mock into the context
func WithMock(ctx context.Context, pkg string, owner string, name string, fn interface{}) context.Context {
	if fn == nil {
		panic(fmt.Errorf("fn cannot be nil"))
	}
	return context.WithValue(ctx, fnMockKey{pkg: pkg, owner: owner, name: name}, fn)
}

// WithMockSetup traverse the object to register all functions,
// the traversing order is not guranteed to be the same with assign order,
// This is not meant to be called by the user, but by the generated stub file.
func WithMockSetup(ctx context.Context, pkg string, obj interface{}) context.Context {
	return withMockSetup(ctx, pkg, obj)
}

// GetMock get mock from ctx
// it supports interceptor mock and functional mock.
func GetMock(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}) (fn interface{}, mockResp bool, mockErr error) {
	return getMock(ctx, stubInfo, inst, req, resp)
}

// getFunc get functional mock from ctx
func getFunc(ctx context.Context, pkg string, owner string, name string) interface{} {
	if ctx == nil {
		return nil
	}
	return ctx.Value(fnMockKey{pkg: pkg, owner: owner, name: name})
}

// AddInterceptor add function calling interceptor
// order: first added interceptor lastly executed
func AddInterceptor(h Interceptor) {
	addInterceptor(h)
}

// TrapFunc provides trap to function, req and resp have their special format.
func TrapFunc(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}, oldFunc interface{}, hasRecv bool, firstIsCtx bool, lastIsErr bool) error {
	return trapFunc(ctx, stubInfo, inst, req, resp, oldFunc, hasRecv, firstIsCtx, lastIsErr, true)
}

// TrapHandler prodives trap to handler, like RPC,TCP,DB handles. req and resp not passed to the handler.
func TrapHandler(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}, oldHandler func(ctx context.Context) error, hasRecv bool, firstIsCtx bool, lastIsErr bool) error {
	return trapFunc(ctx, stubInfo, inst, req, resp, oldHandler, hasRecv, firstIsCtx, lastIsErr, false)
}

// WithTrace will always emit log event fn panics
// NOTE: provider of WithTrace must deal with nil ctx correctly.
var WithTrace = func(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, fn func(ctx context.Context, logger Logger) error) error {
	return fn(ctx, defaultEmptyLogger)
}

// var StartSpan = func(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}) (context.Context, Logger) {
// 	return ctx, defaultEmptySpan
// }

func CallOld() {
	panic(errCallOld)
}

var interceptors []Interceptor

var combinedInterceptor Interceptor

// first added interceptor last executed
func addInterceptor(h Interceptor) {
	if h == nil {
		panic(fmt.Errorf("interceptor cannot be nil"))
	}
	interceptors = append(interceptors, h)
	last := combinedInterceptor
	if last == nil {
		combinedInterceptor = h
	} else {
		combinedInterceptor = func(ctx context.Context, stubInfo *StubInfo, inst, req, resp interface{}, f Filter, next func(ctx context.Context) error) error {
			return h(ctx, stubInfo, inst, req, resp, f, func(ctx context.Context) error {
				return last(ctx, stubInfo, inst, req, resp, f, next) // f will remain the same for all next
			})
		}
	}
}

func trapFunc(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}, oldFunc interface{}, hasRecv bool, firstIsCtx bool, lastIsErr bool, needProcessArgs bool) error {
	ctx = GetContext(ctx)

	f := &filter{}
	fn := func(ctx context.Context, logger Logger) (err error) {
		status := MockStatus_NormalResp
		isPanic := false
		// if shouldCatchPanic==false, then it will never panic.
		// if shouldCatchPanic==true, then it may panic, with different reasons:
		//  - normal panic ,or
		//  - mock panic
		// nevertheless, the panic should be thrown out in any condition.
		shouldCatchPanic := true
		if f.NeedTrace() && logger != nil {
			defer func() {
				var panicErr interface{}
				if shouldCatchPanic {
					if panicErr = recover(); panicErr != nil {
						if pe, ok := panicErr.(error); ok {
							err = pe
						} else {
							err = fmt.Errorf("%v", panicErr)
						}
						isPanic = true
						if status == MockStatus_NormalResp {
							status = MockStatus_NormalError
						} else {
							// this panic is made on purpose.
							status = MockStatus_MockError
						}
					}
				}
				// end the tracing span.
				logger.SetMockStatus(status)
				logger.SetIsPanic(isPanic)
				logger.SetError(err)
				logger.SetResp(resp)
				if panicErr != nil {
					panic(panicErr)
				}
			}()
		}
		if !f.IsForceUseOld() {
			var mockFn interface{}
			var mockResp bool
			mockFn, mockResp, err = GetMock(ctx, stubInfo, inst, req, resp)
			if mockFn != nil {
				callOld := false
				err = func() error {
					defer func() {
						if e := recover(); e != nil {
							if e == errCallOld {
								callOld = true
								return
							}
							panic(e)
						}
					}()
					return callImplFunc(ctx, mockFn, req, resp, inst, hasRecv, firstIsCtx, lastIsErr, needProcessArgs)
				}()
				if !callOld {
					shouldCatchPanic = false
					status = MockStatus_MockResp
					if err != nil {
						status = MockStatus_MockError
					}
					return
				}
				// continue to callOld
			} else if err != nil {
				shouldCatchPanic = false
				status = MockStatus_MockError
				return
			} else if mockResp {
				shouldCatchPanic = false
				status = MockStatus_MockResp
				return
			}
		}
		err = callImplFunc(ctx, oldFunc, req, resp, inst, hasRecv, firstIsCtx, lastIsErr, needProcessArgs)
		if err != nil {
			status = MockStatus_NormalError
		}
		return
	}
	processor := func(ctx context.Context) error {
		if !f.NeedTrace() {
			return fn(ctx, nil)
		}
		// ctx, span := StartSpan(ctx, stubInfo, inst, req)
		return WithTrace(ctx, stubInfo, inst, req, fn)
	}

	if combinedInterceptor == nil {
		return processor(ctx)
	}

	return combinedInterceptor(ctx, stubInfo, inst, req, resp, f, processor)
}

func callImplFunc(ctx context.Context, fn interface{}, req interface{}, resp interface{}, inst interface{}, hasRecv bool, firstIsCtx bool, lastIsErr bool, needProcessArgs bool) error {
	if fn == nil {
		panic(fmt.Errorf("impl function cannot be nil"))
	}
	v := reflect.ValueOf(fn)
	if v.Kind() != reflect.Func {
		panic(fmt.Errorf("fn is not a function:%T", fn))
	}

	var args []reflect.Value
	if hasRecv {
		args = append(args, reflect.ValueOf(inst))
	}
	if firstIsCtx {
		args = append(args, reflect.ValueOf(ctx))
	}
	if needProcessArgs {
		args = append(args, structToValues(req)...)
	}

	var res []reflect.Value
	t := v.Type()
	if t.IsVariadic() {
		res = v.CallSlice(args)
	} else {
		res = v.Call(args)
	}

	var errVal reflect.Value
	if lastIsErr {
		errVal = res[len(res)-1]
		res = res[:len(res)-1]
	}
	if needProcessArgs {
		valuesToStruct(res, resp)
	}

	if errVal.IsValid() && !errVal.IsNil() {
		return errVal.Interface().(error)
	}
	return nil
}

func structToValues(s interface{}) []reflect.Value {
	v := reflect.ValueOf(s).Elem()
	if v.Kind() != reflect.Struct {
		panic(fmt.Errorf("s is not a *struct:%T", s))
	}
	vals := make([]reflect.Value, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		vals = append(vals, v.Field(i))
	}
	return vals
}
func valuesToStruct(src []reflect.Value, dest interface{}) {
	v := reflect.ValueOf(dest).Elem()
	if v.Kind() != reflect.Struct {
		panic(fmt.Errorf("s is not a *struct:%T", dest))
	}
	if len(src) != v.NumField() {
		panic(fmt.Errorf("expecting %d fields, actual:%d, dest=%T", v.NumField(), len(src), dest))
	}
	for i := 0; i < len(src); i++ {
		v.Field(i).Set(src[i])
	}
}

func withMockSetup(ctx context.Context, pkg string, obj interface{}) context.Context {
	traverseMock(obj, func(owner, name string, fn interface{}) {
		// register non-nil
		ctx = WithMock(ctx, pkg, owner, name, fn)
	})
	return ctx
}
func traverseMock(obj interface{}, callback func(owner string, name string, fn interface{})) {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	unwrapName := func(name string) string {
		return strings.TrimPrefix(name, "M_")
	}
	var doTraverse func(x reflect.Value, owner string, depth int)
	doTraverse = func(x reflect.Value, owner string, depth int) {
		if x.Kind() != reflect.Struct {
			panic(fmt.Errorf("obj must be type of struct,actual:%T", obj))
		}
		for i := 0; i < x.NumField(); i++ {
			f := x.Field(i)
			name := unwrapName(x.Type().Field(i).Name)
			if f.Kind() == reflect.Func {
				if !f.IsNil() {
					// register non-nil
					callback(owner, name, f.Interface())
				}
			} else {
				if depth > 1 {
					panic(fmt.Errorf("found non-function at depth:%d", depth))
				}
				doTraverse(f, name, depth+1)
			}
		}
	}
	doTraverse(v, "", 0)
}

// getMock supports json and functional mock.
func getMock(ctx context.Context, stubInfo *StubInfo, inst interface{}, req interface{}, resp interface{}) (fn interface{}, mockResp bool, mockErr error) {
	ctx = GetContext(ctx)
	fn = getFunc(ctx, stubInfo.PkgName, stubInfo.Owner, stubInfo.Name)
	// if fn is nil,then no mock
	return
}

type filter struct {
	noNeedTrace bool
	forceUseOld bool
}

func (c *filter) NeedTrace() bool {
	return !c.noNeedTrace
}
func (c *filter) SetNeedTrace(v bool) {
	c.noNeedTrace = !v
}
func (c *filter) IsForceUseOld() bool {
	return c.forceUseOld
}
func (c *filter) SetForceUseOld(force bool) {
	c.forceUseOld = force
}

var errCallOld = errors.New("mock: call back to old")

// to let user call original function
// candidate names:
//
// mock.Back()
// mock.Undo()
// mock.Skip()
// mock.Return()
// mock.Release()
// mock.Unlock()
// mock.Unset()
// mock.Fallback()
// mock.Continue()
// mock.Callback()
// mock.CallOld() // good
// mock.Unmock()
// mock.Unuse()
// mock.UseOld() // good
// mock.NoMock()
// mock.UseOriginal()
// mock.UnMock()
// mock.Break()
// mock.GotoOirginal()
// mock.Backoff()
// mock.BackOld()
// mock.UseOld()

type emptySpan struct {
}

var defaultEmptyLogger = emptySpan{}
var _ Logger = emptySpan{} // assert

// End implements Span
func (emptySpan) End() {}

// SetError implements Span
func (emptySpan) SetError(err error) {
}

// SetIsPanic implements Span
func (emptySpan) SetIsPanic(isPanic bool) {
}

// SetMockStatus implements Span
func (emptySpan) SetMockStatus(mockStatus MockStatus) {
}

// SetResp implements Span
func (emptySpan) SetResp(resp interface{}) {
}

// SetResp implements Span
func (emptySpan) SetRequest(resp interface{}) {
}
