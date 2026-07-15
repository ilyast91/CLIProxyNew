package openapi

import (
	"encoding/json"
	"testing"
)

func TestDocumentContainsOpenAPIVersion(t *testing.T) {
	var document struct {
		OpenAPI string `json:"openapi"`
	}
	if err := json.Unmarshal(Document(), &document); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	if document.OpenAPI == "" {
		t.Fatal("OpenAPI version is empty")
	}
}

func TestDocumentDescribesSessionLifecycleEndpoints(t *testing.T) {
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(Document(), &document); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}
	for path, method := range map[string]string{
		"/api/v1/me":     "get",
		"/api/v1/logout": "post",
	} {
		operations, ok := document.Paths[path]
		if !ok {
			t.Fatalf("path %q is not documented", path)
		}
		if _, ok := operations[method]; !ok {
			t.Fatalf("method %s %s is not documented", method, path)
		}
	}
}
