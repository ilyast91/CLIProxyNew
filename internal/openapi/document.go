// Package openapi предоставляет встроенный JSON-документ API, сгенерированный из openapi.yaml.
package openapi

import _ "embed"

//go:generate go run ../../cmd/openapijson -in ../../openapi.yaml -out openapi.json
//go:embed openapi.json
var document []byte

// Document возвращает копию встроенной OpenAPI JSON-спецификации.
func Document() []byte {
	return append([]byte(nil), document...)
}
