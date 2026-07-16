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

func TestDocumentDescribesProxyEndpointsWithBearerAuthentication(t *testing.T) {
	type operation struct {
		Security   []map[string][]string `json:"security"`
		Parameters []struct {
			Name     string `json:"name"`
			In       string `json:"in"`
			Required bool   `json:"required"`
		} `json:"parameters"`
	}
	var document struct {
		Paths map[string]map[string]operation `json:"paths"`
	}
	if err := json.Unmarshal(Document(), &document); err != nil {
		t.Fatalf("unmarshal document: %v", err)
	}

	expected := []struct {
		method         string
		path           string
		pathParameters []string
	}{
		{method: "get", path: "/v1/models"},
		{method: "post", path: "/v1/chat/completions"},
		{method: "post", path: "/v1/completions"},
		{method: "post", path: "/v1/images/generations"},
		{method: "post", path: "/v1/images/edits"},
		{method: "post", path: "/v1/videos"},
		{method: "post", path: "/v1/videos/generations"},
		{method: "post", path: "/v1/videos/edits"},
		{method: "post", path: "/v1/videos/extensions"},
		{method: "get", path: "/v1/videos/{request_id}", pathParameters: []string{"request_id"}},
		{method: "post", path: "/v1/messages"},
		{method: "post", path: "/v1/messages/count_tokens"},
		{method: "get", path: "/v1/responses"},
		{method: "post", path: "/v1/responses"},
		{method: "post", path: "/v1/responses/compact"},
		{method: "post", path: "/v1/alpha/search"},
		{method: "post", path: "/openai/v1/videos"},
		{method: "get", path: "/openai/v1/videos/{video_id}/content", pathParameters: []string{"video_id"}},
		{method: "get", path: "/openai/v1/videos/{video_id}", pathParameters: []string{"video_id"}},
		{method: "get", path: "/backend-api/codex/responses"},
		{method: "post", path: "/backend-api/codex/responses"},
		{method: "post", path: "/backend-api/codex/responses/compact"},
		{method: "post", path: "/backend-api/codex/alpha/search"},
		{method: "get", path: "/v1beta/models"},
		{method: "post", path: "/v1beta/interactions"},
		{method: "get", path: "/v1beta/models/{model}:{action}", pathParameters: []string{"model", "action"}},
		{method: "post", path: "/v1beta/models/{model}:{action}", pathParameters: []string{"model", "action"}},
	}

	for _, want := range expected {
		operations, ok := document.Paths[want.path]
		if !ok {
			t.Fatalf("path %q is not documented", want.path)
		}
		operation, ok := operations[want.method]
		if !ok {
			t.Fatalf("method %s %s is not documented", want.method, want.path)
		}
		if !hasBearerAPIKeySecurity(operation.Security) {
			t.Fatalf("method %s %s does not require BearerApiKey", want.method, want.path)
		}
		for _, parameter := range want.pathParameters {
			if !hasRequiredPathParameter(operation.Parameters, parameter) {
				t.Fatalf("method %s %s does not declare required path parameter %q", want.method, want.path, parameter)
			}
		}
	}

	if _, ok := document.Paths["/v1/models/{model}:generateContent"]; ok {
		t.Fatal("legacy Gemini path /v1/models/{model}:generateContent must not be documented")
	}
}

func hasRequiredPathParameter(parameters []struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}, name string) bool {
	for _, parameter := range parameters {
		if parameter.Name == name && parameter.In == "path" && parameter.Required {
			return true
		}
	}
	return false
}

func hasBearerAPIKeySecurity(security []map[string][]string) bool {
	for _, requirement := range security {
		if _, ok := requirement["BearerApiKey"]; ok {
			return true
		}
	}
	return false
}
