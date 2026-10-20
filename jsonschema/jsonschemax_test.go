// Copyright 2026 The JSON Schema Go Project Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package jsonschema_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RussellLuo/jsonschema-go/jsonschema"
	"github.com/google/go-cmp/cmp"
)

func taggedType(typ reflect.Type, tag string) reflect.Type {
	return reflect.StructOf([]reflect.StructField{{
		Name: "Value", Type: typ,
		Tag: reflect.StructTag(`json:"value" jsonschema:"field description" jsonschemax:` + strconv.Quote(tag)),
	}})
}

func validateJSON(t *testing.T, s *jsonschema.Schema, valid, invalid []string) {
	t.Helper()
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range []struct {
		values []string
		valid  bool
	}{{valid, true}, {invalid, false}} {
		for _, input := range group.values {
			var value any
			dec := json.NewDecoder(strings.NewReader(input))
			dec.UseNumber()
			if err := dec.Decode(&value); err != nil {
				t.Fatal(err)
			}
			value = nativeNumbers(t, value)
			if err := r.Validate(value); (err == nil) != group.valid {
				t.Errorf("Validate(%s) = %v, want valid=%v", input, err, group.valid)
			}
		}
	}
}

// Validate expects native Go numeric values (json.Number is treated as a
// string by upstream type validation). Preserve integer precision when
// turning JSON fixtures into the Go values accepted by that API.
func nativeNumbers(t *testing.T, value any) any {
	t.Helper()
	switch v := value.(type) {
	case json.Number:
		if n, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return n
		}
		if n, err := strconv.ParseUint(string(v), 10, 64); err == nil {
			return n
		}
		n, err := v.Float64()
		if err != nil {
			t.Fatal(err)
		}
		return n
	case []any:
		for i := range v {
			v[i] = nativeNumbers(t, v[i])
		}
	case map[string]any:
		for k := range v {
			v[k] = nativeNumbers(t, v[k])
		}
	}
	return value
}

