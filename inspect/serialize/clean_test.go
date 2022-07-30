package serialize

import (
	"encoding/json"
	"fmt"
	"testing"
)

// go test -run TestGeneralizeStringify -v ./support/mock
func TestGeneralizeStringify(t *testing.T) {
	x := map[string]interface{}{
		"11": &tGeneralizeStringify{a: 1234},
	}
	x2 := GeneralizeStringify(x)

	t.Logf("x2:%T", x2)
	t.Logf("x2:%+v", x2)

	x2T := fmt.Sprintf("%T", x2)
	if x2T != "map[string]interface {}" {
		t.FailNow()
	}
	x2V := fmt.Sprintf("%+v", x2)
	if x2V != "map[11:ahaha(1234)]" {
		t.FailNow()
	}
}

type tGeneralizeStringify struct {
	a int
}

func (c *tGeneralizeStringify) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("ahaha(%v)", c.a))
}
