// Copyright 2026 The JSON Schema Go Project Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

/*
# Validation tags

Use the "jsonschemax" struct tag to add validation constraints with [For] or
[ForType], for example:

	type Config struct {
		Port int    `json:"port" jsonschemax:"minimum=1, maximum=65535"`
		Name string `json:"name" jsonschemax:"pattern='^[a-z]{1,8}$'"`
	}

# Tag syntax and constraints

Tags are comma-separated key=value pairs. Whitespace around options, keys,
and values is ignored. Examples use a space after each separating comma.
The supported constraints are:

  - minimum and maximum: inclusive bounds for integer or number fields.
    exclusiveMinimum and exclusiveMaximum exclude the endpoints instead.
  - minLength and maxLength: string lengths, measured in characters.
    Values must be nonnegative integers within the Go int range.
  - pattern: a regular expression for a string field.
  - minItems and maxItems: array lengths. Values must be nonnegative
    integers within the Go int range.
  - uniqueItems: whether array elements must be distinct; true or false.
  - enum: a nonempty array of allowed scalar values compatible with the
    field's schema type.

Invalid tags fail during inference, before validation. Empty tags, unknown
or duplicate keys, invalid values, incompatible schema types, and conflicting
bounds return errors with Go and JSON field paths. For example, minLength=3,
maxLength=2 is rejected. Tags must use the spelling shown above.

# Patterns and enums

Pattern values and enum strings may use single or double quotes. Prefer
single quotes to avoid escaping double quotes in Go tags. A quoted pattern
can contain commas without separating tag options:

	Name string `jsonschemax:"pattern='^[a-z]{1,8}$', minLength=1"`

Patterns use Go's [regexp] syntax, following the existing validator. Matching
is not implicitly anchored: use ^ and $ to match the entire string.

Quoted strings use JSON escapes; single-quoted strings also accept \' for a
literal single quote. Go struct tags add another string-escape layer. These
fields pass ^it's$ and ^\d{1,3}$, respectively, to the regular expression engine:

	Word   string `jsonschemax:"pattern='^it\\'s$'"`
	Digits string `jsonschemax:"pattern='^\\\\d{1,3}$'"`

Patterns may also be unquoted if they contain no commas or significant
surrounding whitespace and do not begin with a quote. Quotes elsewhere in a
bare pattern are literal, so pattern=^it's$ needs no quote escaping.

Enum values are nonempty JSON arrays of scalars; arrays and objects cannot
be enum members. For example:

	Mode  string `jsonschemax:"enum=['dev', 'prod']"`
	Level int    `jsonschemax:"enum=[1, 2, 3]"`

Duplicate members are rejected. Numeric members must also be distinct after
float64 conversion where applicable; see Numeric precision below.

# Field behavior

Constraints are appended as allOf subschemas, preserving the inferred
constraints and nullability. For example, a uint8 field with maximum=1000
still rejects 256 because the inferred uint8 range also applies.

Field presence and nullability are separate. Fields are required unless the
json tag contains omitempty or omitzero. Pointer fields may allow null even
when a tag constrains non-null values:

	Name *string `json:"name,omitempty" jsonschemax:"minLength=1"`

This field may be absent or null. A supplied string must have at least one
character, so an empty string is rejected.

Enum restricts the entire value, including null. Even when the inferred
schema allows null, enum must list it to accept it. The first field below
rejects null; the second accepts it:

	Mode         *string `jsonschemax:"enum=['dev', 'prod']"`
	NullableMode *string `jsonschemax:"enum=['dev', 'prod', null]"`

Listing null cannot make a non-nullable field accept it; inference rejects
enum members incompatible with the field's schema type.

Field constraints apply to the field itself, rather than its elements:

	Servers []string `jsonschemax:"minItems=1, uniqueItems=true"`

This field requires at least one element when it is an array, and rejects
duplicate elements. It does not constrain each string's length: an array
containing one empty string satisfies these constraints. Nullability still
follows the inferred schema. Tags inside element structs apply recursively,
as do tags inside ordinary nested structs.

Tags on anonymous fields are rejected because inference promotes their
fields instead of creating independent properties. JSON property name
collisions involving a tagged field or tagged descendants also return an
error, preventing constraints from being discarded.

# Numeric precision

Mathematically integral candidates, including 2.0 and 3e0, retain their exact
value in the inferred enum. Numeric bounds use float64, as in [Schema], so
large integer bounds may be rounded:

	Exact   int64 `jsonschemax:"enum=[9007199254740993]"`
	Rounded int64 `jsonschemax:"minimum=9007199254740993"`

The enum retains 9007199254740993 exactly in the inferred schema, while the
minimum is rounded to 9007199254740992.

Non-integral enum numbers use float64 to match default encoding/json
decoding. Float64 conversions reject overflow and underflow to zero; other
values may be rounded. For example, enum=[0.1] accepts a value decoded as
float64(0.1), while enum=[0.1, 0.10000000000000001] is rejected because the
candidates become equal after rounding.

Numeric comparison and schema decoding follow the existing package
semantics. Preserve native int64 or uint64 values when validating large
integers; decoding them through float64 may lose precision.

# Custom schemas

When inference selects a custom [ForOptions.TypeSchemas] or standard library
mapping, the replaced type is opaque: its internal Go fields are not
traversed, so tags on those fields do not apply. Mapping selection follows
the existing inference rules.

Tags on the containing field can still apply when the replacement schema
declares an explicit compatible type. Null may also be allowed. For example,
minLength requires string, optionally with null; it rejects a schema that
also allows numbers. Enum members may use any explicitly allowed scalar type.

Field tags on schemas with $ref are rejected because draft-07 ignores sibling
constraints. To extend a reference, wrap it in allOf and declare an explicit
compatible type, for example:

	{
	    "type": "string",
	    "allOf": [{"$ref": "#/$defs/text"}]
	}

Define text in the root schema's $defs before resolving the schema.

# Validating tagged schemas

After inference, call [Schema.Resolve] and then [Resolved.Validate] to
validate JSON values. Objects must be represented as map[string]any,
not Go structs. For the opening Config example, a suitable value is:

	value := map[string]any{"port": 8080, "name": "app"}

The package-level jsonschemax example demonstrates the complete runnable
workflow, including error handling and invalid values.
*/
package jsonschema
