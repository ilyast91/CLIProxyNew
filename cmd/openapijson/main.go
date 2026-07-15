// Команда openapijson преобразует source OpenAPI YAML в JSON для embed в бинарник.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func main() {
	input := flag.String("in", "openapi.yaml", "путь к исходной OpenAPI YAML-спецификации")
	output := flag.String("out", "internal/openapi/openapi.json", "путь к сгенерированной JSON-спецификации")
	flag.Parse()

	raw, err := os.ReadFile(*input)
	if err != nil {
		fail("read input", err)
	}
	var document any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		fail("parse YAML", err)
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		fail("encode JSON", err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fail("create output directory", err)
	}
	if err := os.WriteFile(*output, append(encoded, '\n'), 0o644); err != nil {
		fail("write output", err)
	}
}

func fail(action string, err error) {
	fmt.Fprintf(os.Stderr, "openapijson: %s: %v\n", action, err)
	os.Exit(1)
}
