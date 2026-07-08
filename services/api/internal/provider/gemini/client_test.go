package gemini

import (
	"encoding/json"
	"testing"
)

func TestSanitizeGeminiSchemaStripsUnsupportedKeys(t *testing.T) {
	raw := `{
		"type": "object",
		"$schema": "http://json-schema.org/draft-07/schema#",
		"additionalProperties": false,
		"properties": {
			"query": {"type": "string", "description": "search text"},
			"filters": {
				"type": "array",
				"items": {
					"type": "object",
					"additionalProperties": true,
					"properties": {
						"key": {"type": "string"},
						"value": {"type": "string", "const": "x"}
					}
				}
			}
		},
		"required": ["query"]
	}`
	var schema any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	sanitized := sanitizeGeminiSchema(schema)
	b, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal sanitized: %v", err)
	}

	for _, key := range []string{"additionalProperties", "$schema"} {
		if _, ok := out[key]; ok {
			t.Errorf("expected top-level %q to be stripped", key)
		}
	}

	props, ok := out["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties to survive as a map, got %T", out["properties"])
	}
	filters, ok := props["filters"].(map[string]any)
	if !ok {
		t.Fatalf("expected filters to survive as a map, got %T", props["filters"])
	}
	items, ok := filters["items"].(map[string]any)
	if !ok {
		t.Fatalf("expected filters.items to survive as a map, got %T", filters["items"])
	}
	if _, ok := items["additionalProperties"]; ok {
		t.Error("expected nested additionalProperties (inside array items) to be stripped")
	}
	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected filters.items.properties to survive as a map, got %T", items["properties"])
	}
	value, ok := itemProps["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected filters.items.properties.value to survive as a map, got %T", itemProps["value"])
	}
	if _, ok := value["const"]; ok {
		t.Error("expected nested const to be stripped")
	}

	// Fields Gemini does support must survive untouched.
	if out["type"] != "object" {
		t.Errorf("expected type to survive, got %v", out["type"])
	}
	required, ok := out["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "query" {
		t.Errorf("expected required:[query] to survive, got %v", out["required"])
	}
}

func TestSanitizeGeminiSchemaPassesThroughScalarsAndNil(t *testing.T) {
	if got := sanitizeGeminiSchema(nil); got != nil {
		t.Errorf("expected nil to pass through unchanged, got %v", got)
	}
	if got := sanitizeGeminiSchema("plain string"); got != "plain string" {
		t.Errorf("expected scalar to pass through unchanged, got %v", got)
	}
}