func TestConstraints(t *testing.T) {
	for _, test := range []struct {
		name           string
		typ            reflect.Type
		tag            string
		valid, invalid []string
	}{
		{
			name:    "numeric",
			typ:     reflect.TypeFor[float64](),
			tag:     " minimum = 1 ,maximum=10 ",
			valid:   []string{`{"value":1}`, `{"value":10}`, `{"value":1.5}`, `{"value":1.2}`},
			invalid: []string{`{"value":0.5}`, `{"value":10.5}`},
		},
		{
			name:    "exclusive",
			typ:     reflect.TypeFor[float64](),
			tag:     "exclusiveMinimum=0, exclusiveMaximum=1",
			valid:   []string{`{"value":0.5}`},
			invalid: []string{`{"value":0}`, `{"value":1}`},
		},
		{
			name:    "zero bounds with large exponents",
			typ:     reflect.TypeFor[float64](),
			tag:     "minimum=0e-9999999999,maximum=-0e9999999999",
			valid:   []string{`{"value":0}`},
			invalid: []string{`{"value":-5e-324}`, `{"value":5e-324}`},
		},
		{
			name:    "subnormal bound",
			typ:     reflect.TypeFor[float64](),
			tag:     "minimum=5e-324",
			valid:   []string{`{"value":5e-324}`, `{"value":1e-323}`},
			invalid: []string{`{"value":0}`, `{"value":-5e-324}`},
		},
		{
			name:    "unicode",
			typ:     reflect.TypeFor[string](),
			tag:     "minLength=1,maxLength=2",
			valid:   []string{`{"value":"你"}`, `{"value":"你好"}`},
			invalid: []string{`{"value":""}`, `{"value":"你好啊"}`},
		},
		{
			name:    "pattern",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern="^[a-z]{1,3}$"`,
			valid:   []string{`{"value":"abc"}`},
			invalid: []string{`{"value":"abcd"}`, `{"value":"ABC"}`},
		},
		{
			name:    "single quoted pattern with following constraint",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern='^[a-z]{1,3}$', minLength=2`,
			valid:   []string{`{"value":"ab"}`, `{"value":"abc"}`},
			invalid: []string{`{"value":"a"}`, `{"value":"abcd"}`, `{"value":"ABC"}`},
		},
		{
			name:    "single quoted pattern whitespace",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern=' a,b '`,
			valid:   []string{`{"value":" a,b "}`},
			invalid: []string{`{"value":"a,b"}`},
		},
		{
			name:    "single quoted pattern quotes",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern='^it\'s,"ok"$'`,
			valid:   []string{`{"value":"it's,\"ok\""}`},
			invalid: []string{`{"value":"its,\"ok\""}`, `{"value":"it's,ok"}`},
		},
		{
			name:    "single quoted pattern escapes",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern='^\\d{1,3}$'`,
			valid:   []string{`{"value":"123"}`},
			invalid: []string{`{"value":"abc"}`, `{"value":"1234"}`},
		},
		{
			name:    "pattern matching leading quote",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern="'abc"`,
			valid:   []string{`{"value":"'abc"}`},
			invalid: []string{`{"value":"abc"}`},
		},
		{
			name:    "raw pattern",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern=^[}\[]+$`,
			valid:   []string{`{"value":"[}"}`},
			invalid: []string{`{"value":"a"}`},
		},
		{
			name:    "raw pattern quotes",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern=^it's$`,
			valid:   []string{`{"value":"it's"}`},
			invalid: []string{`{"value":"its"}`},
		},
		{
			name:    "pattern whitespace",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern="^ a,b $"`,
			valid:   []string{`{"value":" a,b "}`},
			invalid: []string{`{"value":"a,b"}`},
		},
		{
			name:    "pattern escapes",
			typ:     reflect.TypeFor[string](),
			tag:     `pattern="^\\d{1,3}$"`,
			valid:   []string{`{"value":"123"}`},
			invalid: []string{`{"value":"abc"}`},
		},
		{
			name:    "array",
			typ:     reflect.TypeFor[[]string](),
			tag:     "minItems=1,maxItems=2,uniqueItems=true",
			valid:   []string{`{"value":["a"]}`, `{"value":["a","b"]}`, `{"value":null}`},
			invalid: []string{`{"value":[]}`, `{"value":["a","a"]}`, `{"value":["a","b","c"]}`},
		},
		{
			name:  "false unique",
			typ:   reflect.TypeFor[[]int](),
			tag:   "uniqueItems=false",
			valid: []string{`{"value":[1,1]}`},
		},
		{
			name:    "nullable pointer",
			typ:     reflect.TypeFor[*string](),
			tag:     "minLength=1",
			valid:   []string{`{"value":null}`, `{"value":"a"}`},
			invalid: []string{`{"value":""}`, `{}`},
		},
		{
			name:    "uint8 baseline",
			typ:     reflect.TypeFor[uint8](),
			tag:     "maximum=1000",
			valid:   []string{`{"value":255}`},
			invalid: []string{`{"value":256}`, `{"value":-1}`},
		},
		{
			name:    "int8 baseline",
			typ:     reflect.TypeFor[int8](),
			tag:     "minimum=-1000",
			valid:   []string{`{"value":-128}`},
			invalid: []string{`{"value":-129}`, `{"value":128}`},
		},
		{
			name:    "fixed array baseline",
			typ:     reflect.TypeFor[[2]int](),
			tag:     "minItems=1,maxItems=3",
			valid:   []string{`{"value":[1,2]}`},
			invalid: []string{`{"value":[1]}`, `{"value":[1,2,3]}`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := jsonschema.ForType(taggedType(test.typ, test.tag), nil)
			if err != nil {
				t.Fatal(err)
			}
			property := s.Properties["value"]
			if len(property.AllOf) != 1 || property.Description != "field description" {
				t.Fatalf("lost constraint or description: %+v", property)
			}
			validateJSON(t, s, test.valid, test.invalid)
		})
	}
}

func TestEnums(t *testing.T) {
	for _, test := range []struct {
		name           string
		typ            reflect.Type
		tag            string
		valid, invalid []string
	}{
		{
			name:    "strings",
			typ:     reflect.TypeFor[string](),
			tag:     `enum=['dev',"prod"], minLength=3`,
			valid:   []string{`{"value":"dev"}`, `{"value":"prod"}`},
			invalid: []string{`{"value":"test"}`, `{"value":null}`},
		},
		{
			name: "escapes",
			typ:  reflect.TypeFor[string](),
			tag:  `enum=['a,b','it\'s','say "hi"','a\\b','a\nb','\u4f60',"double\"quote"]`,
			valid: []string{
				`{"value":"a,b"}`,
				`{"value":"it's"}`,
				`{"value":"say \"hi\""}`,
				`{"value":"a\\b"}`,
				`{"value":"a\nb"}`,
				`{"value":"你"}`,
				`{"value":"double\"quote"}`,
			},
			invalid: []string{`{"value":"a"}`},
		},
		{
			name:    "integers",
			typ:     reflect.TypeFor[int64](),
			tag:     "enum=[1,2.0,3e0]",
			valid:   []string{`{"value":1}`, `{"value":2}`, `{"value":3}`},
			invalid: []string{`{"value":1.5}`, `{"value":"1"}`, `{"value":4}`},
		},
		{
			name:    "precise integers",
			typ:     reflect.TypeFor[uint64](),
			tag:     "enum=[9007199254740993,18446744073709551615]",
			valid:   []string{`{"value":9007199254740993}`, `{"value":18446744073709551615}`},
			invalid: []string{`{"value":9007199254740992}`, `{"value":18446744073709551614}`},
		},
		{
			name:    "numbers",
			typ:     reflect.TypeFor[float64](),
			tag:     "enum=[0.5,1.25]",
			valid:   []string{`{"value":0.5}`},
			invalid: []string{`{"value":1}`},
		},
		{
			name:    "decimals",
			typ:     reflect.TypeFor[float64](),
			tag:     "enum=[0.1,0.2,0.3]",
			valid:   []string{`{"value":0.1}`, `{"value":0.2}`, `{"value":0.3}`},
			invalid: []string{`{"value":0.4}`, `{"value":"0.1"}`},
		},
		{
			name:    "scientific notation",
			typ:     reflect.TypeFor[float64](),
			tag:     "enum=[1e-1,2.3e-2]",
			valid:   []string{`{"value":0.1}`, `{"value":0.023}`},
			invalid: []string{`{"value":0.23}`},
		},
		{
			name:    "booleans",
			typ:     reflect.TypeFor[bool](),
			tag:     "enum=[true]",
			valid:   []string{`{"value":true}`},
			invalid: []string{`{"value":false}`, `{"value":"true"}`},
		},
		{
			name:    "nullable included",
			typ:     reflect.TypeFor[*string](),
			tag:     "enum=['dev',null]",
			valid:   []string{`{"value":"dev"}`, `{"value":null}`},
			invalid: []string{`{"value":"prod"}`},
		},
		{
			name:    "nullable excluded",
			typ:     reflect.TypeFor[*string](),
			tag:     "enum=['dev']",
			valid:   []string{`{"value":"dev"}`},
			invalid: []string{`{"value":null}`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, err := jsonschema.ForType(taggedType(test.typ, test.tag), nil)
			if err != nil {
				t.Fatal(err)
			}
			validateJSON(t, s, test.valid, test.invalid)
			// The emitted schema must remain ordinary JSON Schema, with exact
			// integer tokens and no extension-specific syntax.
			encoded, err := json.Marshal(s)
			if err != nil || !json.Valid(encoded) {
				t.Fatalf("Marshal = %s, %v", encoded, err)
			}
			for _, v := range s.Properties["value"].AllOf[0].Enum {
				if n, ok := v.(json.Number); ok && !strings.Contains(string(encoded), string(n)) {
					t.Fatalf("lost numeric precision: %s", encoded)
				}
			}
		})
	}
}

func TestEnumNativeValues(t *testing.T) {
	type Config struct {
		Ratio float64 `json:"ratio" jsonschemax:"enum=[0.1,2e-1]"`
		ID    uint64  `json:"id" jsonschemax:"enum=[9007199254740993,18446744073709551615]"`
	}
	s, err := jsonschema.For[Config](nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value Config
		valid bool
	}{
		{Config{Ratio: 0.1, ID: 9007199254740993}, true},
		{Config{Ratio: 0.2, ID: 18446744073709551615}, true},
		{Config{Ratio: 0.3, ID: 9007199254740993}, false},
		{Config{Ratio: 0.1, ID: 9007199254740992}, false},
	} {
		// Validate objects as maps while retaining the native field values,
		// especially uint64 integers that would lose precision through float64.
		value := map[string]any{"ratio": test.value.Ratio, "id": test.value.ID}
		if err := r.Validate(value); (err == nil) != test.valid {
			t.Errorf("Validate(%+v) = %v, want valid=%v", test.value, err, test.valid)
		}
	}
}

func TestEnumJSONDecoding(t *testing.T) {
	type Config struct {
		Ratio float64 `json:"ratio" jsonschemax:"enum=[0.1,0.2,3e-1]"`
	}
	s, err := jsonschema.For[Config](nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		input string
		valid bool
	}{
		{`{"ratio":0.1}`, true},
		{`{"ratio":0.2}`, true},
		{`{"ratio":0.3}`, true},
		{`{"ratio":0.4}`, false},
	} {
		var value any
		if err := json.Unmarshal([]byte(test.input), &value); err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(value); (err == nil) != test.valid {
			t.Errorf("Validate(%s) = %v, want valid=%v", test.input, err, test.valid)
		}
	}
}

func TestTagErrors(t *testing.T) {
	for _, test := range []struct {
		typ     reflect.Type
		tag     string
		wantErr string
	}{
		{reflect.TypeFor[string](), "", "expected nonempty key=value"},
		{reflect.TypeFor[string](), " ", "expected nonempty key=value"},
		{reflect.TypeFor[string](), "minLength", "expected nonempty key=value"},
		{reflect.TypeFor[string](), "=1", "expected nonempty key=value"},
		{reflect.TypeFor[string](), "minLength=", "expected nonempty key=value"},
		{reflect.TypeFor[string](), "minLength=1,", "expected nonempty key=value"},
		{reflect.TypeFor[string](), ",minLength=1", "expected nonempty key=value"},
		{reflect.TypeFor[string](), "minLength=1,,maxLength=2", "expected nonempty key=value"},
		{reflect.TypeFor[string](), "MinLength=1", "unknown key"},
		{reflect.TypeFor[string](), "format=email", "unknown key"},
		{reflect.TypeFor[string](), "minLength=1,minLength=2", "duplicate key"},
		{reflect.TypeFor[string](), "minLength=-1", "expected nonnegative integer"},
		{reflect.TypeFor[string](), "minLength=1.5", "expected nonnegative integer"},
		{reflect.TypeFor[string](), "minLength=99999999999999999999999", "expected nonnegative integer"},
		{reflect.TypeFor[string](), "minLength=3,maxLength=2", "conflicting length bounds"},
		{reflect.TypeFor[string](), "minimum=1", "constraint requires explicit schema type"},
		{reflect.TypeFor[int](), "minimum=1,minLength=1", "constraint requires explicit schema type"},
		{reflect.TypeFor[float64](), "minimum=NaN", "expected finite number"},
		{reflect.TypeFor[float64](), "maximum=Inf", "expected finite number"},
		{reflect.TypeFor[float64](), "maximum=-Inf", "expected finite number"},
		{reflect.TypeFor[float64](), "maximum=1e999", "expected finite number"},
		{reflect.TypeFor[float64](), "minimum=1e-9999", "nonzero number underflows float64"},
		{reflect.TypeFor[float64](), "maximum=-1e-9999", "nonzero number underflows float64"},
		{reflect.TypeFor[float64](), "exclusiveMinimum=1e-9999", "nonzero number underflows float64"},
		{reflect.TypeFor[float64](), "exclusiveMaximum=-1e-9999", "nonzero number underflows float64"},
		{reflect.TypeFor[float64](), "minimum=1e-9999999999", "nonzero number underflows float64"},
		{reflect.TypeFor[int](), "minimum=oops", "expected finite number"},
		{reflect.TypeFor[int](), "multipleOf=2", "unknown key"},
		{reflect.TypeFor[map[string]int](), "minProperties=1", "unknown key"},
		{reflect.TypeFor[map[string]int](), "maxProperties=2", "unknown key"},
		{reflect.TypeFor[float64](), "minimum=2,maximum=1", "conflicting numeric bounds"},
		{reflect.TypeFor[float64](), "minimum=1,exclusiveMaximum=1", "conflicting numeric bounds"},
		{reflect.TypeFor[float64](), "exclusiveMinimum=1,maximum=1", "conflicting numeric bounds"},
		{reflect.TypeFor[float64](), "minimum=1,exclusiveMinimum=2,maximum=1.5", "conflicting numeric bounds"},
		{reflect.TypeFor[int](), "exclusiveMinimum=0,exclusiveMaximum=1", "conflicting numeric bounds"},
		{reflect.TypeFor[int](), "minimum=-0.9,maximum=-0.1", "conflicting numeric bounds"},
		{reflect.TypeFor[uint8](), "minimum=256", "conflicting numeric bounds"},
		{reflect.TypeFor[uint](), "maximum=-1", "conflicting numeric bounds"},
		{reflect.TypeFor[[2]int](), "minItems=3", "conflicting items bounds"},
		{reflect.TypeFor[[2]int](), "maxItems=1", "conflicting items bounds"},
		{reflect.TypeFor[[]int](), "minItems=2,maxItems=1", "conflicting items bounds"},
		{reflect.TypeFor[[]int](), "uniqueItems=TRUE", "expected true or false"},
		{reflect.TypeFor[[]int](), "uniqueItems=1", "expected true or false"},
		{reflect.TypeFor[string](), "pattern=[", "error parsing regexp"},
		{reflect.TypeFor[string](), `pattern="["`, "error parsing regexp"},
		{reflect.TypeFor[string](), `pattern="abc`, "unterminated string"},
		{reflect.TypeFor[string](), `pattern="\q"`, "escape"},
		{reflect.TypeFor[string](), `pattern='['`, "error parsing regexp"},
		{reflect.TypeFor[string](), `pattern='abc`, "unterminated string"},
		{reflect.TypeFor[string](), `pattern='abc\'`, "unterminated string"},
		{reflect.TypeFor[string](), `pattern='abc"`, "unterminated string"},
		{reflect.TypeFor[string](), `pattern='\q'`, "escape"},
		{reflect.TypeFor[string](), `pattern='abc'junk`, "invalid character"},
		{reflect.TypeFor[string](), `enum=['dev'}`, "unbalanced brackets"},
		{reflect.TypeFor[string](), "enum=[]", "enum must not be empty"},
		{reflect.TypeFor[string](), "enum='dev'", "enum requires a nonempty array literal"},
		{reflect.TypeFor[string](), "enum=['dev',]", "invalid enum array literal"},
		{reflect.TypeFor[string](), "enum=['dev'] false", "invalid enum array literal"},
		{reflect.TypeFor[string](), `enum=['\q']`, "escape"},
		{reflect.TypeFor[string](), `enum=['it\'s',"it's"]`, "duplicate enum member"},
		{reflect.TypeFor[string](), "enum=[1]", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[string](), "enum=[null]", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[int](), "enum=[1.5]", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[int](), "enum=[1,1.0]", "duplicate enum member"},
		{reflect.TypeFor[int64](), "enum=[9007199254740993,9007199254740993.0]", "duplicate enum member"},
		{reflect.TypeFor[bool](), "enum=[true,true]", "duplicate enum member"},
		{reflect.TypeFor[bool](), "enum=['true']", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[*string](), "enum=[null,null]", "duplicate enum member"},
		{reflect.TypeFor[[]int](), "enum=[[1]]", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[map[string]int](), "enum=[{}]", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[any](), "enum=[1]", "not a scalar allowed by the explicit schema type"},
		{reflect.TypeFor[float64](), "enum=[1e-9999]", "outside the nonzero finite float64 range"},
		{reflect.TypeFor[float64](), "enum=[" + strings.Repeat("9", 310) + ".1]", "outside the nonzero finite float64 range"},
		{reflect.TypeFor[float64](), "enum=[0.1,1e-1]", "duplicate enum member"},
		{reflect.TypeFor[float64](), "enum=[0.1,0.10000000000000001]", "equal after float64 rounding"},
		{reflect.TypeFor[float64](), "enum=[1,0.99999999999999999]", "equal after float64 rounding"},
		{reflect.TypeFor[float64](), "enum=[0.99999999999999999,1]", "equal after float64 rounding"},
	} {
		t.Run(test.typ.String()+"/"+test.tag, func(t *testing.T) {
			s, err := jsonschema.ForType(taggedType(test.typ, test.tag), nil)
			if err == nil || s != nil {
				t.Fatalf("ForType = %v, %v; want nil, error", s, err)
			}
			for _, context := range []string{"Value", `$["value"]`, "=", test.wantErr} {
				if !strings.Contains(err.Error(), context) {
					t.Errorf("missing %q in %v", context, err)
				}
			}
		})
	}
}

func TestNestedTagError(t *testing.T) {
	type Item struct {
		Name string `json:"name" jsonschemax:"minLength=1,minLength=2"`
	}
	type Config struct {
		Children map[string][]*Item `json:"children"`
	}
	s, err := jsonschema.For[Config](nil)
	if err == nil || s != nil {
		t.Fatalf("For = %v, %v; want nil, error", s, err)
	}
	for _, context := range []string{
		"Config.Children[value][].Name",
		`$["children"][*][*]["name"]`,
		`minLength="2"`,
		"duplicate key",
	} {
		if !strings.Contains(err.Error(), context) {
			t.Errorf("missing %q in %v", context, err)
		}
	}
}

type Promoted struct {
	Name string `json:"name" jsonschemax:"minLength=2"`
}
type nestedConfig struct {
	Promoted
	Child   *Promoted            `json:"child,omitempty"`
	List    []Promoted           `json:"list,omitzero" jsonschemax:"minItems=1"`
	Array   [1]Promoted          `json:"array,omitempty"`
	Map     map[string]*Promoted `json:"map,omitempty"`
	Ignored string               `json:"-" jsonschemax:"bad"`
	hidden  string               `jsonschemax:"bad"`
}

func TestTraversal(t *testing.T) {
	s, err := jsonschema.For[nestedConfig](nil)
	if err != nil {
		t.Fatal(err)
	}
	validateJSON(t, s, []string{
		`{"name":"ok"}`,
		`{"name":"ok","child":null,"list":null}`,
		`{"name":"ok","child":{"name":"ok"},"list":[{"name":"ok"}],"array":[{"name":"ok"}],"map":{"a":{"name":"ok"}}}`,
	}, []string{
		`{}`, `{"name":"x"}`, `{"name":"ok","child":{"name":"x"}}`,
		`{"name":"ok","list":[{"name":"x"}]}`, `{"name":"ok","list":[]}`,
		`{"name":"ok","array":[{"name":"x"}]}`, `{"name":"ok","map":{"a":{"name":"x"}}}`,
	})
	// Top-level containers and pointers also recurse into their elements.
	for _, typ := range []reflect.Type{reflect.TypeFor[*Promoted](), reflect.TypeFor[[]Promoted](), reflect.TypeFor[[1]Promoted](), reflect.TypeFor[map[string]Promoted]()} {
		s, err := jsonschema.ForType(typ, nil)
		if err != nil {
			t.Fatal(err)
		}
		switch typ.Kind() {
		case reflect.Pointer:
			validateJSON(t, s, []string{`{"name":"ok"}`, `null`}, []string{`{"name":"x"}`})
		case reflect.Map:
			validateJSON(t, s, []string{`{"a":{"name":"ok"}}`}, []string{`{"a":{"name":"x"}}`})
		default:
			validateJSON(t, s, []string{`[{"name":"ok"}]`}, []string{`[{"name":"x"}]`})
		}
	}
}

func TestTagsOnIgnoredTypes(t *testing.T) {
	type Config struct {
		C     chan int            `jsonschemax:"bad"`
		Slice []chan int          `jsonschemax:"bad"`
		Map   map[string]chan int `jsonschemax:"bad"`
		Good  string              `jsonschemax:"minLength=1"`
	}
	if s, err := jsonschema.For[Config](nil); s != nil || err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("For = %v, %v; want unsupported type error", s, err)
	}
	s, err := jsonschema.For[Config](&jsonschema.ForOptions{IgnoreInvalidTypes: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Properties) != 1 || s.Properties["Good"] == nil {
		t.Fatalf("unexpected properties: %v", s.Properties)
	}
	validateJSON(t, s, []string{`{"Good":"ok"}`}, []string{`{"Good":""}`})
}

type opaqueConfig struct {
	Hidden string `jsonschemax:"bad"`
}

func TestCustomSchemas(t *testing.T) {
	custom := &jsonschema.Schema{
		Type: "string", MinLength: jsonschema.Ptr(2), MaxLength: jsonschema.Ptr(5),
		Enum: []any{"aa", "bbb", "cccc"}, AllOf: []*jsonschema.Schema{{Pattern: "^[a-z]+$"}},
	}
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[opaqueConfig](): custom}}
	before, _ := json.Marshal(custom)
	typ := taggedType(reflect.TypeFor[opaqueConfig](), "enum=['bbb','cccc','dd'],maxLength=3")
	for i := 0; i < 3; i++ {
		s, err := jsonschema.ForType(typ, opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Properties["value"].AllOf) != 2 {
			t.Fatal("lost baseline allOf")
		}
		validateJSON(t, s, []string{`{"value":"bbb"}`}, []string{`{"value":"aa"}`, `{"value":"cccc"}`, `{"value":"dd"}`})
	}
	for _, tag := range []string{"minLength=6", "maxLength=1", "minimum=1", "enum=[1]"} {
		if s, err := jsonschema.ForType(taggedType(reflect.TypeFor[opaqueConfig](), tag), opts); s != nil || err == nil {
			t.Fatalf("expected failure for %s: %v %v", tag, s, err)
		}
	}
	after, _ := json.Marshal(custom)
	if string(before) != string(after) || opts.TypeSchemas[reflect.TypeFor[opaqueConfig]()] != custom {
		t.Fatal("modified caller's schema/options")
	}
	// Opaque nodes allow field constraints only with explicit compatible types.
	for _, replacement := range []*jsonschema.Schema{
		{Ref: "#/$defs/S", Defs: map[string]*jsonschema.Schema{"S": {Type: "string"}}},
		{AnyOf: []*jsonschema.Schema{{Type: "string"}}},
		{},
	} {
		opts.TypeSchemas[reflect.TypeFor[opaqueConfig]()] = replacement
		if s, err := jsonschema.ForType(taggedType(reflect.TypeFor[opaqueConfig](), "minLength=1"), opts); err == nil || s != nil {
			t.Fatalf("expected explicit type error: %v %v", s, err)
		}
	}
	opts.TypeSchemas[reflect.TypeFor[opaqueConfig]()] = &jsonschema.Schema{Types: []string{"null", "string"}}
	s, err := jsonschema.ForType(taggedType(reflect.TypeFor[opaqueConfig](), "enum=[null,'ok'],minLength=2"), opts)
	if err != nil {
		t.Fatal(err)
	}
	validateJSON(t, s, []string{`{"value":null}`, `{"value":"ok"}`}, []string{`{"value":"x"}`})
	// Standard mappings must not be treated as Go structs.
	s, err = jsonschema.ForType(taggedType(reflect.TypeFor[time.Time](), "minLength=1"), nil)
	if err != nil {
		t.Fatal(err)
	}
	validateJSON(t, s, []string{`{"value":"2026-10-20"}`}, []string{`{"value":""}`})
}

