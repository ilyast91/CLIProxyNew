package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// RouterConfigurator возвращает конфигуратор management-маршрутов для SDK ядра.
func RouterConfigurator(login *LoginHandler) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
	return func(router *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
		if login != nil {
			router.POST("/api/v1/login", login.Handle)
		}
	}
}
