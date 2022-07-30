package extension

import (
	"encoding/json"
)

type JSON []byte

func (c *JSON) UnmarshalJSON(data []byte) error {
	*c = JSON(data)
	return nil
}
func (c JSON) MustUnmarshal(v interface{}) {
	err := json.Unmarshal([]byte(c), v)
	if err != nil {
		panic(err)
	}
}

type AnyJSON interface {
	//
	GetJSON() ([]byte, error)
	// plain string
	GetString() (str string, ok bool)
	Copy(dst interface{}) error
}