func TestAmbiguousFields(t *testing.T) {
	type A struct {
		Name string `jsonschemax:"minLength=1"`
	}
	type B struct{ Name string }
	// Go-name conflicts vanish under reflect.VisibleFields, just as upstream.
	type Hidden struct {
		A
		B
	}
	s, err := jsonschema.For[Hidden](nil)
	if err != nil || len(s.Properties) != 0 {
		t.Fatalf("hidden fields: %v, %v", s, err)
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[struct {
			A `jsonschemax:"minLength=1"`
		}](),
		reflect.StructOf([]reflect.StructField{
			{Name: "X", Type: reflect.TypeFor[string](), Tag: `json:"same" jsonschemax:"minLength=1"`},
			{Name: "Y", Type: reflect.TypeFor[string](), Tag: `json:"same"`},
		}),
		reflect.StructOf([]reflect.StructField{
			{Name: "X", Type: reflect.TypeFor[string](), Tag: `json:"same"`},
			{Name: "Y", Type: reflect.TypeFor[int](), Tag: `json:"same" jsonschemax:"minimum=1"`},
		}),
		reflect.StructOf([]reflect.StructField{
			{Name: "X", Type: reflect.TypeFor[A](), Tag: `json:"same"`},
			{Name: "Y", Type: reflect.TypeFor[string](), Tag: `json:"same"`},
		}),
	} {
		if s, err := jsonschema.ForType(typ, nil); err == nil || s != nil {
			t.Fatalf("expected mapping error: %v %v", s, err)
		}
	}
	// Replaced embeddings are opaque, including tags on promoted fields.
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[A](): {Type: "object", Properties: map[string]*jsonschema.Schema{"custom": {Type: "string"}}}}}
	typ := reflect.TypeFor[struct{ A }]()
	want := &jsonschema.Schema{
		Type: "object", Properties: map[string]*jsonschema.Schema{"custom": {Type: "string"}},
		AdditionalProperties: falseSchema(), PropertyOrder: []string{"custom"},
	}
	got, err := jsonschema.ForType(typ, opts)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("embedded override mismatch (-want +got):\n%s", diff)
	}
	// A promoted override property colliding with an extended regular property
	// cannot be mapped reliably, regardless of which one wins upstream.
	for _, typ := range []reflect.Type{
		reflect.TypeFor[struct {
			A
			Custom string `json:"custom" jsonschemax:"minLength=1"`
		}](),
		reflect.TypeFor[struct {
			Custom string `json:"custom" jsonschemax:"minLength=1"`
			A
		}](),
	} {
		if s, err := jsonschema.ForType(typ, opts); err == nil || s != nil || !strings.Contains(err.Error(), "multiple fields") {
			t.Fatalf("expected override collision error: %v %v", s, err)
		}
	}
}

