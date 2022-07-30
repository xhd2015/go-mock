package serialize

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/xhd2015/go-mock/inspect/extension"
)

// go test -run TestUnmarshalSimple -v ./support/mock
func TestUnmarshalSimple(t *testing.T) {
	type I struct {
		Name string
	}
	var v struct {
		A map[string]*I
		B []I
	}
	err := Unmarshal([]byte(`{"A":{"ha":{"Name":"A"}},"B":[{"Name":"B"}]}`), &v)

	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", v)

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", string(bytes))

}

// go test -run TestUnmarshalCustom -v ./support/mock
func TestUnmarshalCustom(t *testing.T) {
	type Custom struct {
		Str string
	}
	extension.ParseValueFromJSON = func(jsonValue extension.AnyJSON, v interface{}) (err error, ok bool) {
		if v, cok := v.(*Custom); cok {
			str, strOK := jsonValue.GetString()
			if strOK {
				ok = true
				v.Str = str
				return
			}
			err = fmt.Errorf("must be string")
			return
		}
		return
	}
	var v struct {
		A Custom
	}
	err := Unmarshal([]byte(`{"A":"my custom"}`), &v)

	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", v)

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", string(bytes))
}

// go test -run TestUnmarshalOmitEmpty -v ./support/mock
func TestUnmarshalOmitEmpty(t *testing.T) {
	var v struct {
		A map[string]string `json:"A,omitempty"`
	}
	err := Unmarshal([]byte(`{"A":{"ok":"my custom"}}`), &v)

	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", v)

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", string(bytes))
	expectBytes := `{"A":{"ok":"my custom"}}`
	if string(bytes) != expectBytes {
		t.Fatalf("expect %s = %+v, actual:%+v", `A`, expectBytes, string(bytes))
	}
}

// go test -run TestJSONSerializeGuessBytes -v ./support/mock
func TestJSONSerializeGuessBytes(t *testing.T) {
	var v = struct {
		A []byte
		B string
		C string
	}{
		A: []byte("{\"A\":\"1\"}"),
		B: "[{\"B\":\"2\"}]",
		C: "haha}",
	}
	c := JSONSerialize(v)
	t.Logf("%+v", c)

	bytes, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("JSON = %s", string(bytes))
	expectBytes := `{"A":{"A":"1"},"B":[{"B":"2"}],"C":"haha}"}`
	if string(bytes) != expectBytes {
		t.Fatalf("expect %s = %+v, actual:%+v", `c`, expectBytes, string(bytes))
	}
}

// go test -run TestMarshalLargeInt64 -v ./support/mock
func TestMarshalLargeInt64(t *testing.T) {
	i123b, err := Marshal(int64(123))
	if err != nil {
		t.Fatal(err)
	}

	i123 := string(i123b)
	expecti123 := `123`
	if i123 != expecti123 {
		t.Fatalf("expect %s = %+v, actual:%+v", `i123`, expecti123, i123)
	}

	iLargeb, err := Marshal(int64(999999999999999999))
	if err != nil {
		t.Fatal(err)
	}

	iLarge := string(iLargeb)
	expectiLarge := `999999999999999999`
	if iLarge != expectiLarge {
		t.Fatalf("expect %s = %+v, actual:%+v", `iLarge`, expectiLarge, iLarge)
	}
}
