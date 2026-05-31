package validator

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ---- StringValidator ----

type StringValidator struct {
	Required bool
	Min      *int
	Max      *int
	Email    *bool
	UUID     *bool
	URL      *bool
	Pattern  *string
	re       *regexp.Regexp // compiled pattern
}

func (v *StringValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		if v.Required {
			return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
		}
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return ValidationErrors{{Path: "", Code: "type", Message: "expected string"}}
	}

	var errs ValidationErrors
	errs = append(errs, v.validateString(str)...)
	return errs
}

func (v *StringValidator) validateString(str string) ValidationErrors {
	var errs ValidationErrors

	if v.Min != nil && len(str) < *v.Min {
		errs = append(errs, ValidationError{
			Code:    "min",
			Message: fmt.Sprintf("must be at least %d characters", *v.Min),
		})
	}
	if v.Max != nil && len(str) > *v.Max {
		errs = append(errs, ValidationError{
			Code:    "max",
			Message: fmt.Sprintf("must be at most %d characters", *v.Max),
		})
	}
	if v.Email != nil && *v.Email {
		if _, err := mail.ParseAddress(str); err != nil {
			errs = append(errs, ValidationError{Code: "email", Message: "invalid email address"})
		}
	}
	if v.UUID != nil && *v.UUID {
		if !isUUID(str) {
			errs = append(errs, ValidationError{Code: "uuid", Message: "invalid UUID"})
		}
	}
	if v.URL != nil && *v.URL {
		if _, err := url.ParseRequestURI(str); err != nil {
			errs = append(errs, ValidationError{Code: "url", Message: "invalid URL"})
		}
	}
	if v.Pattern != nil {
		if v.re == nil {
			v.re = regexp.MustCompile(*v.Pattern)
		}
		if !v.re.MatchString(str) {
			errs = append(errs, ValidationError{Code: "pattern", Message: "does not match required pattern"})
		}
	}

	return errs
}

// ---- NumberValidator ----

type NumberValidator struct {
	Int      *bool
	Min      *float64
	Max      *float64
	Positive *bool
	Negative *bool
}

func (v *NumberValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
	}

	var num float64
	switch n := value.(type) {
	case float64:
		num = n
	case int:
		num = float64(n)
	case int64:
		num = float64(n)
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return ValidationErrors{{Path: "", Code: "type", Message: "invalid number"}}
		}
		num = f
	default:
		return ValidationErrors{{Path: "", Code: "type", Message: "expected number"}}
	}

	var errs ValidationErrors

	if v.Int != nil && *v.Int {
		if num != float64(int64(num)) {
			errs = append(errs, ValidationError{Code: "int", Message: "must be an integer"})
		}
	}
	if v.Min != nil && num < *v.Min {
		errs = append(errs, ValidationError{
			Code:    "min",
			Message: fmt.Sprintf("must be at least %v", *v.Min),
		})
	}
	if v.Max != nil && num > *v.Max {
		errs = append(errs, ValidationError{
			Code:    "max",
			Message: fmt.Sprintf("must be at most %v", *v.Max),
		})
	}
	if v.Positive != nil && *v.Positive && num <= 0 {
		errs = append(errs, ValidationError{Code: "positive", Message: "must be positive"})
	}
	if v.Negative != nil && *v.Negative && num >= 0 {
		errs = append(errs, ValidationError{Code: "negative", Message: "must be negative"})
	}

	return errs
}

// ---- BooleanValidator ----

type BooleanValidator struct{}

func (v *BooleanValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
	}
	if _, ok := value.(bool); !ok {
		return ValidationErrors{{Path: "", Code: "type", Message: "expected boolean"}}
	}
	return nil
}

// ---- EnumValidator ----

type EnumValidator struct {
	Values []string
}

func (v *EnumValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
	}
	str, ok := value.(string)
	if !ok {
		return ValidationErrors{{Path: "", Code: "type", Message: "expected string"}}
	}
	if !slices.Contains(v.Values, str) {
		return ValidationErrors{{
			Code:    "enum",
			Message: fmt.Sprintf("must be one of: %s", strings.Join(v.Values, ", ")),
		}}
	}
	return nil
}

// ---- DateValidator ----

type DateValidator struct {
	Min *string // ISO 8601
	Max *string // ISO 8601
}

