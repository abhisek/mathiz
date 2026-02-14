package llm

import (
	"testing"
)

func TestGeminiModelMapping(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gemini-flash", "gemini-2.0-flash"},
		{"gemini-pro", "gemini-2.0-pro"},
		{"gemini-2.0-flash", "gemini-2.0-flash"}, // Pass-through
	}
	for _, tt := range tests {
		got := resolveModel(tt.input, geminiModels)
		if got != tt.expected {
			t.Errorf("resolveModel(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildGeminiSchema(t *testing.T) {
	def := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"age":   map[string]any{"type": "integer"},
			"grade": map[string]any{"type": "string", "enum": []any{"A", "B", "C"}},
			"scores": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "integer"},
			},
		},
		"required": []any{"name", "age"},
	}

	schema := buildGeminiSchema(def)

	if schema.Type != "OBJECT" {
		t.Fatalf("expected OBJECT type, got %s", schema.Type)
	}
	if len(schema.Properties) != 4 {
		t.Fatalf("expected 4 properties, got %d", len(schema.Properties))
	}
	if schema.Properties["name"].Type != "STRING" {
		t.Fatalf("expected STRING for name, got %s", schema.Properties["name"].Type)
	}
	if schema.Properties["age"].Type != "INTEGER" {
		t.Fatalf("expected INTEGER for age, got %s", schema.Properties["age"].Type)
	}
	if len(schema.Properties["grade"].Enum) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(schema.Properties["grade"].Enum))
	}
	if schema.Properties["scores"].Type != "ARRAY" {
		t.Fatalf("expected ARRAY for scores, got %s", schema.Properties["scores"].Type)
	}
	if schema.Properties["scores"].Items.Type != "INTEGER" {
		t.Fatalf("expected INTEGER for scores items, got %s", schema.Properties["scores"].Items.Type)
	}
	if len(schema.Required) != 2 {
		t.Fatalf("expected 2 required fields, got %d", len(schema.Required))
	}
}
