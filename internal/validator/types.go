package validator

import (
	"fmt"
	"strings"

	"github.com/antines/core/internal/schema"
)

// ValidationError describes a single validation failure.
type ValidationError struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", e.Path, e.Message, e.Code)
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "validation passed"
	}
	msgs := make([]string, len(ve))
	for i, e := range ve {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// ValidatorNode is the interface for all schema validators.
type ValidatorNode interface {
	// Validate checks the value against the schema.
	// Returns nil if valid, or a slice of ValidationErrors.
	Validate(value interface{}) ValidationErrors
}

// Compile converts a SchemaIR into a ValidatorNode tree.
func Compile(s *schema.SchemaIR) (ValidatorNode, error) {
	if s == nil {
		return nil, fmt.Errorf("validator: cannot compile nil schema")
	}

	switch s.Type {
	case "string":
		return compileString(s)
	case "number":
		return compileNumber(s)
	case "boolean":
		return &BooleanValidator{}, nil
	case "enum":
		return compileEnum(s)
	case "date":
		return compileDate(s)
	case "array":
		return compileArray(s)
	case "object":
		return compileObject(s)
	case "nullable":
		return compileNullable(s)
	case "optional":
		return compileOptional(s)
	default:
		return nil, fmt.Errorf("validator: unknown schema type: %q", s.Type)
	}
}

// ---- Compilation helpers ----

func compileString(s *schema.SchemaIR) (*StringValidator, error) {
	v := &StringValidator{}
	vals, err := s.ParseStringValidations()
	if err != nil {
		return nil, fmt.Errorf("validator: string: %w", err)
	}
	if vals != nil {
		v.Required = true
		v.Min = vals.Min
		v.Max = vals.Max
		v.Email = vals.Email
		v.UUID = vals.UUID
		v.URL = vals.URL
		v.Pattern = vals.Pattern
	}
	return v, nil
}

func compileNumber(s *schema.SchemaIR) (*NumberValidator, error) {
	v := &NumberValidator{}
	vals, err := s.ParseNumberValidations()
	if err != nil {
		return nil, fmt.Errorf("validator: number: %w", err)
	}
	if vals != nil {
		v.Int = vals.Int
		v.Min = vals.Min
		v.Max = vals.Max
		v.Positive = vals.Positive
		v.Negative = vals.Negative
	}
	return v, nil
}

func compileEnum(s *schema.SchemaIR) (*EnumValidator, error) {
	if len(s.Values) == 0 {
		return nil, fmt.Errorf("validator: enum must have at least one value")
	}
	return &EnumValidator{Values: s.Values}, nil
}

func compileDate(s *schema.SchemaIR) (*DateValidator, error) {
	v := &DateValidator{}
	vals, err := s.ParseDateValidations()
	if err != nil {
		return nil, fmt.Errorf("validator: date: %w", err)
	}
	if vals != nil {
		v.Min = vals.Min
		v.Max = vals.Max
	}
	return v, nil
}

func compileArray(s *schema.SchemaIR) (*ArrayValidator, error) {
	itemValidator, err := Compile(s.Items)
	if err != nil {
		return nil, fmt.Errorf("validator: array items: %w", err)
	}

	vals, err := s.ParseArrayValidations()
	if err != nil {
		return nil, fmt.Errorf("validator: array: %w", err)
	}

	v := &ArrayValidator{
		ItemValidator: itemValidator,
	}
	if vals != nil {
		v.Min = vals.Min
		v.Max = vals.Max
		v.Unique = vals.Unique
	}
	return v, nil
}

func compileObject(s *schema.SchemaIR) (*ObjectValidator, error) {
	v := &ObjectValidator{
		Fields: make(map[string]FieldValidator),
		Strict: s.Strict,
	}

	for name, field := range s.Fields {
		innerValidator, err := Compile(&field.Schema)
		if err != nil {
			return nil, fmt.Errorf("validator: object field %q: %w", name, err)
		}

		v.Fields[name] = FieldValidator{
			Validator: innerValidator,
			Optional:  field.Optional,
			Nullable:  field.Nullable,
		}
	}

	return v, nil
}

func compileNullable(s *schema.SchemaIR) (*NullableValidator, error) {
	if s.Inner == nil {
		return nil, fmt.Errorf("validator: nullable must have inner schema")
	}
	inner, err := Compile(s.Inner)
	if err != nil {
		return nil, fmt.Errorf("validator: nullable inner: %w", err)
	}
	return &NullableValidator{Inner: inner}, nil
}

func compileOptional(s *schema.SchemaIR) (*OptionalValidator, error) {
	if s.Inner == nil {
		return nil, fmt.Errorf("validator: optional must have inner schema")
	}
	inner, err := Compile(s.Inner)
	if err != nil {
		return nil, fmt.Errorf("validator: optional inner: %w", err)
	}
	return &OptionalValidator{Inner: inner}, nil
}
