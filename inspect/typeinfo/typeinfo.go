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
	Type() TypeDesc
}
type TypeDesc interface {
	Reflect() reflect.Type
}

type funcImpl struct {
	Xargs    FieldList `json:"Args"`
	Xresults FieldList `json:"Results"`
}

type fieldListImpl []TypeInfo

type typeImpl struct {
	Xname string       `json:"Name"`
	Xtype *reflectType `json:"Type"`
}

type reflectType struct {
	Type reflect.Type
}

var _ Func = ((*funcImpl)(nil))
var _ FieldList = ((*fieldListImpl)(nil))
var _ TypeInfo = ((*typeImpl)(nil))
var _ TypeDesc = ((*reflectType)(nil))
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
		Xargs:    fieldListImpl(args),
		Xresults: fieldListImpl(results),
	}
}
func (c *funcImpl) Args() FieldList {
	return c.Xargs
}
func (c *funcImpl) Results() FieldList {
	return c.Xresults
}
func (c fieldListImpl) Len() int {
	return len(c)
}
func (c fieldListImpl) Get(i int) TypeInfo {
	return c[i]
}

func (c *typeImpl) Name() string {
	return c.Xname
}
func (c *typeImpl) Type() TypeDesc {
	return c.Xtype
}
func (c *reflectType) Reflect() reflect.Type {
	return c.Type
}

func (c *reflectType) MarshalJSON() ([]byte, error) {
	schema := GenSchema(c.Type)
	return json.Marshal(schema)
}
