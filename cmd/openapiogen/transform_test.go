package main

import "testing"

func TestNormalizeForOgenConvertsNullableString(t *testing.T) {
	document := map[string]any{"type": []any{"string", "null"}, "format": "date-time"}
	normalized := normalizeForOgen(document).(map[string]any)

	if normalized["type"] != "string" || normalized["nullable"] != true || normalized["format"] != "date-time" {
		t.Fatalf("normalized schema = %#v", normalized)
	}
}

func TestNormalizeForOgenConvertsNullableObjectOrArray(t *testing.T) {
	document := map[string]any{
		"type":                 []any{"object", "array", "null"},
		"additionalProperties": true,
		"items":                map[string]any{},
	}
	normalized := normalizeForOgen(document).(map[string]any)

	if normalized["nullable"] != true {
		t.Fatalf("nullable = %#v, want true", normalized["nullable"])
	}
	variants, ok := normalized["oneOf"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("oneOf = %#v", normalized["oneOf"])
	}
	object := variants[0].(map[string]any)
	array := variants[1].(map[string]any)
	if object["type"] != "object" || object["additionalProperties"] != true || array["type"] != "array" {
		t.Fatalf("variants = %#v", variants)
	}
	if _, exists := normalized["type"]; exists {
		t.Fatalf("union type was not removed: %#v", normalized)
	}
}
