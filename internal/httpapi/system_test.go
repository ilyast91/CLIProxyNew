package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSystemRouterConfiguratorServesLivenessWithoutDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	SystemRouterConfigurator(failingPinger{})(router, nil, nil)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("liveness status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestSystemRouterConfiguratorReportsReadinessFromDatabasePing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, pinger := range map[string]databasePinger{
		"ready":       successfulPinger{},
		"unavailable": failingPinger{},
	} {
		t.Run(name, func(t *testing.T) {
			router := gin.New()
			SystemRouterConfigurator(pinger)(router, nil, nil)

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			want := http.StatusOK
			if name == "unavailable" {
				want = http.StatusServiceUnavailable
			}
			if response.Code != want {
				t.Fatalf("readiness status=%d body=%q, want %d", response.Code, response.Body.String(), want)
			}
		})
	}
}

func TestOpenAPIRouterConfiguratorServesEmbeddedDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	document := []byte(`{"openapi":"3.1.0"}`)
	OpenAPIRouterConfigurator(document)(router, nil, nil)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("openapi status=%d body=%q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("content type=%q", contentType)
	}
	if got := response.Body.String(); got != string(document) {
		t.Fatalf("document=%q, want %q", got, document)
	}
}

func TestOpenAPIRouterConfiguratorServesDocumentationRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	OpenAPIRouterConfigurator([]byte(`{"openapi":"3.1.0"}`))(router, nil, nil)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("docs status=%d body=%q", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `/docs/swagger-ui-bundle.js`) {
		t.Fatalf("docs body does not contain local Swagger UI asset: %s", response.Body.String())
	}
}

func TestOpenAPIRouterConfiguratorServesEmbeddedDocumentationUI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	OpenAPIRouterConfigurator([]byte(`{"openapi":"3.1.0"}`))(router, nil, nil)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("docs status=%d body=%q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
		t.Fatalf("content type=%q", contentType)
	}
	for _, fragment := range []string{
		`/docs/swagger-ui.css`,
		`/docs/swagger-ui-bundle.js`,
		`/openapi.json`,
		`validatorUrl: null`,
	} {
		if !strings.Contains(response.Body.String(), fragment) {
			t.Fatalf("docs body does not contain %q: %s", fragment, response.Body.String())
		}
	}
	for _, fragment := range []string{`src="http://`, `src="https://`, `href="http://`, `href="https://`} {
		if strings.Contains(response.Body.String(), fragment) {
			t.Fatalf("docs body contains remote asset %q: %s", fragment, response.Body.String())
		}
	}
}

func TestOpenAPIRouterConfiguratorServesEmbeddedDocumentationAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	OpenAPIRouterConfigurator([]byte(`{"openapi":"3.1.0"}`))(router, nil, nil)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/swagger-ui-bundle.js", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("asset status=%d body=%q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("asset content type=%q", contentType)
	}
}

func TestMetricsRouterConfiguratorServesPrometheusHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	MetricsRouterConfigurator(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = response.Write([]byte("metric 1\n"))
	}))(router, nil, nil)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK || response.Body.String() != "metric 1\n" {
		t.Fatalf("metrics status=%d body=%q", response.Code, response.Body.String())
	}
}

type successfulPinger struct{}

func (successfulPinger) Ping(context.Context) error { return nil }

type failingPinger struct{}

func (failingPinger) Ping(context.Context) error { return errors.New("database unavailable") }
