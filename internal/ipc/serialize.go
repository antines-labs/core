package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// SerializeInput serializes validated input data (map[string]interface{}) into
// the positional wire format for dispatch to JS.
func SerializeInput(layout *CompiledLayout, data map[string]interface{}) ([]byte, error) {
	if layout == nil {
		return nil, fmt.Errorf("serialize: nil layout")
	}

	bitmaskSize := int(layout.BitmaskSize)
	fixedSize := int(layout.FixedSize)
	varCount := layout.VariableCount

	// allocate: bitmask + fixed + offset table + variable data
	offsetTableSize := varCount * 8 // each entry: 4 offset + 4 length
	buf := make([]byte, bitmaskSize+fixedSize+offsetTableSize)

	// track variable data positions
	// absent optional fields use sentinel 0xFFFFFFFF
	variableOffsets := make([]uint32, varCount)
	for i := range variableOffsets {
		variableOffsets[i] = sentinelAbsent
	}
	var variableData []byte

	bitmask := make([]byte, bitmaskSize)

	var varIdx int
	for _, f := range layout.Fields {
		val, exists := data[f.Name]

		if !exists || val == nil {
			if f.IsOptional {
				if f.Category == FieldVariable {
					varIdx++
				}
				continue
			}
			val = zeroValueForType(f.FieldType)
		}

		if f.IsOptional && f.BitmaskBit >= 0 {
			byteIdx := f.BitmaskBit / 8
			bitIdx := uint(f.BitmaskBit % 8)
			bitmask[byteIdx] |= 1 << bitIdx
		}

		if f.Category == FieldFixed {
			offset := int(f.Offset) + bitmaskSize
			enc := encodeFixedField(f.FieldType, val, int(f.Size))
			copy(buf[offset:offset+int(f.Size)], enc)
		} else {
			enc := encodeVariableField(f.FieldType, val)
			variableOffsets[varIdx] = uint32(len(variableData))
			variableData = append(variableData, enc...)
			varIdx++
		}
	}

	for i := 0; i < varCount; i++ {
		offset := bitmaskSize + fixedSize + i*8
		start := variableOffsets[i]
		var length uint32
		if start != sentinelAbsent {
			if i+1 < varCount && variableOffsets[i+1] != sentinelAbsent {
				length = variableOffsets[i+1] - start
			} else {
				length = uint32(len(variableData)) - start
			}
		}
		binary.LittleEndian.PutUint32(buf[offset:offset+4], start)
		binary.LittleEndian.PutUint32(buf[offset+4:offset+8], length)
	}

	copy(buf[:bitmaskSize], bitmask)

	buf = append(buf, variableData...)

	return buf, nil
}

// DeserializeOutput deserializes the positional wire format from JS result
// into a map[string]interface{}.
func DeserializeOutput(layout *CompiledLayout, data []byte) (map[string]interface{}, error) {
	if layout == nil {
		return nil, fmt.Errorf("deserialize: nil layout")
	}

	result := make(map[string]interface{})
	bitmaskSize := int(layout.BitmaskSize)
	fixedSize := int(layout.FixedSize)
	varCount := layout.VariableCount

	bitmask := data[:bitmaskSize]

	var varIdx int
	for _, f := range layout.Fields {
		if f.IsOptional && f.BitmaskBit >= 0 {
			byteIdx := f.BitmaskBit / 8
			bitIdx := uint(f.BitmaskBit % 8)
			if byteIdx >= len(bitmask) || (bitmask[byteIdx]&(1<<bitIdx)) == 0 {
				if f.Category == FieldVariable {
					varIdx++
				}
				continue
			}
		}

		if f.Category == FieldFixed {
			offset := bitmaskSize + int(f.Offset)
			val, err := decodeFixedField(f.FieldType, data[offset:offset+int(f.Size)])
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", f.Name, err)
			}
			result[f.Name] = val
		} else {
			tableOffset := bitmaskSize + fixedSize + varIdx*8
			start := binary.LittleEndian.Uint32(data[tableOffset : tableOffset+4])
			length := binary.LittleEndian.Uint32(data[tableOffset+4 : tableOffset+8])

			var val interface{}
			if start != sentinelAbsent {
				if length > 0 {
					variableStart := uint32(bitmaskSize + fixedSize + varCount*8)
					val = decodeVariableField(f.FieldType, data[variableStart+start:variableStart+start+length])
				} else {
					val = zeroValueForWire(f.FieldType)
				}
			}
			result[f.Name] = val
			result[f.Name] = val
			varIdx++
		}
	}

	return result, nil
}

