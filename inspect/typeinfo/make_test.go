package typeinfo

import (
	"encoding/json"
	"reflect"
	"testing"
)

// go test -run TestT -v ./support/xgo/inspect/typeinfo
func TestT(t *testing.T) {
	var x func() = func() {}
	val := MakeDefault(reflect.TypeOf(x), nil)

	_, err := json.Marshal(val)
	if err != nil {
		t.Fatal(err)
	}

	_, err = json.Marshal(x)
	if err == nil {
		t.Fatal("expect err")
	}
	t.Logf("x err:%v", err)

	// struct

	{
		var x2 struct {
			F func()
		}
		_, err = json.Marshal(x2)
		if err == nil {
			t.Fatal("expect err")
		}
		t.Logf("x2 err:%v", err)
	}
}
