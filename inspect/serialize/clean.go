package serialize

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/xhd2015/go-mock/inspect/typeinfo"
)

const _SKIP_MOCK = true // don't mock me

// CleanSerializable convert v of go type
// into general json type: interface{}, map[string]interface{},[]interface{}, etc.
// clean is for outgoing message, such as http response, where a go value will
// be exported to external world.
func CleanSerializable(v interface{}) interface{} {
	return doClean(reflect.ValueOf(v), nil, &cleanOpts{})
}

func JSONSerialize(v interface{}) interface{} {
	return doClean(reflect.ValueOf(v), nil, &cleanOpts{
		byteSliceGuessJSON: true,
		respectUnmarshaler: true,
		structUseSortedMap: true,
		// noUnmarshalable:true,
	})
}

func GeneralizeStringify(v interface{}) interface{} {
	return doClean(reflect.ValueOf(v), nil, &cleanOpts{
		byteSliceGuessJSON:     true,
		respectUnmarshaler:     true,
		structUseSortedMap:     false,
		stringifyJSONMarshaler: true,
	})
}

// Generalize like JSONSerialize,
// but use `map[string]interface{}` instead of `SortedMap`
// for struct. It's primarily for internal use, not to
// export value to outside world.
func Generalize(v interface{}) interface{} {
	return doClean(reflect.ValueOf(v), nil, &cleanOpts{
		byteSliceGuessJSON: true,
		respectUnmarshaler: true,
		structUseSortedMap: false,
	})
}

type cleanOpts struct {
	byteSliceGuessJSON     bool
	respectUnmarshaler     bool
	structUseSortedMap     bool
	stringifyJSONMarshaler bool
	// noUnmarshalable        bool // exclude Func,Chan, map[interface{}]...
}

func doClean(v reflect.Value, path []string, opts *cleanOpts) interface{} {
	if len(path) > 50 {
		panic(fmt.Errorf("CleanSerializable possibly cyclic reference:%v... ", strings.Join(path[:10], ".")))
	}
	defer func() {
		if len(path) == 0 {
			if e := recover(); e != nil {
				panic(fmt.Errorf("CleanSerializable err:%v %v", strings.Join(path, "."), e))
			}
		}
	}()
	if !v.IsValid() {
		return nil
	}
	// implements json.Marshaler
	if opts.respectUnmarshaler {
		jsonVal := GetJSONMarshaler(v)
		if jsonVal.IsValid() {
			if opts.stringifyJSONMarshaler {
				data, err := jsonVal.Interface().(json.Marshaler).MarshalJSON()
				if err != nil {
					panic(err)
				}
				var x interface{}
				err = json.Unmarshal(data, &x)
				if err != nil {
					panic(err)
				}
				return x
			}
			return jsonVal.Interface()
		}
	}

	kind := v.Kind()
	if opts.byteSliceGuessJSON {
		var bytes []byte
		var ok bool
		if v.Type() == typeinfo.ByteSliceType {
			if v.IsNil() {
				return nil
			}
			bytes, ok = v.Interface().([]byte)
		} else if v.Type() == typeinfo.StringType {
			var s string
			s, ok = v.Interface().(string)
			if ok {
				bytes = []byte(s)
			}
		}
		if ok && len(bytes) >= 2 {
			// try json
			if (bytes[0] == '{' && bytes[len(bytes)-1] == '}') || bytes[0] == '[' && bytes[len(bytes)-1] == ']' {
				var m interface{}
				err := json.Unmarshal(bytes, &m)
				if err == nil {
					// if ok, return m
					return m
				}
			}
		}
	}
	switch kind {
	case reflect.Ptr:
		if v.IsNil() {
			return nil
		}
		e := doClean(v.Elem(), append(path, "&"), opts)
		return &e
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		e := doClean(v.Elem(), append(path, "@"), opts)
		return e
	case reflect.Array, reflect.Slice:
		arr := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			e := doClean(v.Index(i), append(path, strconv.FormatInt(int64(i), 10)), opts)
			arr[i] = e
		}
		return arr
	case reflect.Map:
		m := make(map[string]interface{}, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := doClean(iter.Key(), append(path, "$key"), opts)
			val := doClean(iter.Value(), append(path, "$value"), opts)
			m[fmt.Sprint(key)] = val
		}
		return m
	case reflect.Struct:
		var sortedMap *typeinfo.SortedMap
		var mp map[string]interface{}
		if opts.structUseSortedMap {
			sortedMap = typeinfo.NewSortedMap(v.NumField())
		} else {
			mp = make(map[string]interface{}, v.NumField())
		}

		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := v.Type().Field(i)
			if fieldType.Anonymous {
				// unwrap indrect
				if field.Kind() == reflect.Ptr {
					if field.IsNil() {
						continue
					}
					field = field.Elem()
				}
				res := doClean(field, path, opts)
				if opts.structUseSortedMap {
					innerObj, ok := res.(*typeinfo.SortedMap)
					if ok {
						innerObj.Range(func(key string, val interface{}) bool {
							sortedMap.Set(key, val)
							return true
						})
					}
				} else {
					innerObj, ok := res.(map[string]interface{})
					if ok {
						for k, v := range innerObj {
							mp[k] = v
						}
					}
				}
				continue
			}
			jsonName, omitEmpty := typeinfo.GetExportedJSONName(&fieldType)
			if jsonName == "" {
				continue
			}
			if omitEmpty && typeinfo.IsZero(field) {
				continue
			}

			res := doClean(field, append(path, fieldType.Name), opts)
			if opts.structUseSortedMap {
				sortedMap.Set(jsonName, res)
			} else {
				mp[jsonName] = res
			}
		}
		if opts.structUseSortedMap {
			return sortedMap
		} else {
			return mp
		}
	case reflect.Chan, reflect.Func:
		// ignore
		return nil
	default:
		i := v.Interface()
		if i64, ok := i.(int64); ok {
			// i64 as string
			return i64
		}
		return i
	}
}

func GetJSONMarshaler(v reflect.Value) reflect.Value {
	var jsonVal reflect.Value
	if v.Type().Implements(typeinfo.JSONMarshaler) {
		jsonVal = v
	} else if reflect.PtrTo(v.Type()).Implements(typeinfo.JSONMarshaler) {
		if v.CanAddr() {
			jsonVal = v.Addr()
		} else {
			jsonVal = reflect.New(v.Type())
			jsonVal.Elem().Set(v)
		}
	}
	return jsonVal
}
