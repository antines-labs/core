package ipc

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/antines-labs/core/internal/schema"
)

func mustLayout(t *testing.T, rawJSON string) *CompiledLayout {
	t.Helper()
	var s schema.SchemaIR
	if err := json.Unmarshal([]byte(rawJSON), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	l, err := CalculateLayout(&s)
	if err != nil {
		t.Fatalf("CalculateLayout: %v", err)
	}
	return l
}

// ---- Layout calculation tests ----

func TestLayoutEmptyObject(t *testing.T) {
	l := mustLayout(t, `{"type":"object","fields":{},"strict":false}`)
	if l.FixedSize != 0 {
		t.Errorf("expected FixedSize=0, got %d", l.FixedSize)
	}
	if l.BitmaskSize != 0 {
		t.Errorf("expected BitmaskSize=0, got %d", l.BitmaskSize)
	}
	if l.VariableCount != 0 {
		t.Errorf("expected VariableCount=0, got %d", l.VariableCount)
	}
}

func TestLayoutFixedFields(t *testing.T) {
	l := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["age","active","role","createdAt"],
		"fields":{
			"age":{"schema":{"type":"number"},"optional":false,"nullable":false},
			"active":{"schema":{"type":"boolean"},"optional":false,"nullable":false},
			"role":{"schema":{"type":"enum","values":["a","b"]},"optional":false,"nullable":false},
			"createdAt":{"schema":{"type":"date"},"optional":false,"nullable":false}
		}
	}`)

	// 8 (number) + 1 (boolean) + 2 (enum) + 8 (date) = 19
	if l.FixedSize != 19 {
		t.Errorf("expected FixedSize=19, got %d", l.FixedSize)
	}
	if l.VariableCount != 0 {
		t.Errorf("expected VariableCount=0, got %d", l.VariableCount)
	}
	if l.BitmaskSize != 0 {
		t.Errorf("expected BitmaskSize=0, got %d", l.BitmaskSize)
	}

	if len(l.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(l.Fields))
	}

	// age: fixed, offset 0, size 8
	f := l.Fields[0]
	if f.Offset != 0 || f.Size != 8 || f.Category != FieldFixed {
		t.Errorf("age: offset=%d size=%d cat=%d", f.Offset, f.Size, f.Category)
	}

	// active: fixed, offset 8, size 1
	f = l.Fields[1]
	if f.Offset != 8 || f.Size != 1 {
		t.Errorf("active: offset=%d size=%d", f.Offset, f.Size)
	}

	// role: fixed, offset 9, size 2
	f = l.Fields[2]
	if f.Offset != 9 || f.Size != 2 {
		t.Errorf("role: offset=%d size=%d", f.Offset, f.Size)
	}

	// createdAt: fixed, offset 11, size 8
	f = l.Fields[3]
	if f.Offset != 11 || f.Size != 8 {
		t.Errorf("createdAt: offset=%d size=%d", f.Offset, f.Size)
	}
}

func TestLayoutVariableFields(t *testing.T) {
	l := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["name","tags"],
		"fields":{
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"tags":{"schema":{"type":"array","items":{"type":"string"}},"optional":true,"nullable":false}
		}
	}`)

	if l.FixedSize != 0 {
		t.Errorf("expected FixedSize=0, got %d", l.FixedSize)
	}
	if l.VariableCount != 2 {
		t.Errorf("expected VariableCount=2, got %d", l.VariableCount)
	}
	if l.BitmaskSize != 1 {
		t.Errorf("expected BitmaskSize=1, got %d", l.BitmaskSize)
	}

	// name: variable, not optional
	if l.Fields[0].BitmaskBit != -1 {
		t.Errorf("expected name bitmaskBit=-1, got %d", l.Fields[0].BitmaskBit)
	}

	// tags: variable, optional, bit 0
	if l.Fields[1].BitmaskBit != 0 {
		t.Errorf("expected tags bitmaskBit=0, got %d", l.Fields[1].BitmaskBit)
	}
}

