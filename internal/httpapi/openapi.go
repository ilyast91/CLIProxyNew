package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/swaggest/swgui/v5emb"
)

// OpenAPIRouterConfigurator регистрирует эндпоинты OpenAPI JSON и Swagger UI.
func OpenAPIRouterConfigurator(document []byte) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
	docsHandler := gin.WrapH(v5emb.New("CLIProxyNew API", "/openapi.json", "/docs/"))
	return func(router *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
		router.GET("/openapi.json", func(c *gin.Context) {
			c.Data(http.StatusOK, "application/json; charset=utf-8", document)
		})
		router.GET("/docs", docsHandler)
		router.GET("/docs/*path", docsHandler)
	}
}
