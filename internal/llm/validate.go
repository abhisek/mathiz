package llm

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// schemaCache caches compiled JSON schemas by name.
var schemaCache sync.Map // map[string]*jsonschema.Schema

// validateResponse validates raw JSON against the given Schema.
// Returns nil if no schema is provided or validation passes.
// Returns *ErrInvalidResponse on failure.
func validateResponse(schema *Schema, raw json.RawMessage) error {
	if schema == nil {
		return nil
	}

	// Parse JSON first.
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return &ErrInvalidResponse{
			Content: raw,
			Err:     fmt.Errorf("invalid JSON: %w", err),
		}
	}

	// Get or compile the schema.
	compiled, err := getCompiledSchema(schema)
	if err != nil {
		return &ErrInvalidResponse{
			Content: raw,
			Err:     fmt.Errorf("compile schema %q: %w", schema.Name, err),
		}
	}

	// Validate against schema.
	if err := compiled.Validate(parsed); err != nil {
		return &ErrInvalidResponse{
			Content: raw,
			Err:     fmt.Errorf("schema validation failed: %w", err),
		}
	}

	return nil
}

// getCompiledSchema returns a cached compiled schema or compiles and caches it.
func getCompiledSchema(schema *Schema) (*jsonschema.Schema, error) {
	if cached, ok := schemaCache.Load(schema.Name); ok {
		return cached.(*jsonschema.Schema), nil
	}

	// The jsonschema library expects a parsed JSON value (any), not raw bytes.
	// Marshal then unmarshal to get a clean any representation.
	defBytes, err := json.Marshal(schema.Definition)
	if err != nil {
		return nil, fmt.Errorf("marshal schema definition: %w", err)
	}
	var defParsed any
	if err := json.Unmarshal(defBytes, &defParsed); err != nil {
		return nil, fmt.Errorf("parse schema definition: %w", err)
	}

	c := jsonschema.NewCompiler()
	schemaURL := fmt.Sprintf("schema://%s.json", schema.Name)
	if err := c.AddResource(schemaURL, defParsed); err != nil {
		return nil, fmt.Errorf("add resource: %w", err)
	}

	compiled, err := c.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}

	schemaCache.Store(schema.Name, compiled)
	return compiled, nil
}
