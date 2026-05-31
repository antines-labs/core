package ipc

import (
	"fmt"
	"sort"

	"github.com/antines-labs/core/internal/schema"
)

// FieldCategory indicates whether a field has fixed or variable size.
type FieldCategory uint8

const (
	FieldFixed    FieldCategory = 0
	FieldVariable FieldCategory = 1
)

// FieldType describes the data type of a field.
type FieldType uint8

const (
	TypeString  FieldType = 0
	TypeNumber  FieldType = 1
	TypeBoolean FieldType = 2
	TypeEnum    FieldType = 3
	TypeDate    FieldType = 4
	TypeArray   FieldType = 5
	TypeObject  FieldType = 6
)

// FieldLayout describes the layout of a single field in the wire format.
type FieldLayout struct {
	Name       string
	FieldType  FieldType
	Category   FieldCategory
	Offset     uint32 // byte offset in fixed section, or index in offset table
	Size       uint32 // byte size (0 for variable fields)
	BitmaskBit int    // -1 if not optional
	IsOptional bool
	IsNullable bool
}

// CompiledLayout holds the complete wire format layout for a schema.
type CompiledLayout struct {
	Fields        []FieldLayout
	FixedSize     uint32
	BitmaskSize   uint32
	VariableCount int
}

// fieldTypeAndSize resolves a SchemaIR node to its wire type and size.
func fieldTypeAndSize(s *schema.SchemaIR) (FieldType, FieldCategory, uint32, error) {
	switch s.Type {
	case "string":
		return TypeString, FieldVariable, 0, nil
	case "number":
		return TypeNumber, FieldFixed, 8, nil
	case "boolean":
		return TypeBoolean, FieldFixed, 1, nil
	case "enum":
		return TypeEnum, FieldFixed, 2, nil
	case "date":
		return TypeDate, FieldFixed, 8, nil
	case "array":
		return TypeArray, FieldVariable, 0, nil
	case "object":
		return TypeObject, FieldVariable, 0, nil
	case "nullable":
		if s.Inner == nil {
			return 0, 0, 0, fmt.Errorf("nullable: missing inner schema")
		}
		innerType, innerCat, innerSize, err := fieldTypeAndSize(s.Inner)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("nullable: %w", err)
		}
		if innerCat == FieldFixed {
			return innerType, FieldFixed, innerSize + 1, nil // +1 sentinel byte
		}
		return innerType, FieldVariable, 0, nil
	case "optional":
		if s.Inner == nil {
			return 0, 0, 0, fmt.Errorf("optional: missing inner schema")
		}
		// In object field context: bitmask handles optionality, so unwrap to inner type.
		// In wrapper context (e.g., array items): stays variable with sentinel byte.
		// For IPC object fields, the bitmask already tracks presence, so we just
		// delegate to the inner type.
		return fieldTypeAndSize(s.Inner)
	default:
		return 0, 0, 0, fmt.Errorf("unknown schema type: %q", s.Type)
	}
}

// fieldOrdered returns field names in insertion order (fieldOrder) if available,
// falling back to alphabetically sorted names (deterministic).
func fieldOrdered(s *schema.SchemaIR) []string {
	if len(s.FieldOrder) > 0 {
		return s.FieldOrder
	}
	names := make([]string, 0, len(s.Fields))
	for name := range s.Fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CalculateLayout computes the wire format layout from a SchemaIR object schema.
func CalculateLayout(s *schema.SchemaIR) (*CompiledLayout, error) {
	if s.Type != "object" {
		return nil, fmt.Errorf("calculateLayout: expected object schema, got %q", s.Type)
	}

	var fields []FieldLayout
	var fixedSize uint32
	var bitmaskBit int
	var variableCount int

	for _, name := range fieldOrdered(s) {
		f := s.Fields[name]
		ft, cat, size, err := fieldTypeAndSize(&f.Schema)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", name, err)
		}

		fl := FieldLayout{
			Name:       name,
			FieldType:  ft,
			Category:   cat,
			IsOptional: f.Optional,
			IsNullable: f.Nullable,
		}

		if cat == FieldFixed {
			fl.Offset = fixedSize
			fl.Size = size
			fixedSize += size
		} else {
			fl.Offset = uint32(variableCount)
			fl.Size = 0
			variableCount++
		}

		if f.Optional {
			fl.BitmaskBit = bitmaskBit
			bitmaskBit++
		} else {
			fl.BitmaskBit = -1
		}

		fields = append(fields, fl)
	}

	var bitmaskSize uint32
	if bitmaskBit > 0 {
		bitmaskSize = uint32((bitmaskBit + 7) / 8)
	}

	return &CompiledLayout{
		Fields:        fields,
		FixedSize:     fixedSize,
		BitmaskSize:   bitmaskSize,
		VariableCount: variableCount,
	}, nil
}
