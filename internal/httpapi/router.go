package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// RouterConfigurator возвращает конфигуратор management-маршрутов для SDK ядра.
func RouterConfigurator(login *LoginHandler, sessions *identity.SessionAuthenticator, logout gin.HandlerFunc) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
	return func(router *gin.Engine, _ *handlers.BaseAPIHandler, _ *config.Config) {
		if login != nil {
			router.POST("/api/v1/login", login.Handle)
		}
		if logout != nil {
			router.POST("/api/v1/logout", logout)
		}
		if sessions != nil {
			management := router.Group("/api/v1", SessionMiddleware(sessions))
			management.GET("/me", CurrentUserHandler)
		}
	}
}
