package typeinfo

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type MakeDefaultOptions struct {
	DefaultValueProvider func(t reflect.Type) (val interface{}, ok bool)
}

func MakeDefault(t reflect.Type, opts *MakeDefaultOptions) interface{} {
	val := makeDefault(t, nil, opts)
	if !val.IsValid() {
		return nil
	}
	return val.Interface()
}

// makeDefault mock default value for a type.
func makeDefault(t reflect.Type, path []string, opts *MakeDefaultOptions) reflect.Value {
	if len(path) > 1000 {
		panic(fmt.Errorf("makeDefault possibly cyclic reference:%v... ", strings.Join(path[:10], ".")))
	}
	defer func() {
		if len(path) == 0 {
			if e := recover(); e != nil {
				panic(fmt.Errorf("makeDefault err:%v %v", strings.Join(path, "."), e))
			}
		}
	}()

	if opts != nil && opts.DefaultValueProvider != nil {
		defaultValue, ok := opts.DefaultValueProvider(t)
		if ok {
			return reflect.ValueOf(defaultValue)
		}
	}

	kind := t.Kind()
	switch kind {
	case reflect.Ptr:
		p := reflect.New(t.Elem())
		val := makeDefault(t.Elem(), append(path, "&"), opts)
		if val.IsValid() {
			p.Elem().Set(val)
		}
		return p
	case reflect.Interface:
		// v := reflect.New(t.Elem()) // interface type has no Elem()
		// v.Elem().Set(mockType(t.Elem(), append(path, "#"))) // not needed
		// return v
		// return mockSpecialInterfaceType(t)
		return reflect.New(t).Elem()
	case reflect.Array:
		arr := reflect.New(t).Elem()
		for i := 0; i < arr.Len(); i++ {
			val := makeDefault(t.Elem(), append(path, strconv.FormatInt(int64(i), 10)), opts)
			if val.IsValid() {
				arr.Index(i).Set(val)
			}
		}
		return arr
	case reflect.Slice:
		// []byte. Elem is reflect.Uint8, but we cannot tell if is []byte, or []uint8
		// []byte compitable:
		if t.Elem().Kind() == ByteSliceType.Elem().Kind() {
			// empty slice
			return reflect.ValueOf([]byte(nil))
		}
		slice := reflect.New(t).Elem()
		val := makeDefault(t.Elem(), append(path, "[]"), opts)
		if val.IsValid() {
			return reflect.Append(slice, val)
		}
		return slice
	case reflect.Map:
		m := reflect.New(t).Elem()
		m.Set(reflect.MakeMapWithSize(t, 1)) // must make map, otherwise panic: assignment to entry in nil map
		valK := makeDefault(t.Key(), append(path, "$key"), opts)
		valV := makeDefault(t.Elem(), append(path, "$value"), opts)
		if valK.IsValid() && valV.IsValid() {
			m.SetMapIndex(valK, valV)
		}
		return m
	case reflect.Struct:
		v := reflect.New(t).Elem()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := t.Field(i).Name
			if strings.ToUpper(name[0:1]) != name[0:1] {
				// must be exported
				continue
			}
			val := makeDefault(field.Type, append(path, name), opts)
			if val.IsValid() {
				v.Field(i).Set(val)
			}
		}
		return v
	case reflect.Chan, reflect.Func:
		// ignore
		return reflect.Value{}
	default:
		return reflect.New(t).Elem()
	}
}
