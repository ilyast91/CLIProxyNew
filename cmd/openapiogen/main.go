// Команда openapiogen создаёт compatibility projection OpenAPI 3.1 для ogen.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func main() {
	input := flag.String("in", "openapi.yaml", "путь к исходной OpenAPI YAML-спецификации")
	output := flag.String("out", "internal/openapi/ogen/openapi.compat.yaml", "путь к compatibility projection для ogen")
	flag.Parse()

	raw, err := os.ReadFile(*input)
	if err != nil {
		fail("read input", err)
	}
	var document any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		fail("parse YAML", err)
	}
	encoded, err := yaml.Marshal(normalizeForOgen(document))
	if err != nil {
		fail("encode YAML", err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fail("create output directory", err)
	}
	if err := os.WriteFile(*output, encoded, 0o644); err != nil {
		fail("write output", err)
	}
}

func normalizeForOgen(value any) any {
	switch typed := value.(type) {
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = normalizeForOgen(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeForOgen(item)
		}
		return normalizeUnionType(out)
	default:
		return value
	}
}

func normalizeUnionType(schema map[string]any) map[string]any {
	types, ok := schema["type"].([]any)
	if !ok {
		return schema
	}
	hasNull := false
	nonNull := make([]string, 0, len(types))
	for _, item := range types {
		name, ok := item.(string)
		if !ok {
			return schema
		}
		if name == "null" {
			hasNull = true
			continue
		}
		nonNull = append(nonNull, name)
	}
	if !hasNull {
		return schema
	}
	if len(nonNull) == 1 {
		schema["type"] = nonNull[0]
		schema["nullable"] = true
		return schema
	}
	if len(nonNull) == 2 && nonNull[0] == "object" && nonNull[1] == "array" {
		object := map[string]any{"type": "object"}
		if value, ok := schema["additionalProperties"]; ok {
			object["additionalProperties"] = value
			delete(schema, "additionalProperties")
		}
		array := map[string]any{"type": "array"}
		if value, ok := schema["items"]; ok {
			array["items"] = value
			delete(schema, "items")
		}
		delete(schema, "type")
		schema["oneOf"] = []any{object, array}
		schema["nullable"] = true
	}
	return schema
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "openapiogen: %s: %v\n", action, err)
	os.Exit(1)
}
