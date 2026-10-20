// Copyright 2026 The JSON Schema Go Project Authors. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package jsonschema

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// splitTag keeps commas inside strings and nested JSON values intact. JSON
// validity is checked separately, after extracting each key and value.
func splitTag(tag string) ([]string, error) {
	var parts []string
	var stack []byte
	var quote byte
	escaped, start := false, 0
	inValue, rawPattern := false, false
	for i := 0; i < len(tag); i++ {
		c := tag[i]
		// Unquoted patterns are Go regexp text, not JSON containers. In
		// particular, escaped brackets and braces inside character classes
		// must not affect nesting. Patterns containing commas use quotes.
		if rawPattern {
			if c == ',' {
				parts = append(parts, tag[start:i])
				start, inValue, rawPattern = i+1, false, false
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '=' && !inValue && len(stack) == 0 {
			inValue = true
			if strings.TrimSpace(tag[start:i]) == "pattern" {
				value := strings.TrimSpace(tag[i+1:])
				rawPattern = !strings.HasPrefix(value, `"`) && !strings.HasPrefix(value, "'")
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '[', '{':
			stack = append(stack, c)
		case ']', '}':
			if len(stack) == 0 || (c == ']' && stack[len(stack)-1] != '[') || (c == '}' && stack[len(stack)-1] != '{') {
				return nil, fmt.Errorf("unbalanced brackets")
			}
			stack = stack[:len(stack)-1]
		case ',':
			if len(stack) == 0 {
				parts = append(parts, tag[start:i])
				start = i + 1
				inValue = false
			}
		}
	}
	if quote != 0 || len(stack) != 0 {
		return nil, fmt.Errorf("unterminated string or JSON value")
	}
	return append(parts, tag[start:]), nil
}

func tagError(goPath, jsonPath, key, value string, err error) error {
	return fmt.Errorf("jsonschemax: Go %s, JSON %s, %s=%q: %w", goPath, jsonPath, key, value, err)
}

func parseTag(tag string, base *Schema, goPath, jsonPath string) (*Schema, error) {
	if base.Ref != "" {
		return nil, tagError(goPath, jsonPath, "tag", tag, fmt.Errorf("cannot extend a schema with $ref: draft-07 ignores sibling constraints"))
	}
	parts, err := splitTag(tag)
	if err != nil {
		return nil, tagError(goPath, jsonPath, "tag", tag, err)
	}
	s := new(Schema)
	seen := map[string]bool{}
	for _, part := range parts {
		key, value, found := strings.Cut(strings.TrimSpace(part), "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		fail := func(err error) (*Schema, error) {
			return nil, tagError(goPath, jsonPath, key, value, err)
		}
		if !found || key == "" || value == "" {
			return fail(fmt.Errorf("expected nonempty key=value"))
		}
		if seen[key] {
			return fail(fmt.Errorf("duplicate key"))
		}
		seen[key] = true
		var types []string
		switch key {
		case "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
			types = []string{"integer", "number"}
			f, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return fail(fmt.Errorf("expected finite number"))
			}
			if f == 0 {
				// ParseFloat silently rounds tiny nonzero values to zero. Check
				// with a wider exponent range before accepting a zero bound.
				n, _, err := big.ParseFloat(value, 0, 64, big.ToNearestEven)
				if err != nil || n.Sign() != 0 {
					return fail(fmt.Errorf("nonzero number underflows float64"))
				}
			}
			switch key {
			case "minimum":
				s.Minimum = &f
			case "maximum":
				s.Maximum = &f
			case "exclusiveMinimum":
				s.ExclusiveMinimum = &f
			case "exclusiveMaximum":
				s.ExclusiveMaximum = &f
			}
		case "minLength", "maxLength", "minItems", "maxItems":
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 {
				return fail(fmt.Errorf("expected nonnegative integer within int range"))
			}
			switch key {
			case "minLength":
				s.MinLength, types = &n, []string{"string"}
			case "maxLength":
				s.MaxLength, types = &n, []string{"string"}
			case "minItems":
				s.MinItems, types = &n, []string{"array"}
			case "maxItems":
				s.MaxItems, types = &n, []string{"array"}
			}
		case "uniqueItems":
			types = []string{"array"}
			if value != "true" && value != "false" {
				return fail(fmt.Errorf("expected true or false"))
			}
			s.UniqueItems = value == "true"
		case "pattern":
			types = []string{"string"}
			pattern := value
			if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'") {
				normalized, err := normalizeQuotes(value)
				if err != nil {
					return fail(err)
				}
				if err := json.Unmarshal([]byte(normalized), &pattern); err != nil {
					return fail(err)
				}
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return fail(err)
			}
			s.Pattern = pattern
		case "enum":
			var err error
			s.Enum, err = parseEnum(value, base)
			if err != nil {
				return fail(err)
			}
			guardEnumNumbers(s, base)
		default:
			return fail(fmt.Errorf("unknown key"))
		}
		if len(types) > 0 && !hasOnlyTypes(base, types...) {
			return fail(fmt.Errorf("constraint requires explicit schema type %v (optionally nullable)", types))
		}
	}
	if err := checkBounds(base, s); err != nil {
		return nil, tagError(goPath, jsonPath, "tag", tag, err)
	}
	return s, nil
}

// Every non-null schema type must be explicitly allowed by the constraint.
func hasOnlyTypes(s *Schema, allowed ...string) bool {
	types := s.Types
	if s.Type != "" {
		types = []string{s.Type}
	}
	nonNull := false
	for _, typ := range types {
		if typ == "null" {
			continue
		}
		if !slices.Contains(allowed, typ) {
			return false
		}
		nonNull = true
	}
	return nonNull
}

func allows(s *Schema, typ string) bool {
	if s.Type != "" {
		return s.Type == typ
	}
	return slices.Contains(s.Types, typ)
}

// normalizeQuotes translates single-quoted strings to JSON strings without
// changing numeric tokens or permitting any other non-JSON syntax.
func normalizeQuotes(value string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '\'' && value[i] != '"' {
			out.WriteByte(value[i])
			i++
			continue
		}
		quote := value[i]
		i++
		var body strings.Builder
		closed := false
		for i < len(value) {
			c := value[i]
			i++
			if c == quote {
				closed = true
				break
			}
			if c == '\\' {
				if i == len(value) {
					return "", fmt.Errorf("incomplete escape")
				}
				next := value[i]
				i++
				if quote == '\'' && next == '\'' {
					body.WriteByte('\'')
				} else {
					body.WriteByte('\\')
					body.WriteByte(next)
				}
			} else {
				if quote == '\'' && c == '"' {
					body.WriteByte('\\')
				}
				body.WriteByte(c)
			}
		}
		if !closed {
			return "", fmt.Errorf("unterminated string")
		}
		var decoded string
		if err := json.Unmarshal([]byte(`"`+body.String()+`"`), &decoded); err != nil {
			return "", err
		}
		encoded, _ := json.Marshal(decoded)
		out.Write(encoded)
	}
	return out.String(), nil
}

