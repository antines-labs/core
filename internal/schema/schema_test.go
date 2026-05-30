package schema

import (
	"encoding/json"
	"testing"
)

func TestParseStringValidations(t *testing.T) {
	s := SchemaIR{
		Type: "string",
		Validations: json.RawMessage(`{
			"min": 2,
			"max": 100,
			"email": true
		}`),
	}

	v, err := s.ParseStringValidations()
	if err != nil {
		t.Fatalf("ParseStringValidations: %v", err)
	}
	if v.Min == nil || *v.Min != 2 {
		t.Errorf("expected min=2, got %v", v.Min)
	}
	if v.Max == nil || *v.Max != 100 {
		t.Errorf("expected max=100, got %v", v.Max)
	}
	if v.Email == nil || !*v.Email {
		t.Error("expected email=true")
	}
}

func TestParseNumberValidations(t *testing.T) {
	s := SchemaIR{
		Type: "number",
		Validations: json.RawMessage(`{
			"int": true,
			"min": 0,
			"max": 150
		}`),
	}

	v, err := s.ParseNumberValidations()
	if err != nil {
		t.Fatalf("ParseNumberValidations: %v", err)
	}
	if v.Int == nil || !*v.Int {
		t.Error("expected int=true")
	}
	if v.Min == nil || *v.Min != 0 {
		t.Errorf("expected min=0, got %v", v.Min)
	}
	if v.Max == nil || *v.Max != 150 {
		t.Errorf("expected max=150, got %v", v.Max)
	}
}

func TestParseValidationsNonMatchingType(t *testing.T) {
	// ParseStringValidations on a number type should return empty validations
	s := SchemaIR{Type: "number", Validations: json.RawMessage(`{"int": true}`)}
	v, err := s.ParseStringValidations()
	if err != nil {
		t.Fatalf("ParseStringValidations: %v", err)
	}
	if v.Min != nil || v.Max != nil {
		t.Error("expected empty string validations for number type")
	}
}

func TestParseNoValidations(t *testing.T) {
	s := SchemaIR{Type: "string"}
	v, err := s.ParseStringValidations()
	if err != nil {
		t.Fatalf("ParseStringValidations: %v", err)
	}
	if v.Min != nil || v.Max != nil {
		t.Error("expected empty validations")
	}
}