func TestLayoutMixedFields(t *testing.T) {
	l := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["id","age","name","email"],
		"fields":{
			"id":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"age":{"schema":{"type":"number"},"optional":true,"nullable":false},
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"email":{"schema":{"type":"string"},"optional":true,"nullable":false}
		}
	}`)

	// Fixed: age (8 bytes)
	if l.FixedSize != 8 {
		t.Errorf("expected FixedSize=8, got %d", l.FixedSize)
	}
	// Variable: id, name, email
	if l.VariableCount != 3 {
		t.Errorf("expected VariableCount=3, got %d", l.VariableCount)
	}
	// 2 optional fields → 1 byte bitmask
	if l.BitmaskSize != 1 {
		t.Errorf("expected BitmaskSize=1, got %d", l.BitmaskSize)
	}

	// age: fixed, bit 0
	if l.Fields[1].BitmaskBit != 0 {
		t.Errorf("expected age bitmaskBit=0, got %d", l.Fields[1].BitmaskBit)
	}
	// email: variable, bit 1
	if l.Fields[3].BitmaskBit != 1 {
		t.Errorf("expected email bitmaskBit=1, got %d", l.Fields[3].BitmaskBit)
	}
}

// ---- Serialization tests ----

func TestSerializeDeserializeFixedOnly(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["age","active"],
		"fields":{
			"age":{"schema":{"type":"number"},"optional":false,"nullable":false},
			"active":{"schema":{"type":"boolean"},"optional":false,"nullable":false}
		}
	}`)

	input := map[string]interface{}{
		"age":    float64(30),
		"active": true,
	}

	data, err := SerializeInput(layout, input)
	if err != nil {
		t.Fatalf("SerializeInput: %v", err)
	}

	// Expected: bitmask empty + age (8 bytes) + active (1 byte) = 9 bytes
	if len(data) != 9 {
		t.Errorf("expected 9 bytes, got %d", len(data))
	}

	output, err := DeserializeOutput(layout, data)
	if err != nil {
		t.Fatalf("DeserializeOutput: %v", err)
	}

	age, ok := output["age"].(float64)
	if !ok || age != 30 {
		t.Errorf("expected age=30, got %v", output["age"])
	}

	active, ok := output["active"].(bool)
	if !ok || !active {
		t.Errorf("expected active=true, got %v", output["active"])
	}
}

func TestSerializeDeserializeNumber(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fields":{
			"value":{"schema":{"type":"number"},"optional":false,"nullable":false}
		}
	}`)

	testCases := []float64{0, -1, 3.14, math.MaxFloat64, 42}
	for _, tc := range testCases {
		data, err := SerializeInput(layout, map[string]interface{}{"value": tc})
		if err != nil {
			t.Fatalf("SerializeInput(%v): %v", tc, err)
		}

		output, err := DeserializeOutput(layout, data)
		if err != nil {
			t.Fatalf("DeserializeOutput(%v): %v", tc, err)
		}

		got, ok := output["value"].(float64)
		if !ok || got != tc {
			t.Errorf("value=%v: expected %v, got %v", tc, tc, got)
		}
	}
}

func TestSerializeDeserializeBoolean(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fields":{
			"flag":{"schema":{"type":"boolean"},"optional":false,"nullable":false}
		}
	}`)

	for _, tc := range []bool{true, false} {
		data, err := SerializeInput(layout, map[string]interface{}{"flag": tc})
		if err != nil {
			t.Fatalf("SerializeInput(%v): %v", tc, err)
		}

		output, err := DeserializeOutput(layout, data)
		if err != nil {
			t.Fatalf("DeserializeOutput(%v): %v", tc, err)
		}

		got, ok := output["flag"].(bool)
		if !ok || got != tc {
			t.Errorf("flag=%v: expected %v, got %v", tc, tc, got)
		}
	}
}

func TestSerializeDeserializeString(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fields":{
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false}
		}
	}`)

	testCases := []string{"hello", "", "a longer string with spaces"}
	for _, tc := range testCases {
		data, err := SerializeInput(layout, map[string]interface{}{"name": tc})
		if err != nil {
			t.Fatalf("SerializeInput(%q): %v", tc, err)
		}

		output, err := DeserializeOutput(layout, data)
		if err != nil {
			t.Fatalf("DeserializeOutput(%q): %v", tc, err)
		}

		got, ok := output["name"].(string)
		if !ok || got != tc {
			t.Errorf("name=%q: expected %q, got %q", tc, tc, got)
		}
	}
}

func TestSerializeDeserializeOptionalPresent(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["name","email"],
		"fields":{
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"email":{"schema":{"type":"string"},"optional":true,"nullable":false}
		}
	}`)

	input := map[string]interface{}{
		"name":  "Alice",
		"email": "alice@example.com",
	}

	data, err := SerializeInput(layout, input)
	if err != nil {
		t.Fatalf("SerializeInput: %v", err)
	}

	output, err := DeserializeOutput(layout, data)
	if err != nil {
		t.Fatalf("DeserializeOutput: %v", err)
	}

	if output["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", output["name"])
	}
	if output["email"] != "alice@example.com" {
		t.Errorf("expected email=alice@example.com, got %v", output["email"])
	}
}

