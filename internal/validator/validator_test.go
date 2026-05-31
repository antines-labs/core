package validator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/antines-labs/core/internal/schema"
)

// ---- String ----

func TestStringValid(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {"min": 2, "max": 100}}`)
	assertValid(t, v, "hello")
	assertValid(t, v, "ab")
	assertValid(t, v, strings.Repeat("a", 100))
}

func TestStringMinMax(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {"min": 3, "max": 10}}`)
	assertInvalid(t, v, "ab", "min")
	assertInvalid(t, v, strings.Repeat("a", 11), "max")
}

func TestStringEmail(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {"email": true}}`)
	assertValid(t, v, "user@example.com")
	assertValid(t, v, "a.b@c.co")
	assertInvalid(t, v, "not-an-email", "email")
}

func TestStringUUID(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {"uuid": true}}`)
	assertValid(t, v, "550e8400-e29b-41d4-a716-446655440000")
	assertInvalid(t, v, "not-a-uuid", "uuid")
	assertInvalid(t, v, "550e8400-e29b-41d4-a716", "uuid")
}

func TestStringURL(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {"url": true}}`)
	assertValid(t, v, "https://example.com")
	assertValid(t, v, "http://a.b/c?d=e")
	assertInvalid(t, v, "not a url", "url")
}

func TestStringPattern(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {"pattern": "^[a-z]+$"}}`)
	assertValid(t, v, "hello")
	assertInvalid(t, v, "Hello123", "pattern")
}

func TestStringTypeError(t *testing.T) {
	v := mustCompile(t, `{"type": "string"}`)
	assertInvalid(t, v, 42, "type")
	assertInvalid(t, v, true, "type")
}

func TestStringRequired(t *testing.T) {
	v := mustCompile(t, `{"type": "string", "validations": {}}`)
	assertInvalid(t, v, nil, "required")
}

// ---- Number ----

func TestNumberValid(t *testing.T) {
	v := mustCompile(t, `{"type": "number"}`)
	assertValid(t, v, 42.0)
	assertValid(t, v, -1.5)
}

func TestNumberInt(t *testing.T) {
	v := mustCompile(t, `{"type": "number", "validations": {"int": true}}`)
	assertValid(t, v, 42)
	assertValid(t, v, 0)
	assertInvalid(t, v, 3.14, "int")
}

func TestNumberMinMax(t *testing.T) {
	v := mustCompile(t, `{"type": "number", "validations": {"min": 0, "max": 150}}`)
	assertValid(t, v, 0)
	assertValid(t, v, 150)
	assertInvalid(t, v, -1, "min")
	assertInvalid(t, v, 151, "max")
}

func TestNumberPositiveNegative(t *testing.T) {
	v := mustCompile(t, `{"type": "number", "validations": {"positive": true}}`)
	assertValid(t, v, 1)
	assertValid(t, v, 0.1)
	assertInvalid(t, v, 0, "positive")
	assertInvalid(t, v, -1, "positive")

	v2 := mustCompile(t, `{"type": "number", "validations": {"negative": true}}`)
	assertValid(t, v2, -1)
	assertInvalid(t, v2, 0, "negative")
	assertInvalid(t, v2, 1, "negative")
}

// ---- Boolean ----

func TestBoolean(t *testing.T) {
	v := mustCompile(t, `{"type": "boolean"}`)
	assertValid(t, v, true)
	assertValid(t, v, false)
	assertInvalid(t, v, "true", "type")
	assertInvalid(t, v, 1, "type")
	assertInvalid(t, v, nil, "required")
}

// ---- Enum ----

func TestEnum(t *testing.T) {
	v := mustCompile(t, `{"type": "enum", "values": ["admin", "member"]}`)
	assertValid(t, v, "admin")
	assertValid(t, v, "member")
	assertInvalid(t, v, "viewer", "enum")
	assertInvalid(t, v, 42, "type")
}

// ---- Date ----

func TestDate(t *testing.T) {
	v := mustCompile(t, `{"type": "date"}`)
	assertValid(t, v, "2024-01-15T10:30:00Z")
	assertValid(t, v, "2024-01-15")
	assertInvalid(t, v, "not-a-date", "date")
	assertInvalid(t, v, nil, "required")
}

// ---- Array ----

func TestArrayValid(t *testing.T) {
	v := mustCompile(t, `{"type": "array", "items": {"type": "string"}}`)
	assertValid(t, v, []interface{}{"a", "b"})
	assertValid(t, v, []interface{}{})
}

func TestArrayMinMax(t *testing.T) {
	v := mustCompile(t, `{"type": "array", "items": {"type": "string"}, "validations": {"min": 1, "max": 3}}`)
	assertValid(t, v, []interface{}{"a"})
	assertValid(t, v, []interface{}{"a", "b", "c"})
	assertInvalid(t, v, []interface{}{}, "min")
	assertInvalid(t, v, []interface{}{"a", "b", "c", "d"}, "max")
}

func TestArrayUnique(t *testing.T) {
	v := mustCompile(t, `{"type": "array", "items": {"type": "string"}, "validations": {"unique": true}}`)
	assertValid(t, v, []interface{}{"a", "b", "c"})
	assertInvalid(t, v, []interface{}{"a", "b", "a"}, "unique")
}

