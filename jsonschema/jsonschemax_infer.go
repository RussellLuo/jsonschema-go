// Copyright 2026 The JSON Schema Go Project Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package jsonschema

import (
	"fmt"
	"reflect"
)

// jsonschemaxInference carries extension state through the existing inference
// traversal. Each child has its own paths and reports tags to its parent.
type jsonschemaxInference struct {
	goPath, jsonPath string
	tagged           bool // this subtree contains an applied tag
	taggedProperties map[string]bool
}

func newJSONSchemaxInference(t reflect.Type) *jsonschemaxInference {
	return &jsonschemaxInference{goPath: t.String(), jsonPath: "$"}
}

func (x *jsonschemaxInference) inferElement(container reflect.Type, seen map[reflect.Type]bool, ignore bool, schemas map[reflect.Type]*Schema) (*Schema, error) {
	suffix := "[]"
	if container.Kind() == reflect.Map {
		suffix = "[value]"
	}
	child := jsonschemaxInference{goPath: x.goPath + suffix, jsonPath: x.jsonPath + "[*]"}
	s, err := forType(container.Elem(), seen, ignore, schemas, &child)
	if err == nil && s != nil {
		x.tagged = x.tagged || child.tagged
	}
	return s, err
}

func (x *jsonschemaxInference) field(t reflect.Type, field reflect.StructField, name string) jsonschemaxInference {
	goPath := x.goPath
	for _, i := range field.Index {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		f := t.Field(i)
		goPath += "." + f.Name
		t = f.Type
	}
	return jsonschemaxInference{goPath: goPath, jsonPath: x.jsonPath + fmt.Sprintf("[%q]", name)}
}

func (x *jsonschemaxInference) checkAnonymous(t reflect.Type, field reflect.StructField) error {
	if tag, ok := field.Tag.Lookup("jsonschemax"); ok {
		child := x.field(t, field, "")
		return tagError(child.goPath, x.jsonPath, "tag", tag, fmt.Errorf("anonymous field has no independent JSON property"))
	}
	return nil
}

func (x *jsonschemaxInference) checkEmbeddedProperty(t reflect.Type, field reflect.StructField, name string) error {
	if x.taggedProperties[name] {
		child := x.field(t, field, name)
		return child.collision("")
	}
	return nil
}

func (x *jsonschemaxInference) applyField(child *jsonschemaxInference, field reflect.StructField, name string, properties map[string]*Schema, s *Schema) error {
	tag, tagged := field.Tag.Lookup("jsonschemax")
	if properties[name] != nil && (child.tagged || tagged || x.taggedProperties[name]) {
		return child.collision(tag)
	}
	if tagged {
		constraint, err := parseTag(tag, s, child.goPath, child.jsonPath)
		if err != nil {
			return err
		}
		s.AllOf = append(s.AllOf, constraint)
	}
	if child.tagged || tagged {
		if x.taggedProperties == nil {
			x.taggedProperties = make(map[string]bool)
		}
		x.taggedProperties[name] = true
		x.tagged = true
	}
	return nil
}

func (x *jsonschemaxInference) collision(tag string) error {
	return tagError(x.goPath, x.jsonPath, "tag", tag, fmt.Errorf("multiple fields map to the same JSON property; cannot preserve constraints"))
}