func parseEnum(value string, base *Schema) ([]any, error) {
	if !strings.HasPrefix(value, "[") {
		return nil, fmt.Errorf("enum requires a nonempty array literal")
	}
	normalized, err := normalizeQuotes(value)
	if err != nil {
		return nil, err
	}
	// Valid also rejects trailing input; UseNumber preserves integer precision
	// and lets us check mathematical equality before rounding non-integers.
	if !json.Valid([]byte(normalized)) {
		return nil, fmt.Errorf("invalid enum array literal")
	}
	dec := json.NewDecoder(strings.NewReader(normalized))
	dec.UseNumber()
	var values []any
	if err := dec.Decode(&values); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("enum must not be empty")
	}
	// Reuse parsed numbers for type checks and both rounds of duplicate detection.
	numbers := make([]*big.Rat, len(values))
	for i, value := range values {
		valid := false
		switch v := value.(type) {
		case nil:
			valid = allows(base, "null")
		case string:
			valid = allows(base, "string")
		case bool:
			valid = allows(base, "boolean")
		case json.Number:
			r, ok := new(big.Rat).SetString(string(v))
			if !ok {
				return nil, fmt.Errorf("enum number %q is out of range", v)
			}
			numbers[i] = r
			valid = allows(base, "number") || (allows(base, "integer") && r.IsInt())
		}
		if !valid {
			return nil, fmt.Errorf("enum member %v is not a scalar allowed by the explicit schema type", value)
		}
		for j, previous := range values[:i] {
			if previous == value || (numbers[i] != nil && numbers[j] != nil && numbers[i].Cmp(numbers[j]) == 0) {
				return nil, fmt.Errorf("duplicate enum member %v", value)
			}
		}
	}
	for i, value := range values {
		n, ok := value.(json.Number)
		if !ok {
			continue
		}
		// Upstream compares json.Number as an exact rational but native floats
		// as binary rationals. Store non-integers as float64 so ordinary JSON
		// decoding and direct float64 fields agree with the inferred enum.
		if !numbers[i].IsInt() {
			f, err := n.Float64()
			if err != nil || math.IsInf(f, 0) || f == 0 {
				return nil, fmt.Errorf("enum number %q is outside the nonzero finite float64 range", n)
			}
			values[i] = f
			numbers[i] = new(big.Rat).SetFloat64(f)
		}
		for j, previous := range values[:i] {
			if numbers[j] != nil && numbers[j].Cmp(numbers[i]) == 0 {
				return nil, fmt.Errorf("enum numbers become equal after float64 rounding: %v and %s", previous, n)
			}
		}
	}
	return values, nil
}