func TestTaggedDescendantCollisions(t *testing.T) {
	type Item struct {
		Name string `jsonschemax:"minLength=1"`
	}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*Item](), reflect.TypeFor[[]Item](),
		reflect.TypeFor[[1]Item](), reflect.TypeFor[map[string]Item](),
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", typ, reverse), func(t *testing.T) {
				fields := []reflect.StructField{
					{Name: "Nested", Type: typ, Tag: `json:"same"`},
					{Name: "Plain", Type: reflect.TypeFor[string](), Tag: `json:"same"`},
				}
				if reverse {
					slices.Reverse(fields)
				}
				s, err := jsonschema.ForType(reflect.StructOf(fields), nil)
				if err == nil || s != nil || !strings.Contains(err.Error(), "multiple fields") {
					t.Fatalf("expected descendant collision error: %v %v", s, err)
				}
			})
		}
	}
}

func TestReferencedSchemas(t *testing.T) {
	for _, base := range []*jsonschema.Schema{
		{Ref: "#/$defs/text", Type: "string"},
		{Ref: "#/$defs/text", Types: []string{"null", "string"}},
	} {
		opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[opaqueConfig](): base,
		}}
		for _, tag := range []string{"minLength=2", "enum=['ok']"} {
			s, err := jsonschema.ForType(taggedType(reflect.TypeFor[opaqueConfig](), tag), opts)
			if s != nil || err == nil {
				t.Fatalf("ForType with $ref and %q = %v, %v; want nil, error", tag, s, err)
			}
			for _, context := range []string{"Value", `$["value"]`, "$ref", "draft-07"} {
				if !strings.Contains(err.Error(), context) {
					t.Errorf("missing %q in %v", context, err)
				}
			}
		}
		// Without a field tag, the reference remains unchanged and opaque.
		typ := reflect.TypeFor[struct{ Value opaqueConfig }]()
		got, err := jsonschema.ForType(typ, opts)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(base, got.Properties["Value"]); diff != "" {
			t.Errorf("reference mismatch (-want +got):\n%s", diff)
		}
	}
	// Placing the reference inside allOf lets field constraints apply in both drafts.
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[opaqueConfig](): {
			Type: "string", AllOf: []*jsonschema.Schema{{Ref: "#/$defs/text"}},
		},
	}}
	for _, draft := range []string{
		"http://json-schema.org/draft-07/schema#",
		"https://json-schema.org/draft/2020-12/schema",
	} {
		t.Run(draft, func(t *testing.T) {
			s, err := jsonschema.ForType(taggedType(reflect.TypeFor[opaqueConfig](), "minLength=2"), opts)
			if err != nil {
				t.Fatal(err)
			}
			s.Schema = draft
			s.Defs = map[string]*jsonschema.Schema{"text": {Type: "string", Pattern: "^[a-z]+$"}}
			validateJSON(t, s, []string{`{"value":"ok"}`}, []string{`{"value":"x"}`, `{"value":"OK"}`})
		})
	}
}

