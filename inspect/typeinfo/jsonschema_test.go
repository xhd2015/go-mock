package typeinfo

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type GrandfatherType struct {
	FamilyName string `json:"family_name" jsonschema:"required"`
}

type SomeBaseType struct {
	SomeBaseProperty     int `json:"some_base_property"`
	SomeBasePropertyYaml int `yaml:"some_base_property_yaml"`
	// The jsonschema required tag is nonsensical for private and ignored properties.
	// Their presence here tests that the fields *will not* be required in the output
	// schema, even if they are tagged required.
	somePrivateBaseProperty   string          `jsonschema:"required"`
	SomeIgnoredBaseProperty   string          `json:"-" jsonschema:"required"`
	SomeSchemaIgnoredProperty string          `jsonschema:"-,required"`
	Grandfather               GrandfatherType `json:"grand"`

	SomeUntaggedBaseProperty           bool `jsonschema:"required"`
	someUnexportedUntaggedBaseProperty bool
}

type MapType map[string]interface{}

type nonExported struct {
	PublicNonExported  int
	privateNonExported int
}

type ProtoEnum int32

func (ProtoEnum) EnumDescriptor() ([]byte, []int) { return []byte(nil), []int{0} }

const (
	Unset ProtoEnum = iota
	Great
)

type Bytes []byte
type TestUser struct {
	SomeBaseType
	nonExported
	MapType

	ID       int                    `json:"id" jsonschema:"required"`
	Name     string                 `json:"name" jsonschema:"required,minLength=1,maxLength=20,pattern=.*,description=this is a property,title=the name,example=joe,example=lucy,default=alex,readOnly=true"`
	Password string                 `json:"password" jsonschema:"writeOnly=true"`
	Friends  []int                  `json:"friends,omitempty" jsonschema_description:"list of IDs, omitted when empty"`
	Tags     map[string]interface{} `json:"tags,omitempty"`

	TestFlag       bool
	IgnoredCounter int `json:"-"`

	// Tests for RFC draft-wright-json-schema-validation-00, section 7.3
	BirthDate time.Time `json:"birth_date,omitempty"`
	// Website   url.URL   `json:"website,omitempty"`
	// IPAddress net.IP    `json:"network_address,omitempty"`

	// Tests for RFC draft-wright-json-schema-hyperschema-00, section 4
	Photo  []byte `json:"photo,omitempty" jsonschema:"required"`
	Photo2 Bytes  `json:"photo2,omitempty" jsonschema:"required"`

	// Tests for jsonpb enum support
	Feeling ProtoEnum `json:"feeling,omitempty"`
	Age     int       `json:"age" jsonschema:"minimum=18,maximum=120,exclusiveMaximum=true,exclusiveMinimum=true"`
	Email   string    `json:"email" jsonschema:"format=email"`

	// Test for "extras" support
	Baz string `jsonschema_extras:"foo=bar,hello=world,foo=bar1"`

	// Tests for simple enum tags
	Color      string  `json:"color" jsonschema:"enum=red,enum=green,enum=blue"`
	Rank       int     `json:"rank,omitempty" jsonschema:"enum=1,enum=2,enum=3"`
	Multiplier float64 `json:"mult,omitempty" jsonschema:"enum=1.0,enum=1.5,enum=2.0"`

	// Tests for enum tags on slices
	Roles      []string  `json:"roles" jsonschema:"enum=admin,enum=moderator,enum=user"`
	Priorities []int     `json:"priorities,omitempty" jsonschema:"enum=-1,enum=0,enum=1,enun=2"`
	Offsets    []float64 `json:"offsets,omitempty" jsonschema:"enum=1.570796,enum=3.141592,enum=6.283185"`

	// Test for raw JSON
	Raw json.RawMessage `json:"raw"`
}

// go test -run TestSchemaGeneration -v ./support/xgo/inspect/typeinfo
func TestSchemaGeneration(t *testing.T) {
	tests := []struct {
		typ       interface{}
		reflector *GenSchemaOptions
		fixture   string
	}{
		{&TestUser{}, &GenSchemaOptions{}, "fixtures/defaults.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/allow_additional_props.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/required_from_jsontags.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/defaults_expanded_toplevel.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/ignore_type.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/no_reference.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/fully_qualified.json"},
		// {&TestUser{}, &GenSchemaOptions{}, "fixtures/no_ref_qual_types.json"},
	}

	for _, tt := range tests {
		name := strings.TrimSuffix(filepath.Base(tt.fixture), ".json")
		t.Run(name, func(t *testing.T) {
			f, err := ioutil.ReadFile(tt.fixture)
			if err != nil {
				t.Fatal(err)
			}

			actualSchema := GenSchema(reflect.TypeOf(tt.typ))
			expectedSchema := &Schema{}

			err = json.Unmarshal(f, expectedSchema)
			if err != nil {
				t.Fatal(err)
			}

			expectedJSON := f

			// expectedJSON, _ := json.MarshalIndent(expectedSchema, "", "  ")
			actualJSON, _ := json.MarshalIndent(actualSchema, "", "  ")
			expectedJSONStr := string(expectedJSON)
			actualJSONStr := string(actualJSON)
			if expectedJSONStr != actualJSONStr {
				err := ioutil.WriteFile(strings.TrimSuffix(tt.fixture, ".json")+".expect.json", expectedJSON, 0777)
				ioutil.WriteFile(strings.TrimSuffix(tt.fixture, ".json")+".actual.json", actualJSON, 0777)
				if err != nil {
					t.Fatal(err)
				}
				if len(expectedJSONStr) != len(actualJSONStr) {
					t.Errorf("len:%v %v vs %v", len(f), len(expectedJSONStr), len(actualJSONStr))
				}
				// t.Fatalf("expect: %v, actual: %v", string(expectedJSON), string(actualJSON))
			}
		})
	}
}