// Upstream Equal treats json.Number("1") and the string "1" as equal.
// Most fields have a single non-null type, so their inferred type check already
// prevents that ambiguity. For custom unions allowing both strings and numbers,
// add standard typed enum branches to preserve JSON scalar identity as well as
// numeric precision. Keep Enum itself on the constraint schema.
func guardEnumNumbers(s, base *Schema) {
	if !allows(base, "string") {
		return
	}
	groups := map[string][]any{}
	for _, value := range s.Enum {
		typ := "null"
		switch value.(type) {
		case string:
			typ = "string"
		case bool:
			typ = "boolean"
		case json.Number, float64:
			typ = "number"
		}
		groups[typ] = append(groups[typ], value)
	}
	if len(groups["number"]) == 0 {
		return
	}
	for _, typ := range []string{"null", "string", "boolean", "number"} {
		if values := groups[typ]; len(values) > 0 {
			s.AnyOf = append(s.AnyOf, &Schema{Type: typ, Enum: values})
		}
	}
}

func checkBounds(base, tag *Schema) error {
	for _, pair := range []struct {
		name                       string
		min, max, baseMin, baseMax *int
	}{
		{"length", tag.MinLength, tag.MaxLength, base.MinLength, base.MaxLength},
		{"items", tag.MinItems, tag.MaxItems, base.MinItems, base.MaxItems},
	} {
		if pair.min == nil && pair.max == nil {
			continue
		}
		low, high := 0, math.MaxInt
		for _, p := range []*int{pair.min, pair.baseMin} {
			if p != nil && *p > low {
				low = *p
			}
		}
		for _, p := range []*int{pair.max, pair.baseMax} {
			if p != nil && *p < high {
				high = *p
			}
		}
		if low > high {
			return fmt.Errorf("conflicting %s bounds (including baseline): %d > %d", pair.name, low, high)
		}
	}
	if tag.Minimum == nil && tag.Maximum == nil && tag.ExclusiveMinimum == nil && tag.ExclusiveMaximum == nil {
		return nil
	}
	low, high := math.Inf(-1), math.Inf(1)
	lowOpen, highOpen := false, false
	for _, s := range []*Schema{base, tag} {
		for _, b := range []struct {
			p    *float64
			open bool
		}{{s.Minimum, false}, {s.ExclusiveMinimum, true}} {
			if b.p != nil && (*b.p > low || (*b.p == low && b.open)) {
				low, lowOpen = *b.p, b.open
			}
		}
		for _, b := range []struct {
			p    *float64
			open bool
		}{{s.Maximum, false}, {s.ExclusiveMaximum, true}} {
			if b.p != nil && (*b.p < high || (*b.p == high && b.open)) {
				high, highOpen = *b.p, b.open
			}
		}
	}
	conflict := low > high || (low == high && (lowOpen || highOpen))
	if hasOnlyTypes(base, "integer") && !conflict && !math.IsInf(low, 0) && !math.IsInf(high, 0) {
		// Use rationals for exclusive endpoints; adding 1 to large floats loses precision.
		lower, upper := new(big.Rat).SetFloat64(low), new(big.Rat).SetFloat64(high)
		// Div rounds down even for negative bounds; adjust to the allowed integers.
		minInt := new(big.Int).Div(lower.Num(), lower.Denom())
		maxInt := new(big.Int).Div(upper.Num(), upper.Denom())
		if !lower.IsInt() || lowOpen {
			minInt.Add(minInt, big.NewInt(1))
		}
		if upper.IsInt() && highOpen {
			maxInt.Sub(maxInt, big.NewInt(1))
		}
		conflict = minInt.Cmp(maxInt) > 0
	}
	if conflict {
		return fmt.Errorf("conflicting numeric bounds (including baseline)")
	}
	return nil
}
