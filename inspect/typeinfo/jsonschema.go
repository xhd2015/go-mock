package typeinfo

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Version is the JSON Schema version.
// If extending JSON Schema with custom values use a custom URI.
// RFC draft-wright-json-schema-00, section 6
var Version = "http://json-schema.org/draft-04/schema#"

// Schema is the root schema.
// RFC draft-wright-json-schema-00, section 4.5
type Schema struct {
	*Type
	Definitions Definitions `json:"definitions,omitempty"`
}
type SchemaList []*Type // the first schema is the root schema

// Definitions hold schema definitions.
// http://json-schema.org/latest/json-schema-validation.html#rfc.section.5.26
// RFC draft-wright-json-schema-validation-00, section 5.26
type Definitions map[string]*Type

var (
	timeType = reflect.TypeOf(time.Time{}) // date-time RFC section 7.3.1
)

// Type represents a JSON Schema object type.
type Type struct {
	// URI extension by this package
	URI string `json:"uri,omitempty"` // serve as an ID, unique inside definitions
	// RFC draft-wright-json-schema-00
	Version string `json:"$schema,omitempty"` // section 6.1
	Ref     string `json:"$ref,omitempty"`    // section 7
	// RFC draft-wright-json-schema-validation-00, section 5
	MultipleOf           int              `json:"multipleOf,omitempty"`           // section 5.1
	Maximum              int              `json:"maximum,omitempty"`              // section 5.2
	ExclusiveMaximum     bool             `json:"exclusiveMaximum,omitempty"`     // section 5.3
	Minimum              int              `json:"minimum,omitempty"`              // section 5.4
	ExclusiveMinimum     bool             `json:"exclusiveMinimum,omitempty"`     // section 5.5
	MaxLength            int              `json:"maxLength,omitempty"`            // section 5.6
	MinLength            int              `json:"minLength,omitempty"`            // section 5.7
	Pattern              string           `json:"pattern,omitempty"`              // section 5.8
	AdditionalItems      *Type            `json:"additionalItems,omitempty"`      // section 5.9
	Items                *Type            `json:"items,omitempty"`                // section 5.9
	MaxItems             int              `json:"maxItems,omitempty"`             // section 5.10
	MinItems             int              `json:"minItems,omitempty"`             // section 5.11
	UniqueItems          bool             `json:"uniqueItems,omitempty"`          // section 5.12
	MaxProperties        int              `json:"maxProperties,omitempty"`        // section 5.13
	MinProperties        int              `json:"minProperties,omitempty"`        // section 5.14
	Required             []string         `json:"required,omitempty"`             // section 5.15
	Properties           *SortedMap       `json:"properties,omitempty"`           // section 5.16
	PatternProperties    map[string]*Type `json:"patternProperties,omitempty"`    // section 5.17
	AdditionalProperties json.RawMessage  `json:"additionalProperties,omitempty"` // section 5.18
	Dependencies         map[string]*Type `json:"dependencies,omitempty"`         // section 5.19
	Enum                 []interface{}    `json:"enum,omitempty"`                 // section 5.20
	Type                 string           `json:"type,omitempty"`                 // section 5.21
	AllOf                []*Type          `json:"allOf,omitempty"`                // section 5.22
	AnyOf                []*Type          `json:"anyOf,omitempty"`                // section 5.23
	OneOf                []*Type          `json:"oneOf,omitempty"`                // section 5.24
	Not                  *Type            `json:"not,omitempty"`                  // section 5.25
	Definitions          Definitions      `json:"definitions,omitempty"`          // section 5.26
	// RFC draft-wright-json-schema-validation-00, section 6, 7
	Title       string        `json:"title,omitempty"`       // section 6.1
	Description string        `json:"description,omitempty"` // section 6.1
	Default     interface{}   `json:"default,omitempty"`     // section 6.2
	Format      string        `json:"format,omitempty"`      // section 7
	Examples    []interface{} `json:"examples,omitempty"`    // section 7.4
	// RFC draft-handrews-json-schema-validation-02, section 9.4
	ReadOnly  bool `json:"readOnly,omitempty"`
	WriteOnly bool `json:"writeOnly,omitempty"`
	// RFC draft-wright-json-schema-hyperschema-00, section 4
	Media          *Type  `json:"media,omitempty"`          // section 4.3
	BinaryEncoding string `json:"binaryEncoding,omitempty"` // section 4.3

	Extras map[string]interface{} `json:"-"`
}

func GenSchema(t reflect.Type) *Schema {
	defs := make(map[reflect.Type]*Type)
	root := genSchema(t, defs, nil, nil)

	schemaDefs := make(Definitions, len(defs))
	for _, v := range defs {
		if v == root || v.URI == "" {
			continue
		}
		schemaDefs[v.URI] = v
	}
	return &Schema{
		Type:        root,
		Definitions: schemaDefs,
	}
}
func GenSchemaList(t reflect.Type) SchemaList {
	defs := make(map[reflect.Type]*Type)
	root := genSchema(t, defs, nil, nil)

	list := make(SchemaList,0,len(defs))
	list = append(list, root)

	for _, v := range defs {
		if v == root || v.URI == "" {
			continue
		}
		list = append(list, v)
	}
	return list
}

