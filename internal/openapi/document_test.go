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