func (v *DateValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
	}
	str, ok := value.(string)
	if !ok {
		return ValidationErrors{{Path: "", Code: "type", Message: "expected date string"}}
	}

	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		// Try other common ISO 8601 formats
		t, err = time.Parse("2006-01-02T15:04:05", str)
		if err != nil {
			t, err = time.Parse("2006-01-02", str)
			if err != nil {
				return ValidationErrors{{Path: "", Code: "date", Message: "invalid date format, expected ISO 8601"}}
			}
		}
	}

	var errs ValidationErrors

	if v.Min != nil {
		minTime, err := time.Parse(time.RFC3339, *v.Min)
		if err == nil && t.Before(minTime) {
			errs = append(errs, ValidationError{Code: "min", Message: fmt.Sprintf("date must be after %s", *v.Min)})
		}
	}
	if v.Max != nil {
		maxTime, err := time.Parse(time.RFC3339, *v.Max)
		if err == nil && t.After(maxTime) {
			errs = append(errs, ValidationError{Code: "max", Message: fmt.Sprintf("date must be before %s", *v.Max)})
		}
	}

	return errs
}

// ---- ArrayValidator ----

type ArrayValidator struct {
	ItemValidator ValidatorNode
	Min           *int
	Max           *int
	Unique        *bool
}

func (v *ArrayValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
	}

	arr, ok := value.([]interface{})
	if !ok {
		return ValidationErrors{{Path: "", Code: "type", Message: "expected array"}}
	}

	var errs ValidationErrors

	if v.Min != nil && len(arr) < *v.Min {
		errs = append(errs, ValidationError{
			Code:    "min",
			Message: fmt.Sprintf("must have at least %d items", *v.Min),
		})
	}
	if v.Max != nil && len(arr) > *v.Max {
		errs = append(errs, ValidationError{
			Code:    "max",
			Message: fmt.Sprintf("must have at most %d items", *v.Max),
		})
	}

	if v.Unique != nil && *v.Unique {
		seen := make(map[interface{}]bool)
		for _, item := range arr {
			if seen[item] {
				errs = append(errs, ValidationError{Code: "unique", Message: "items must be unique"})
				break
			}
			seen[item] = true
		}
	}

	if v.ItemValidator != nil {
		for i, item := range arr {
			itemErrs := v.ItemValidator.Validate(item)
			for _, e := range itemErrs {
				e.Path = fmt.Sprintf("[%d]%s", i, e.Path)
				errs = append(errs, e)
			}
		}
	}

	return errs
}

// ---- ObjectValidator ----

type FieldValidator struct {
	Validator ValidatorNode
	Optional  bool
	Nullable  bool
}

type ObjectValidator struct {
	Fields map[string]FieldValidator
	Strict bool
}

func (v *ObjectValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return ValidationErrors{{Path: "", Code: "required", Message: "value is required"}}
	}

	obj, ok := value.(map[string]interface{})
	if !ok {
		return ValidationErrors{{Path: "", Code: "type", Message: "expected object"}}
	}

	var errs ValidationErrors

	if v.Strict {
		for key := range obj {
			if _, defined := v.Fields[key]; !defined {
				errs = append(errs, ValidationError{
					Path:    key,
					Code:    "unknown",
					Message: fmt.Sprintf("unknown field %q", key),
				})
			}
		}
	}

	// Validate known fields
	for name, field := range v.Fields {
		val, present := obj[name]

		if !present {
			if field.Optional {
				continue
			}
			if field.Nullable {
				// nullable without optional means the field must be present but can be null
				// Actually, if it's not present and not optional, it's an error
			}
			errs = append(errs, ValidationError{
				Path:    name,
				Code:    "required",
				Message: fmt.Sprintf("field %q is required", name),
			})
			continue
		}

		if val == nil && field.Nullable {
			continue
		}

		fieldErrs := field.Validator.Validate(val)
		for _, e := range fieldErrs {
			if e.Path == "" {
				e.Path = name
			} else {
				e.Path = name + "." + e.Path
			}
			errs = append(errs, e)
		}
	}

	return errs
}

// ---- NullableValidator ----

type NullableValidator struct {
	Inner ValidatorNode
}

func (v *NullableValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return nil
	}
	return v.Inner.Validate(value)
}

// ---- OptionalValidator ----

type OptionalValidator struct {
	Inner ValidatorNode
}

func (v *OptionalValidator) Validate(value interface{}) ValidationErrors {
	if value == nil {
		return nil
	}
	return v.Inner.Validate(value)
}

// ---- Helpers ----

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexChar(c) {
				return false
			}
		}
	}
	return true
}

func isHexChar(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// json.Number is not imported in the standard encoding/json
// Let's use a different approach - import encoding/json
