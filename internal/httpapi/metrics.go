package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// MetricsRouterConfigurator регистрирует независимый Prometheus endpoint.
func MetricsRouterConfigurator(handler http.Handler) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
	return func(router *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
		if handler != nil {
			router.GET("/metrics", gin.WrapH(handler))
		}
	}
}
