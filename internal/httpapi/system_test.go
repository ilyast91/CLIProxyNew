package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

type successfulPinger struct{}

func (successfulPinger) Ping(context.Context) error { return nil }

type failingPinger struct{}

func (failingPinger) Ping(context.Context) error { return errors.New("database unavailable") }
