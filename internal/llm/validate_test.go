package llm

import (
	"encoding/json"
	"errors"
	"testing"
)

func testSchema() *Schema {
	return &Schema{
		Name:        "test-object",
		Description: "A test object",
		Definition: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string"},
				"age":   map[string]any{"type": "integer", "minimum": 0},
				"grade": map[string]any{"type": "string", "enum": []any{"A", "B", "C"}},
			},
			"required": []any{"name", "age"},
		},
	}
}

func TestValidateResponse_ValidJSON(t *testing.T) {
	raw := json.RawMessage(`{"name":"Alice","age":10,"grade":"A"}`)
	err := validateResponse(testSchema(), raw)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateResponse_ValidWithoutOptional(t *testing.T) {
	raw := json.RawMessage(`{"name":"Bob","age":8}`)
	err := validateResponse(testSchema(), raw)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateResponse_MissingRequired(t *testing.T) {
	raw := json.RawMessage(`{"name":"Charlie"}`)
	err := validateResponse(testSchema(), raw)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	var invErr *ErrInvalidResponse
	if !errors.As(err, &invErr) {
		t.Fatalf("expected ErrInvalidResponse, got: %T", err)
	}
}

func TestValidateResponse_WrongType(t *testing.T) {
	raw := json.RawMessage(`{"name":"Dave","age":"ten"}`)
	err := validateResponse(testSchema(), raw)
	if err == nil {
		t.Fatal("expected error for wrong type")
	}
	var invErr *ErrInvalidResponse
	if !errors.As(err, &invErr) {
		t.Fatalf("expected ErrInvalidResponse, got: %T", err)
	}
}

func TestValidateResponse_InvalidEnum(t *testing.T) {
	raw := json.RawMessage(`{"name":"Eve","age":9,"grade":"D"}`)
	err := validateResponse(testSchema(), raw)
	if err == nil {
		t.Fatal("expected error for invalid enum value")
	}
	var invErr *ErrInvalidResponse
	if !errors.As(err, &invErr) {
		t.Fatalf("expected ErrInvalidResponse, got: %T", err)
	}
}

func TestValidateResponse_MalformedJSON(t *testing.T) {
	raw := json.RawMessage(`{not json}`)
	err := validateResponse(testSchema(), raw)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	var invErr *ErrInvalidResponse
	if !errors.As(err, &invErr) {
		t.Fatalf("expected ErrInvalidResponse, got: %T", err)
	}
}

func TestValidateResponse_EmptyResponse(t *testing.T) {
	raw := json.RawMessage(``)
	err := validateResponse(testSchema(), raw)
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestValidateResponse_NilSchema(t *testing.T) {
	raw := json.RawMessage(`{"anything":"goes"}`)
	err := validateResponse(nil, raw)
	if err != nil {
		t.Fatalf("expected no error with nil schema, got: %v", err)
	}
}

func TestValidateResponse_NestedObjects(t *testing.T) {
	schema := &Schema{
		Name:        "test-nested",
		Description: "Nested test",
		Definition: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"student": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
					"required": []any{"name"},
				},
				"scores": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "integer"},
				},
			},
			"required": []any{"student", "scores"},
		},
	}

	valid := json.RawMessage(`{"student":{"name":"Alice"},"scores":[90,85,92]}`)
	if err := validateResponse(schema, valid); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	invalid := json.RawMessage(`{"student":{"name":"Alice"},"scores":["not","ints"]}`)
	if err := validateResponse(schema, invalid); err == nil {
		t.Fatal("expected error for wrong array item type")
	}
}