type GenSchemaOptions struct {
}

func genURI(t reflect.Type, defs map[reflect.Type]*Type) string {
	if t.PkgPath() != "" && t.Name() != "" {
		return t.PkgPath() + "." + t.Name()
	}
	return fmt.Sprintf("%d", len(defs))
}

func RefOrUse(t *Type) *Type {
	if t.URI == "" {
		// primitive types
		return t
	}
	return &Type{
		Ref: t.URI,
	}
}

func genSchema(t reflect.Type, defs map[reflect.Type]*Type, path []string, opts *GenSchemaOptions) (s *Type) {
	kind := t.Kind()

	// handle primitive type
	// TODO: what if named primitive type defines unmarhsal JSON?
	// without really unmarshal, we cannot know the static type,
	// so we cannot make any effore
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Type{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &Type{Type: "number"}
	case reflect.Bool:
		return &Type{Type: "boolean"}
	case reflect.String:
		return &Type{Type: "string"}
	case reflect.Struct:
		switch t {
		case timeType: // date-time RFC section 7.3.1
			return &Type{Type: "string", Format: "date-time"}
		}
	case reflect.Ptr:
		return genSchema(t.Elem(), defs, append(path, "&"), opts)
	}

	s = defs[t]
	if s != nil {
		return s
	}
	s = &Type{
		URI: genURI(t, defs),
	}
	defs[t] = s

	if len(path) > 1000 {
		panic(fmt.Errorf("genSchema possibly cyclic reference:%v... ", strings.Join(path[:10], ".")))
	}
	defer func() {
		if len(path) == 0 {
			if e := recover(); e != nil {
				panic(fmt.Errorf("genSchema err:%v %v", strings.Join(path, "."), e))
			}
		}
	}()

	// TODO: custom interceptor
	// defaultValue, ok := extension.DefaultValue(t)
	// if ok {
	// 	return reflect.ValueOf(defaultValue)
	// }

	switch kind {
	case reflect.Struct:
		s.Type = "object"
		s.Properties = NewSortedMap(t.NumField())
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.Anonymous {
				subType := genSchema(field.Type, defs, append(path, field.Name), opts)
				if subType.Type == "object" && subType.Properties != nil {
					// merge sorted map
					subType.Properties.Range(func(key string, val interface{}) bool {
						s.Properties.Set(key, val)
						return true
					})
				}
				continue
			}

			jsonName, _ := GetExportedJSONName(&field)
			if jsonName == "" {
				continue
			}
			subType := genSchema(field.Type, defs, append(path, field.Name), opts)
			s.Properties.Set(jsonName, RefOrUse(subType))
		}

	case reflect.Interface:
		// don't know how to do that
	case reflect.Array, reflect.Slice:
		s.Type = "array"
		if kind == reflect.Array {
			s.MinItems = t.Len()
			s.MaxItems = t.Len()
		}
		s.Items = RefOrUse(genSchema(t.Elem(), defs, append(path, "$elem"), opts))
	case reflect.Map:
		s.Type = "object"

		patternKey := ".*"
		switch t.Key().Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			patternKey = "^[0-9]+$"

			s.PatternProperties = map[string]*Type{
				"^[0-9]+$": RefOrUse(genSchema(t.Elem(), defs, append(path, "$key"), opts)),
			}
			s.AdditionalProperties = []byte("false")
		}
		s.PatternProperties = map[string]*Type{
			patternKey: RefOrUse(genSchema(t.Elem(), defs, append(path, "$key"), opts)),
		}
	case reflect.Chan:
		// ignore
	case reflect.Func:
		// ignore
	default:
		panic(fmt.Errorf("genSchema unhandled type:%v", t))
	}
	return s
}

// GetExportedJSONName get json name that will appear in marshaled json
func GetExportedJSONName(fieldType *reflect.StructField) (jsonName string, omitEmpty bool) {
	fieldName := fieldType.Name
	// / must be exported
	if strings.ToUpper(fieldName[0:1]) != fieldName[0:1] {
		return
	}
	jsonTag := fieldType.Tag.Get("json")
	idx := strings.Index(jsonTag, ",")
	jsonName = jsonTag
	if idx >= 0 {
		jsonName = jsonTag[:idx]
		jsonOpts := strings.Split(jsonTag[idx+1:], ",")
		for _, opt := range jsonOpts {
			if opt == "omitempty" {
				omitEmpty = true
				break
			}
		}
	}
	if jsonName == "-" {
		jsonName = ""
		return // ignored
	}
	// omit empty
	if jsonName == "" {
		jsonName = fieldName
	}
	return
}

func IsZero(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	if v.IsZero() {
		return true
	}
	switch v.Kind() {
	case reflect.Map:
		return v.Len() == 0
	case reflect.Slice:
		return v.Len() == 0
	}
	return false
}
