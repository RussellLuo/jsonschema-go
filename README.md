# JSON Schema for Go

This module implements the [JSON Schema](https://json-schema.org/) specification.
The `jsonschema` package supports creating schemas, validating JSON values
against a schema, and inferring a schema from a Go struct. See the package
documentation for usage.

## jsonschemax: struct tag extensions

This fork adds a `jsonschemax` struct tag for validation constraints when
inferring JSON Schema with `jsonschema.For` or `jsonschema.ForType`.
It addresses requests for tag-based constraints such as
[upstream issue #47](https://github.com/google/jsonschema-go/issues/47).

### Example

```go
import "github.com/RussellLuo/jsonschema-go/jsonschema"

type Config struct {
    Port int    `json:"port" jsonschemax:"minimum=1, maximum=65535"`
    Mode string `json:"mode" jsonschemax:"enum=['dev', 'prod']"`
}

schema, err := jsonschema.For[Config](nil)
```

See [jsonschemax_example_test.go](jsonschema/jsonschemax_example_test.go) for
a complete, runnable example of schema inference and validation.

### Supported constraints

| Constraint | Keywords | Example |
| --- | --- | --- |
| Number bounds | `minimum`, `maximum` | `minimum=1, maximum=65535` |
| Exclusive bounds | `exclusiveMinimum`, `exclusiveMaximum` | `exclusiveMinimum=0, exclusiveMaximum=1` |
| Strings | `minLength`, `maxLength`, `pattern` | `minLength=1, maxLength=32, pattern='^[a-z]+$'` |
| Arrays | `minItems`, `maxItems`, `uniqueItems` | `minItems=1, maxItems=10, uniqueItems=true` |
| Scalar choices | `enum` | `enum=['dev', 'prod']` or `enum=[1, 2, 3]` |

See [Validation tags](https://pkg.go.dev/github.com/RussellLuo/jsonschema-go/jsonschema#hdr-Validation_tags)
for tag syntax and validation semantics.

## Contributing

This module welcomes external contributions.
It has no dependencies outside of the standard library, and can be built with
the standard Go toolchain. Run `go test ./...` at the module root to run all
the tests.

## Issues

This project uses the [GitHub issue
tracker](https://github.com/RussellLuo/jsonschema-go/issues) for bug reports, feature requests, and other issues.

Please [report
bugs](https://github.com/RussellLuo/jsonschema-go/issues/new?template=bug_report.md). If the module is
not working as you expected, it is likely due to a bug or inadequate
documentation, and reporting an issue will help us address this shortcoming.

When reporting a bug, make sure to answer these five questions:

1. What did you do?
2. What did you see?
3. What did you expect to see?
4. What version of `github.com/RussellLuo/jsonschema-go` are you using?
5. What version of Go are you using (`go version`)?

## License

This project is licensed under the MIT license. See the LICENSE file for details.

