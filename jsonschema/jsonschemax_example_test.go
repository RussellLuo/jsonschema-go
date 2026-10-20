// Copyright 2026 The JSON Schema Go Project Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package jsonschema_test

import (
	"fmt"

	"github.com/RussellLuo/jsonschema-go/jsonschema"
)

func Example_jsonschemax() {
	type Config struct {
		Port    int      `json:"port" jsonschemax:"minimum=1, maximum=65535"`
		Name    string   `json:"name" jsonschemax:"minLength=1, maxLength=64, pattern='^[a-z]+$'"`
		Servers []string `json:"servers" jsonschemax:"minItems=1, uniqueItems=true"`
		Mode    string   `json:"mode" jsonschemax:"enum=['dev', 'prod']"`
	}
	schema, err := jsonschema.For[Config](nil)
	if err != nil {
		panic(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		panic(err)
	}
	value := map[string]any{
		"port":    8080,
		"name":    "app",
		"servers": []string{"localhost"},
		"mode":    "prod",
	}
	fmt.Println("valid:", resolved.Validate(value) == nil)

	value["port"] = 0
	fmt.Println("invalid port:", resolved.Validate(value) == nil)
	value["port"] = 8080

	value["name"] = "App"
	fmt.Println("invalid name:", resolved.Validate(value) == nil)
	value["name"] = "app"

	value["servers"] = []string{"localhost", "localhost"}
	fmt.Println("duplicate servers:", resolved.Validate(value) == nil)
	value["servers"] = []string{"localhost"}

	value["mode"] = "test"
	fmt.Println("invalid mode:", resolved.Validate(value) == nil)

	// Output:
	// valid: true
	// invalid port: false
	// invalid name: false
	// duplicate servers: false
	// invalid mode: false
}
