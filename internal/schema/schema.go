package schema

import "encoding/json"

// IR represents any schema node in the Schema IR format.
type IR struct {
	Type string `json:"type"`

	// String / Number / Date / Array validations (opaque, parsed by type)
	Validations json.RawMessage `json:"validations,omitempty"`

	// Object
	Fields     map[string]FieldIR `json:"fields,omitempty"`
	FieldOrder []string           `json:"fieldOrder,omitempty"` // preserves insertion order
	Strict     bool               `json:"strict"`

	// Enum
	Values []string `json:"values,omitempty"`

	// Array
	Items *IR `json:"items,omitempty"`

	// Nullable / Optional
	Inner *IR `json:"inner,omitempty"`
}

type FieldIR struct {
	Schema      IR     `json:"schema"`
	Optional    bool   `json:"optional"`
	Nullable    bool   `json:"nullable"`
	Description string `json:"description,omitempty"`
}

// ---- Validation types (used after type assertion) ----

type StringValidations struct {
	Min     *int    `json:"min,omitempty"`
	Max     *int    `json:"max,omitempty"`
	Email   *bool   `json:"email,omitempty"`
	UUID    *bool   `json:"uuid,omitempty"`
	URL     *bool   `json:"url,omitempty"`
	Pattern *string `json:"pattern,omitempty"`
}

type NumberValidations struct {
	Int      *bool    `json:"int,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Positive *bool    `json:"positive,omitempty"`
	Negative *bool    `json:"negative,omitempty"`
}

type DateValidations struct {
	Min *string `json:"min,omitempty"`
	Max *string `json:"max,omitempty"`
}

type ArrayValidations struct {
	Min    *int  `json:"min,omitempty"`
	Max    *int  `json:"max,omitempty"`
	Unique *bool `json:"unique,omitempty"`
}

// ---- Helpers ----

func (s *IR) ParseStringValidations() (*StringValidations, error) {
	if s.Type != "string" || len(s.Validations) == 0 {
		return &StringValidations{}, nil
	}
	var v StringValidations
	if err := json.Unmarshal(s.Validations, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *IR) ParseNumberValidations() (*NumberValidations, error) {
	if s.Type != "number" || len(s.Validations) == 0 {
		return &NumberValidations{}, nil
	}
	var v NumberValidations
	if err := json.Unmarshal(s.Validations, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *IR) ParseDateValidations() (*DateValidations, error) {
	if s.Type != "date" || len(s.Validations) == 0 {
		return &DateValidations{}, nil
	}
	var v DateValidations
	if err := json.Unmarshal(s.Validations, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *IR) ParseArrayValidations() (*ArrayValidations, error) {
	if s.Type != "array" || len(s.Validations) == 0 {
		return &ArrayValidations{}, nil
	}
	var v ArrayValidations
	if err := json.Unmarshal(s.Validations, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
