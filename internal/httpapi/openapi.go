package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// OpenAPIRouterConfigurator регистрирует endpoint со встроенной OpenAPI JSON-спецификацией.
func OpenAPIRouterConfigurator(document []byte) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
	return func(router *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
		router.GET("/openapi.json", func(c *gin.Context) {
			c.Data(http.StatusOK, "application/json; charset=utf-8", document)
		})
	}
}
