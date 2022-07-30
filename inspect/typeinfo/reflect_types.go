package typeinfo

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
)

var CtxType = reflect.TypeOf((*context.Context)(nil)).Elem()
var ErrorType = reflect.TypeOf((*error)(nil)).Elem()
var EmptyStructType = reflect.TypeOf(&struct{}{}).Elem()
var ByteSliceType = reflect.TypeOf((*[]byte)(nil)).Elem()
var StringType = reflect.TypeOf((*string)(nil)).Elem()
var JSONUnmarshaler = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
var JSONMarshaler = reflect.TypeOf((*json.Marshaler)(nil)).Elem()

func IsPrimitive(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Bool:
		return true
	case reflect.String:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

func SetPrimitive(s string, v reflect.Value) (ok bool) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			v.SetInt(i)
			ok = true
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(s, 10, 64)
		if err == nil {
			v.SetUint(i)
			ok = true
		}
	case reflect.Bool:
		i, err := strconv.ParseBool(s)
		if err == nil {
			v.SetBool(i)
			ok = true
		}
	case reflect.String:
		v.SetString(s)
		ok = true
	case reflect.Float32, reflect.Float64:
		i, err := strconv.ParseFloat(s, 64)
		if err == nil {
			v.SetFloat(i)
			ok = true
		}
	}
	return
}