package manifest

import (
	"testing"

	"github.com/antines-labs/core/internal/schema"
)

func TestLoadValidManifest(t *testing.T) {
	m, err := Load("testdata/valid.json")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if m.Version != 1 {
		t.Errorf("expected version 1, got %d", m.Version)
	}

	if len(m.Routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(m.Routes))
	}

	// Route 1: POST /users
	r1 := m.Routes[0]
	if r1.Method != "POST" {
		t.Errorf("expected POST, got %s", r1.Method)
	}
	if r1.Path != "/users" {
		t.Errorf("expected /users, got %s", r1.Path)
	}
	if r1.HandlerID != 1 {
		t.Errorf("expected handlerId 1, got %d", r1.HandlerID)
	}
	if !r1.HasHandler {
		t.Error("expected hasHandler=true")
	}
	if r1.Schema.Input == nil {
		t.Fatal("expected input schema")
	}
	if r1.Schema.Input.Type != "object" {
		t.Errorf("expected input type object, got %s", r1.Schema.Input.Type)
	}
	if r1.Schema.Output == nil {
		t.Fatal("expected output schema")
	}
	if r1.Schema.Errors == nil {
		t.Fatal("expected errors")
	}
	if _, ok := r1.Schema.Errors["email_taken"]; !ok {
		t.Error("expected email_taken error")
	}

	// Route 2: POST /auth/login
	r2 := m.Routes[1]
	if r2.Method != "POST" {
		t.Errorf("expected POST, got %s", r2.Method)
	}
	if r2.Path != "/auth/login" {
		t.Errorf("expected /auth/login, got %s", r2.Path)
	}
	if r2.HandlerID != 2 {
		t.Errorf("expected handlerId 2, got %d", r2.HandlerID)
	}
	// Nested output schema
	nestedObj := r2.Schema.Output
	if nestedObj.Type != "object" {
		t.Fatalf("expected output type object, got %s", nestedObj.Type)
	}
	userField, ok := nestedObj.Fields["user"]
	if !ok {
		t.Fatal("expected user field in output")
	}
	if userField.Schema.Type != "object" {
		t.Errorf("expected user field type object, got %s", userField.Schema.Type)
	}

	// Route 3: GET /health (Go-only)
	r3 := m.Routes[2]
	if r3.Method != "GET" {
		t.Errorf("expected GET, got %s", r3.Method)
	}
	if r3.Path != "/health" {
		t.Errorf("expected /health, got %s", r3.Path)
	}
	if r3.HasHandler {
		t.Error("expected hasHandler=false for Go-only route")
	}
	if r3.Schema.Input != nil {
		t.Error("expected nil input schema for Go-only route")
	}
	if r3.Schema.Output == nil {
		t.Fatal("expected output schema")
	}

	// Parse string validations
	if r1.Schema.Input.Type == "object" {
		nameField := r1.Schema.Input.Fields["name"]
		if nameField.Schema.Type == "string" {
			v, err := nameField.Schema.ParseStringValidations()
			if err != nil {
				t.Fatalf("ParseStringValidations: %v", err)
			}
			if v.Min == nil || *v.Min != 2 {
				t.Errorf("expected min=2, got %v", v.Min)
			}
			if v.Max == nil || *v.Max != 100 {
				t.Errorf("expected max=100, got %v", v.Max)
			}
		}

		emailField := r1.Schema.Input.Fields["email"]
		if emailField.Schema.Type == "string" {
			v, err := emailField.Schema.ParseStringValidations()
			if err != nil {
				t.Fatalf("ParseStringValidations: %v", err)
			}
			if v.Email == nil || !*v.Email {
				t.Error("expected email=true")
			}
		}
	}
}

func TestLoadInvalidVersion(t *testing.T) {
	_, err := Load("testdata/invalid_version.json")
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestLoadEmptyRoutes(t *testing.T) {
	_, err := Load("testdata/empty_routes.json")
	if err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestLoadUnknownType(t *testing.T) {
	_, err := Load("testdata/unknown_type.json")
	if err == nil {
		t.Fatal("expected error for unknown schema type")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	_, err := Load("testdata/nonexistent.json")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestSchemaValidationHelpers(t *testing.T) {
	m, err := Load("testdata/valid.json")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// String validations
	input := m.Routes[0].Schema.Input
	nameField := input.Fields["name"]
	sv, err := nameField.Schema.ParseStringValidations()
	if err != nil {
		t.Fatalf("ParseStringValidations: %v", err)
	}
	if sv.Min == nil || *sv.Min != 2 {
		t.Errorf("expected min 2, got %v", sv.Min)
	}
	if sv.Max == nil || *sv.Max != 100 {
		t.Errorf("expected max 100, got %v", sv.Max)
	}

	// Number validations
	health := m.Routes[2]
	uptimeField := health.Schema.Output.Fields["uptime"]
	nv, err := uptimeField.Schema.ParseNumberValidations()
	if err != nil {
		t.Fatalf("ParseNumberValidations: %v", err)
	}
	if nv.Min != nil || nv.Max != nil {
		t.Errorf("expected no number validations, got min=%v max=%v", nv.Min, nv.Max)
	}

	// Array validations (none in test data — just verify it returns empty without error)
	emptyArr := &struct{}{} // not using
	_ = emptyArr

	av, err := (&schema.SchemaIR{Type: "array"}).ParseArrayValidations()
	if err != nil {
		t.Fatalf("ParseArrayValidations: %v", err)
	}
	if av.Min != nil || av.Max != nil {
		t.Errorf("expected empty array validations")
	}
}
