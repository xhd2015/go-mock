package serialize

import (
	"encoding/json"
)

func Marshal(v interface{}) (data []byte, err error) {
	return json.Marshal(JSONSerialize(v))
}
