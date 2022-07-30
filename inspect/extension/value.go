package extension

import (
	"reflect"
)

const _SKIP_MOCK = true

// extension point for stringify assert value
var StringifyValue = func(v interface{}) (val string, ok bool) {
	return "", false
}

var ParseValueFromJSON = func(jsonValue AnyJSON, v interface{}) (err error, ok bool) {
	return nil, false
}

var DefaultValue = func(t reflect.Type) (val interface{}, ok bool) {
	return nil, false
}