func TestMixedScalarEnum(t *testing.T) {
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[opaqueConfig](): {Types: []string{"string", "number", "boolean", "null"}},
	}}
	for _, test := range []struct {
		tag            string
		valid, invalid []string
	}{
		{
			tag:     "enum=[1]",
			valid:   []string{`{"value":1}`},
			invalid: []string{`{"value":"1"}`, `{"value":2}`},
		},
		{
			tag:     "enum=[1,'1']",
			valid:   []string{`{"value":1}`, `{"value":"1"}`},
			invalid: []string{`{"value":"2"}`, `{"value":2}`},
		},
		{
			tag:     "enum=[1,'2',true,null]",
			valid:   []string{`{"value":1}`, `{"value":"2"}`, `{"value":true}`, `{"value":null}`},
			invalid: []string{`{"value":"1"}`, `{"value":2}`, `{"value":false}`},
		},
		{
			tag:     "enum=[0.1,1,'2']",
			valid:   []string{`{"value":0.1}`, `{"value":1}`, `{"value":"2"}`},
			invalid: []string{`{"value":"0.1"}`, `{"value":"1"}`, `{"value":2}`},
		},
		{
			tag:     "enum=[0.1,'0.1']",
			valid:   []string{`{"value":0.1}`, `{"value":"0.1"}`},
			invalid: []string{`{"value":0.2}`, `{"value":"0.2"}`},
		},
	} {
		s, err := jsonschema.ForType(taggedType(reflect.TypeFor[opaqueConfig](), test.tag), opts)
		if err != nil {
			t.Fatal(err)
		}
		validateJSON(t, s, test.valid, test.invalid)
	}
}