// ---- Fixed field encoding ----

func encodeFixedField(ft FieldType, val interface{}, size int) []byte {
	buf := make([]byte, size)

	switch ft {
	case TypeNumber:
		var f float64
		switch v := val.(type) {
		case float64:
			f = v
		case int:
			f = float64(v)
		case int64:
			f = float64(v)
		default:
			f = 0
		}
		binary.LittleEndian.PutUint64(buf, math.Float64bits(f))

	case TypeBoolean:
		if b, ok := val.(bool); ok && b {
			buf[0] = 1
		}

	case TypeEnum:
		var idx uint16
		if s, ok := val.(string); ok {
			// The caller is expected to pass the enum index as uint16 or string
			// For now, encode string as 0 (caller should pre-convert)
			_ = s
		}
		if i, ok := val.(float64); ok {
			idx = uint16(i)
		}
		binary.LittleEndian.PutUint16(buf, idx)

	case TypeDate:
		var ms int64
		switch v := val.(type) {
		case string:
			t, err := time.Parse(time.RFC3339, v)
			if err == nil {
				ms = t.UnixMilli()
			}
		case time.Time:
			ms = v.UnixMilli()
		case float64:
			ms = int64(v)
		}
		binary.LittleEndian.PutUint64(buf, uint64(ms))
	}

	return buf
}

func decodeFixedField(ft FieldType, data []byte) (interface{}, error) {
	switch ft {
	case TypeNumber:
		bits := binary.LittleEndian.Uint64(data)
		return math.Float64frombits(bits), nil
	case TypeBoolean:
		return data[0] != 0, nil
	case TypeEnum:
		idx := binary.LittleEndian.Uint16(data)
		return float64(idx), nil
	case TypeDate:
		ms := int64(binary.LittleEndian.Uint64(data))
		return time.UnixMilli(ms).UTC(), nil
	default:
		return nil, fmt.Errorf("unsupported fixed field type: %d", ft)
	}
}

// ---- Variable field encoding ----

func encodeVariableField(ft FieldType, val interface{}) []byte {
	switch ft {
	case TypeString:
		if s, ok := val.(string); ok {
			return []byte(s)
		}
		return []byte(fmt.Sprintf("%v", val))
	case TypeArray, TypeObject:
		// For now: JSON encode nested data
		// Future: recursive positional encoding for objects
		if b, err := json.Marshal(val); err == nil {
			return b
		}
		return []byte{}
	default:
		return []byte(fmt.Sprintf("%v", val))
	}
}

func decodeVariableField(ft FieldType, data []byte) interface{} {
	switch ft {
	case TypeString:
		return string(data)
	case TypeArray, TypeObject:
		var v interface{}
		if err := json.Unmarshal(data, &v); err == nil {
			return v
		}
		return string(data)
	default:
		return string(data)
	}
}

// ---- Helpers ----

const sentinelAbsent = 0xFFFFFFFF

func zeroValueForType(ft FieldType) interface{} {
	switch ft {
	case TypeNumber:
		return float64(0)
	case TypeBoolean:
		return false
	case TypeEnum:
		return float64(0)
	case TypeDate:
		return int64(0)
	default:
		return ""
	}
}

func zeroValueForWire(ft FieldType) interface{} {
	switch ft {
	case TypeString:
		return ""
	case TypeArray:
		return []interface{}{}
	case TypeObject:
		return map[string]interface{}{}
	default:
		return nil
	}
}
