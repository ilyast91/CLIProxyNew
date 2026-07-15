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

type successfulPinger struct{}

func (successfulPinger) Ping(context.Context) error { return nil }

type failingPinger struct{}

func (failingPinger) Ping(context.Context) error { return errors.New("database unavailable") }
