package serialize

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/xhd2015/go-mock/inspect/extension"
	"github.com/xhd2015/go-mock/inspect/typeinfo"
)

func Mock(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	return mockType(reflect.TypeOf(v), nil).Interface()
}
func MockType(t reflect.Type) interface{} {
	v := mockType(t, nil)
	if v.IsValid() {
		return v.Interface()
	}
	return nil
}

// mockType mock default value for a type.
func mockType(t reflect.Type, path []string) reflect.Value {
	defer func() {
		if len(path) == 0 {
			if e := recover(); e != nil {
				panic(fmt.Errorf("err:%v %v", strings.Join(path, "."), e))
			}
		}
	}()

	defaultValue, ok := extension.DefaultValue(t)
	if ok {
		return reflect.ValueOf(defaultValue)
	}

	kind := t.Kind()
	switch kind {
	case reflect.Ptr:
		p := reflect.New(t.Elem())
		p.Elem().Set(mockType(t.Elem(), append(path, "&")))
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
			arr.Index(i).Set(mockType(t.Elem(), append(path, strconv.FormatInt(int64(i), 10))))
		}
		return arr
	case reflect.Slice:
		// []byte. Elem is reflect.Uint8, but we cannot tell if is []byte, or []uint8
		// []byte compitable:
		if t.Elem().Kind() == typeinfo.ByteSliceType.Elem().Kind() {
			// empty slice
			return reflect.ValueOf([]byte(nil))
		}
		slice := reflect.New(t).Elem()
		return reflect.Append(slice, mockType(t.Elem(), append(path, "[]")))
	case reflect.Map:
		m := reflect.New(t).Elem()
		m.Set(reflect.MakeMapWithSize(t, 1)) // must make map, otherwise panic: assignment to entry in nil map
		m.SetMapIndex(mockType(t.Key(), append(path, "$key")), mockType(t.Elem(), append(path, "$value")))
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
			v.Field(i).Set(mockType(field.Type, append(path, name)))
		}
		return v
	case reflect.Chan:
		// ignore
		return reflect.New(t).Elem()
	default:
		return reflect.New(t).Elem()
	}
}

// func mockSpecialInterfaceType(t reflect.Type) reflect.Value {
// if t == types.BoolType {
// 	return reflect.ValueOf(types.NewBool(false))
// } else if t == reflect.TypeOf((*json.Number)(nil)).Elem() {
// 	return reflect.ValueOf(json.Number("0"))
// }
// return reflect.New(t).Elem()
// }