func TestIgnoredCollision(t *testing.T) {
	for _, skipped := range []reflect.Type{
		reflect.TypeFor[chan int](),
		reflect.TypeFor[[]chan int](),
		reflect.TypeFor[map[string]chan int](),
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%v", skipped, reverse), func(t *testing.T) {
				fields := []reflect.StructField{
					{Name: "Skipped", Type: skipped, Tag: `json:"value" jsonschemax:"invalid"`},
					{Name: "Value", Type: reflect.TypeFor[string](), Tag: `json:"value" jsonschemax:"minLength=1"`},
				}
				if reverse {
					slices.Reverse(fields)
				}
				s, err := jsonschema.ForType(reflect.StructOf(fields), &jsonschema.ForOptions{IgnoreInvalidTypes: true})
				if err != nil {
					t.Fatal(err)
				}
				validateJSON(t, s, []string{`{"value":"ok"}`}, []string{`{"value":""}`})
			})
		}
	}
}

func TestNestedEmbeddedOverride(t *testing.T) {
	type Inner struct {
		Name string `jsonschemax:"invalid"`
	}
	type Outer struct {
		Inner `jsonschemax:"invalid"`
		Other string `jsonschemax:"invalid"`
	}
	type Sibling struct {
		Count int `json:"count" jsonschemax:"minimum=1"`
	}
	type Second struct {
		Value string `jsonschemax:"invalid"`
	}
	// Skipping replaced descendants must not skip adjacent embeddings or fields.
	type Config struct {
		Outer
		Sibling
		Second
		Tail string `json:"tail" jsonschemax:"minLength=1"`
	}
	opts := &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[Outer]():  {Type: "object", Properties: map[string]*jsonschema.Schema{"outer": {Type: "string"}}},
		reflect.TypeFor[Inner]():  {Type: "object", Properties: map[string]*jsonschema.Schema{"inner": {Type: "string"}}},
		reflect.TypeFor[Second](): {Type: "object", Properties: map[string]*jsonschema.Schema{"second": {Type: "string"}}},
	}}
	want := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"outer":  {Type: "string"},
			"count":  {Type: "integer", AllOf: []*jsonschema.Schema{{Minimum: jsonschema.Ptr(1.0)}}},
			"second": {Type: "string"},
			"tail":   {Type: "string", AllOf: []*jsonschema.Schema{{MinLength: jsonschema.Ptr(1)}}},
		},
		AdditionalProperties: falseSchema(), PropertyOrder: []string{"outer", "count", "second", "tail"},
		Required: []string{"count", "tail"},
	}
	got, err := jsonschema.For[Config](opts)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("opaque embedded fields mismatch (-want +got):\n%s", diff)
	}
	validateJSON(t, got, []string{`{"count":1,"tail":"ok"}`}, []string{
		`{"count":0,"tail":"ok"}`, `{"count":1,"tail":""}`,
	})
}

// BenchmarkForType measures inference on nested types without tags, including
// IgnoreInvalidTypes, which must not cause repeated inference of subtrees.
func BenchmarkForType(b *testing.B) {
	for _, depth := range []int{10, 50, 100} {
		typ := reflect.TypeFor[string]()
		for range depth {
			typ = reflect.StructOf([]reflect.StructField{{Name: "Child", Type: typ}})
		}
		for _, ignore := range []bool{false, true} {
			opts := &jsonschema.ForOptions{IgnoreInvalidTypes: ignore}
			b.Run(fmt.Sprintf("depth=%d/ignore=%v", depth, ignore), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					if _, err := jsonschema.ForType(typ, opts); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
