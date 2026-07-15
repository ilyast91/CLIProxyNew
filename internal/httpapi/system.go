package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

const readinessTimeout = 3 * time.Second

type databasePinger interface {
	Ping(context.Context) error
}

// SystemRouterConfigurator регистрирует независимые liveness и readiness probes.
func SystemRouterConfigurator(pinger databasePinger) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
	return func(router *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
		router.GET("/healthz", func(c *gin.Context) {
			c.String(http.StatusOK, "ok")
		})
		router.GET("/readyz", func(c *gin.Context) {
			if pinger == nil {
				c.String(http.StatusServiceUnavailable, "not ready")
				return
			}
			ctx, cancel := context.WithTimeout(c.Request.Context(), readinessTimeout)
			defer cancel()
			if err := pinger.Ping(ctx); err != nil {
				c.String(http.StatusServiceUnavailable, "not ready")
				return
			}
			c.String(http.StatusOK, "ok")
		})
	}
}
