package serialize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/xhd2015/go-mock/inspect/extension"
	"github.com/xhd2015/go-mock/inspect/typeinfo"
)

type anyJSON struct {
	v interface{}
}

func WrapJSON(v interface{}) extension.AnyJSON {
	return &anyJSON{v}
}

func (c *anyJSON) GetJSON() ([]byte, error) {
	return json.Marshal(c.v)
}

func (c *anyJSON) Copy(dst interface{}) error {
	return Copy(c.v, dst)
}
func (c *anyJSON) GetString() (str string, ok bool) {
	if c.v == nil {
		return "", false
	}
	if s, ok := c.v.(string); ok {
		return s, true
	}
	if s, ok := c.v.([]byte); ok {
		return string(s), true
	}
	return "", false
}

func Copy(src interface{}, dst interface{}) (err error) {
	bytes, err := json.Marshal(src)
	if err != nil {
		return
	}
	err = Unmarshal(bytes, dst)
	return
}
func Unmarshal(data []byte, req interface{}) (err error) {
	if req == nil {
		err = fmt.Errorf("cannot unmarshal to nil")
		return
	}
	v := reflect.ValueOf(req)
	if v.Kind() != reflect.Ptr {
		err = fmt.Errorf("unmarshal destination must be pointer,found:%T", reflect.TypeOf(req))
		return
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var m interface{}
	if !dec.More() {
		return
	}
	err = dec.Decode(&m)
	if err != nil {
		return
	}

	defer func() {
		if e := recover(); e != nil {
			err = e.(error)
		}
	}()
	rootCause := "<root>"
	doUnmarshal(v, m, nil, &rootCause)
	return
}

// const debug = false
var debug = os.Getenv("GO_DEBUG_UNMARSHAL") == "true"

func doUnmarshal(v reflect.Value, m interface{}, path []string, rootCause *string) {
	if m == nil {
		return
	}
	if len(path) > 50 {
		panic(fmt.Errorf("Unmarshal possibly cyclic reference:%v... ", strings.Join(path[:10], ".")))
	}
	defer func() {
		if e := recover(); e != nil {
			if len(path) > 0 && *rootCause == "<root>" {
				*rootCause = strings.Join(path, ".")
			}
			if len(path) == 0 {
				panic(fmt.Errorf("Unmarshal err:%v %v", *rootCause, e))
			} else {
				panic(e)
			}
		}
	}()
	if !v.IsValid() {
		panic("invalid value")
	}

	// for debug:
	var typeStr string
	if debug {
		typeStr = v.Type().String()
		fmt.Printf("type:%s", typeStr)
	}
	// checkout if both version: T & *T
	// implements json.Unmarshaler
	// dynamic checker have a higher priority than static
	// json.Unmarshaler
	dst := v.Interface()
	if v.Kind() != reflect.Ptr {
		if v.CanAddr() {
			// not temp
			dst = v.Addr().Interface()
		}
	} else if v.IsNil() {
		v.Set(reflect.New(v.Type().Elem()))
		dst = v.Interface()
	}
	err, ok := extension.ParseValueFromJSON(WrapJSON(m), dst)
	if ok {
		if err != nil {
			panic(err)
		}
		return
	}

	callUnmarshaler := func(v interface{}) (ok bool, err error) {
		var unmarshaler json.Unmarshaler
		unmarshaler, ok = v.(json.Unmarshaler)
		if ok {
			var bytes []byte
			bytes, err = json.Marshal(m)
			if err != nil {
				return
			}
			err = unmarshaler.UnmarshalJSON(bytes)
			if err != nil {
				return
			}
		}
		return
	}

	if v.Type().Implements(typeinfo.JSONUnmarshaler) {
		if v.Kind() == reflect.Ptr && v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		ok, err := callUnmarshaler(v.Interface())
		if err != nil {
			panic(err)
		}
		if ok {
			return
		}
	} else if reflect.PtrTo(v.Type()).Implements(typeinfo.JSONUnmarshaler) {
		if v.CanAddr() {
			ok, err := callUnmarshaler(v.Addr().Interface())
			if err != nil {
				panic(err)
			}
			if ok {
				return
			}
		}
	}

	// if dst is []byte, then just use json marshal
	if v.Type() == typeinfo.ByteSliceType {
		if s, ok := m.(string); ok {
			v.Set(reflect.ValueOf([]byte(s)))
			return
		}
		if bytes, ok := m.([]byte); ok {
			v.Set(reflect.ValueOf(bytes))
			return
		}
		// otherwise use json marshal
		bytes, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		v.Set(reflect.ValueOf(bytes))
		return
	} else if v.Type() == typeinfo.StringType {
		if s, ok := m.(string); ok {
			v.Set(reflect.ValueOf(s))
			return
		}
		if bytes, ok := m.([]byte); ok {
			v.Set(reflect.ValueOf(string(bytes)))
			return
		}
		// otherwise use json marshal
		bytes, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		v.Set(reflect.ValueOf(string(bytes)))
		return
	}

	// we can copy nested string into slice,map,object.

	kind := v.Kind()
	switch kind {
	case reflect.Ptr:
		if v.IsNil() {
			// passed in PTR is not assignable,must be nil to do so
			v.Set(reflect.New(v.Type().Elem()))
		}
		doUnmarshal(v.Elem(), m, append(path, "&"), rootCause)
	case reflect.Interface:
		if v.IsNil() {
			// empty interface
			if v.NumMethod() == 0 {
				v.Set(reflect.ValueOf(m))
				return
			}
			// TODO: add special extension
			panic(fmt.Errorf("non empty interface with nil data, cannot unmarshal it: type=%s", v.Type()))
		}
		doUnmarshal(v.Elem(), m, append(path, "@"), rootCause)
	case reflect.Array:
		list := m.([]interface{})
		for i := 0; i < len(list); i++ {
			doUnmarshal(v.Index(i), list[i], append(path, strconv.FormatInt(int64(i), 10)), rootCause)
		}
	case reflect.Slice:
		list := m.([]interface{})
		if v.IsNil() { // may be bug, we must always ensure v's length is enough
			v.Set(reflect.MakeSlice(v.Type(), len(list), len(list)))
		}
		for i := 0; i < len(list); i++ {
			doUnmarshal(v.Index(i), list[i], append(path, strconv.FormatInt(int64(i), 10)), rootCause)
		}
	case reflect.Map:
		mp := m.(map[string]interface{})
		if v.IsNil() {
			v.Set(reflect.MakeMapWithSize(v.Type(), len(mp)))
		}
		for k, e := range mp {
			// map key is special, always string -> int or something like
			// so cannot
			umKey := reflect.New(v.Type().Key()).Elem()
			convertStringToPrimitive(k, umKey)
			// doUnmarshal(umKey, k, append(path, "$key"), rootCause)

			umValue := reflect.New(v.Type().Elem()).Elem()
			doUnmarshal(umValue, e, append(path, k), rootCause)
			v.SetMapIndex(umKey, umValue)
		}
	case reflect.Struct:
		mp := m.(map[string]interface{})
		if len(mp) == 0 {
			return
		}
		removeKeys := make(map[string]bool, len(mp))
		// decode top level fields, then delete used fields
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := v.Type().Field(i)
			if fieldType.Anonymous {
				continue
			}
			jsonName, omitEmpty := typeinfo.GetExportedJSONName(&fieldType)
			if jsonName == "" {
				continue
			}
			if omitEmpty && typeinfo.IsZero(field) {
				continue
			}
			mpVal, ok := mp[jsonName]
			if !ok {
				continue
			}
			removeKeys[jsonName] = true
			// switch field.Kind() {
			// case reflect.Slice:
			// case reflect.Map:
			// case reflect.Ptr,reflect.Interface:
			// 	if field.IsNil() {
			// 		field.Set(reflect.New(field.Type()).Elem())
			// 	}
			// }
			// must always pass a pointer
			fieldArg := field
			if field.Kind() == reflect.Struct {
				fieldArg = field.Addr()
			}
			doUnmarshal(fieldArg, mpVal, append(path, fieldType.Name), rootCause)
		}

		innerM := make(map[string]interface{}, len(mp)-len(removeKeys))
		for k, v := range mp {
			if !removeKeys[k] {
				innerM[k] = v
			}
		}
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := v.Type().Field(i)
			if !fieldType.Anonymous {
				continue
			}
			if field.Kind() == reflect.Ptr {
				if field.IsNil() {
					// init
					field.Set(reflect.New(field.Type()).Elem())
				}
			}
			doUnmarshal(field, innerM, path, rootCause)
		}
	case reflect.Chan, reflect.Func:
		// ignore
	default:
		// string to int, solving the int64 problem
		if typeinfo.IsPrimitive(kind) {
			if s, ok := m.(string); ok {
				if s != "" {
					typeinfo.SetPrimitive(s, v)
				}
				return
			}
		}

		// primitive types, using simple json unmarshal
		bytes, err := json.Marshal(m)
		if err != nil {
			panic(err)
		}
		x := reflect.New(v.Type())
		err = json.Unmarshal(bytes, x.Interface())
		if err != nil {
			panic(err)
		}
		v.Set(x.Elem())
	}
}

func convertStringToPrimitive(s string, v reflect.Value) {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			panic(err)
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			panic(err)
		}
		v.SetUint(i)
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			panic(err)
		}
		v.SetBool(b)
	case reflect.Slice:
		if v.Type() == typeinfo.ByteSliceType {
			v.Set(reflect.ValueOf([]byte(s)))
			return
		}
		panic(fmt.Errorf("cannot convert string to %s", v.Type()))
	case reflect.String:
		v.SetString(s)
	default:
		panic(fmt.Errorf("cannot convert string to %s", v.Type()))
	}
}
