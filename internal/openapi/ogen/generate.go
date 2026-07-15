// Package ogen содержит сгенерированные typed OpenAPI bindings для будущих HTTP adapters.
package ogen

//go:generate go run ../../../cmd/openapiogen -in ../../../openapi.yaml -out openapi.compat.yaml
//go:generate go run github.com/ogen-go/ogen/cmd/ogen@v1.20.3 --target . --package ogen openapi.compat.yaml
