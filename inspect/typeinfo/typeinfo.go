package typeinfo

import (
	"encoding/json"
	"reflect"
)

const _SKIP_MOCK = true

type Func interface {
	Args() FieldList
	Results() FieldList
}

type FieldList interface {
	Len() int
	Get(i int) TypeInfo
}

type TypeInfo interface {
	Name() string
	Type() reflect.Type
}

type funcImpl struct {
	Xargs    FieldList `json:"Args"`
	Xresults FieldList `json:"Results"`
}

type fieldListImpl struct {
	Types []TypeInfo `json:"Types"`
}

type typeImpl struct {
	Xname string       `json:"Name"`
	Xtype *reflectType `json:"Type"`
}

type reflectType struct {
	reflect.Type
}

var _ Func = ((*funcImpl)(nil))
var _ FieldList = ((*fieldListImpl)(nil))
var _ TypeInfo = ((*typeImpl)(nil))
var _ json.Marshaler = ((*reflectType)(nil))

func NewTypeInfo(name string, rtype reflect.Type) TypeInfo {
	return &typeImpl{
		Xname: name,
		Xtype: &reflectType{
			Type: rtype,
		},
	}
}
func NewFunc(args []TypeInfo, results []TypeInfo) Func {
	return &funcImpl{
		Xargs: &fieldListImpl{
			Types: args,
		},
		Xresults: &fieldListImpl{
			Types: results,
		},
	}
}
func (c *funcImpl) Args() FieldList {
	return c.Xargs
}
func (c *funcImpl) Results() FieldList {
	return c.Xresults
}
func (c *fieldListImpl) Len() int {
	return len(c.Types)
}
func (c *fieldListImpl) Get(i int) TypeInfo {
	return c.Types[i]
}

func (c *typeImpl) Name() string {
	return c.Xname
}
func (c *typeImpl) Type() reflect.Type {
	return c.Xtype
}

func (c *reflectType) MarshalJSON() ([]byte, error) {
	schema := GenSchema(c.Type)
	return json.Marshal(schema)
}