func TestSerializeDeserializeOptionalAbsent(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["name","email"],
		"fields":{
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"email":{"schema":{"type":"string"},"optional":true,"nullable":false}
		}
	}`)

	input := map[string]interface{}{
		"name": "Alice",
		// email absent
	}

	data, err := SerializeInput(layout, input)
	if err != nil {
		t.Fatalf("SerializeInput: %v", err)
	}

	output, err := DeserializeOutput(layout, data)
	if err != nil {
		t.Fatalf("DeserializeOutput: %v", err)
	}

	if output["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", output["name"])
	}
	if _, exists := output["email"]; exists {
		t.Errorf("expected email to be absent, got %v", output["email"])
	}
}

func TestSerializeDeserializeDate(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fields":{
			"ts":{"schema":{"type":"date"},"optional":false,"nullable":false}
		}
	}`)

	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	input := map[string]interface{}{
		"ts": now.Format(time.RFC3339),
	}

	data, err := SerializeInput(layout, input)
	if err != nil {
		t.Fatalf("SerializeInput: %v", err)
	}

	output, err := DeserializeOutput(layout, data)
	if err != nil {
		t.Fatalf("DeserializeOutput: %v", err)
	}

	got, ok := output["ts"].(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T", output["ts"])
	}
	if !got.Equal(now) {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestSerializeDeserializeMixed(t *testing.T) {
	layout := mustLayout(t, `{
		"type":"object","strict":false,
		"fieldOrder":["id","age","name","active","email"],
		"fields":{
			"id":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"age":{"schema":{"type":"number","validations":{"int":true}},"optional":true,"nullable":false},
			"name":{"schema":{"type":"string"},"optional":false,"nullable":false},
			"active":{"schema":{"type":"boolean"},"optional":false,"nullable":false},
			"email":{"schema":{"type":"string"},"optional":true,"nullable":false}
		}
	}`)

	input := map[string]interface{}{
		"id":     "abc-123",
		"age":    float64(30),
		"name":   "Alice",
		"active": true,
		// email absent
	}

	data, err := SerializeInput(layout, input)
	if err != nil {
		t.Fatalf("SerializeInput: %v", err)
	}

	output, err := DeserializeOutput(layout, data)
	if err != nil {
		t.Fatalf("DeserializeOutput: %v", err)
	}

	if output["id"] != "abc-123" {
		t.Errorf("id: expected abc-123, got %v", output["id"])
	}
	if output["age"] != float64(30) {
		t.Errorf("age: expected 30, got %v", output["age"])
	}
	if output["name"] != "Alice" {
		t.Errorf("name: expected Alice, got %v", output["name"])
	}
	if output["active"] != true {
		t.Errorf("active: expected true, got %v", output["active"])
	}
	if _, exists := output["email"]; exists {
		t.Errorf("email: expected absent, got %v", output["email"])
	}
}

// ---- Header tests ----

func TestHeaderEncodeDecode(t *testing.T) {
	h := NewHeader(DirGoToJS, MsgDispatch, 42, 7, 200, 1024)

	buf := h.Encode()
	decoded, err := DecodeHeader(buf)
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}

	if decoded.Magic != Magic {
		t.Errorf("Magic: expected 0x%X, got 0x%X", Magic, decoded.Magic)
	}
	if decoded.Version != Version {
		t.Errorf("Version: expected %d, got %d", Version, decoded.Version)
	}
	if decoded.Direction != DirGoToJS {
		t.Errorf("Direction: expected %d, got %d", DirGoToJS, decoded.Direction)
	}
	if decoded.MsgType != MsgDispatch {
		t.Errorf("MsgType: expected %d, got %d", MsgDispatch, decoded.MsgType)
	}
	if decoded.RequestID != 42 {
		t.Errorf("RequestID: expected 42, got %d", decoded.RequestID)
	}
	if decoded.HandlerID != 7 {
		t.Errorf("HandlerID: expected 7, got %d", decoded.HandlerID)
	}
	if decoded.PayloadLen != 1024 {
		t.Errorf("PayloadLen: expected 1024, got %d", decoded.PayloadLen)
	}
	if decoded.StatusCode != 200 {
		t.Errorf("StatusCode: expected 200, got %d", decoded.StatusCode)
	}
}

func TestHeaderInvalidMagic(t *testing.T) {
	var buf [32]byte
	// Write bad magic
	binary.LittleEndian.PutUint32(buf[0:4], 0xDEADBEEF)

	_, err := DecodeHeader(buf)
	if err == nil {
		t.Fatal("expected error for invalid magic")
	}
}