func TestArrayItemValidation(t *testing.T) {
	v := mustCompile(t, `{"type": "array", "items": {"type": "string", "validations": {"min": 2}}}`)
	assertValid(t, v, []interface{}{"hello", "world"})
	errs := v.Validate([]interface{}{"a"})
	if len(errs) == 0 {
		t.Fatal("expected validation error")
	}
	if errs[0].Path != "[0]" {
		t.Errorf("expected path [0], got %q", errs[0].Path)
	}
}

// ---- Object ----

func TestObjectValid(t *testing.T) {
	v := mustCompile(t, `{
		"type": "object",
		"fields": {
			"name": {"schema": {"type": "string"}, "optional": false, "nullable": false},
			"age": {"schema": {"type": "number"}, "optional": true, "nullable": false}
		}
	}`)
	assertValid(t, v, map[string]interface{}{"name": "Alice", "age": 30})
	assertValid(t, v, map[string]interface{}{"name": "Bob"}) // age is optional
}

func TestObjectRequired(t *testing.T) {
	v := mustCompile(t, `{
		"type": "object",
		"fields": {
			"name": {"schema": {"type": "string"}, "optional": false, "nullable": false}
		}
	}`)
	assertInvalid(t, v, map[string]interface{}{}, "required")
}

func TestObjectNullable(t *testing.T) {
	v := mustCompile(t, `{
		"type": "object",
		"fields": {
			"avatar": {"schema": {"type": "string"}, "optional": false, "nullable": true}
		}
	}`)
	assertValid(t, v, map[string]interface{}{"avatar": nil})
	assertValid(t, v, map[string]interface{}{"avatar": "https://example.com/avatar.png"})
}

func TestObjectStrict(t *testing.T) {
	v := mustCompile(t, `{
		"type": "object",
		"fields": {
			"name": {"schema": {"type": "string"}, "optional": false, "nullable": false}
		},
		"strict": true
	}`)
	assertValid(t, v, map[string]interface{}{"name": "Alice"})
	assertInvalid(t, v, map[string]interface{}{"name": "Alice", "extra": "oops"}, "unknown")
}

func TestObjectNested(t *testing.T) {
	v := mustCompile(t, `{
		"type": "object",
		"fields": {
			"profile": {
				"schema": {
					"type": "object",
					"fields": {
						"bio": {"schema": {"type": "string", "validations": {"min": 2}}, "optional": false, "nullable": false}
					},
					"strict": false
				},
				"optional": false,
				"nullable": false
			}
		}
	}`)
	assertValid(t, v, map[string]interface{}{
		"profile": map[string]interface{}{"bio": "Hello!"},
	})
	errs := v.Validate(map[string]interface{}{
		"profile": map[string]interface{}{"bio": "X"},
	})
	if len(errs) == 0 {
		t.Fatal("expected nested validation error")
	}
	if errs[0].Path != "profile.bio" {
		t.Errorf("expected path profile.bio, got %q", errs[0].Path)
	}
	if errs[0].Code != "min" {
		t.Errorf("expected code min, got %q", errs[0].Code)
	}
}

// ---- Nullable / Optional wrappers ----

func TestNullable(t *testing.T) {
	v := mustCompile(t, `{"type": "nullable", "inner": {"type": "string", "validations": {"min": 2}}}`)
	assertValid(t, v, nil)
	assertValid(t, v, "hello")
	assertInvalid(t, v, "x", "min")
}

func TestOptional(t *testing.T) {
	v := mustCompile(t, `{"type": "optional", "inner": {"type": "string", "validations": {"min": 2}}}`)
	assertValid(t, v, nil)
	assertValid(t, v, "hello")
	assertInvalid(t, v, "x", "min")
}

// ---- Integration: compile from IR ----

func TestCompileFromIR(t *testing.T) {
	ir := &schema.IR{
		Type: "object",
		Fields: map[string]schema.FieldIR{
			"name": {
				Schema: schema.IR{
					Type:        "string",
					Validations: json.RawMessage(`{"min": 2, "max": 100}`),
				},
				Optional: false,
				Nullable: false,
			},
			"email": {
				Schema: schema.IR{
					Type:        "string",
					Validations: json.RawMessage(`{"email": true}`),
				},
				Optional: false,
				Nullable: false,
			},
		},
		Strict: false,
	}

	v, err := Compile(ir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	assertValid(t, v, map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	})

	assertInvalid(t, v, map[string]interface{}{
		"name":  "A",
		"email": "not-email",
	}, "min", "email")
}

// ---- Helpers ----

func mustCompile(t *testing.T, rawJSON string) Node {
	t.Helper()
	var ir schema.IR
	if err := json.Unmarshal([]byte(rawJSON), &ir); err != nil {
		t.Fatalf("Unmarshal schema: %v", err)
	}
	v, err := Compile(&ir)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return v
}

func assertValid(t *testing.T, v Node, value interface{}) {
	t.Helper()
	errs := v.Validate(value)
	if len(errs) > 0 {
		t.Errorf("expected valid, got errors: %v", errs)
	}
}

func assertInvalid(t *testing.T, v Node, value interface{}, expectedCodes ...string) {
	t.Helper()
	errs := v.Validate(value)
	if len(errs) == 0 {
		t.Errorf("expected validation error(s) with codes %v, got none", expectedCodes)
		return
	}

	// Check that all expected codes are present
	for _, code := range expectedCodes {
		found := false
		for _, e := range errs {
			if e.Code == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error code %q not found in errors: %v", code, errs)
		}
	}
}
