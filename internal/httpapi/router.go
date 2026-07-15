package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/auth/identity"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// RouterConfigurator возвращает конфигуратор management-маршрутов для SDK ядра.
func RouterConfigurator(login *LoginHandler, sessions *identity.SessionAuthenticator, logout gin.HandlerFunc, keys *APIKeyHandler, usage *UsageHandler, adminUsers *AdminUserHandler, adminKeys *AdminAPIKeyHandler, oauthSessions *AdminOAuthSessionHandler, providerKeys *AdminProviderKeyHandler, accountTest *AdminAccountTestHandler, models *AdminModelHandler) func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) {
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
			if keys != nil {
				management.GET("/me/keys", keys.List)
				management.POST("/me/keys", keys.Create)
				management.DELETE("/me/keys/:keyID", keys.Revoke)
			}
			if usage != nil {
				management.GET("/me/usage", usage.Get)
			}
			if adminUsers != nil || adminKeys != nil || oauthSessions != nil || providerKeys != nil || accountTest != nil || models != nil {
				admin := management.Group("/admin", RequireRole(identity.RoleAdmin))
				if adminUsers != nil {
					admin.GET("/users", adminUsers.List)
					admin.PATCH("/users/:userID", adminUsers.SetStatus)
				}
				if adminKeys != nil {
					admin.GET("/keys", adminKeys.List)
				}
				if oauthSessions != nil {
					admin.GET("/oauth/sessions", oauthSessions.ListPending)
					admin.GET("/oauth/sessions/:state", oauthSessions.Get)
					admin.DELETE("/oauth/sessions/:state", oauthSessions.Cancel)
				}
				if providerKeys != nil {
					admin.POST("/providers/keys", providerKeys.Create)
				}
				if accountTest != nil {
					admin.POST("/accounts/:accountID/test", accountTest.Test)
				}
				if models != nil {
					admin.PUT("/models/:modelAlias", models.Upsert)
					admin.DELETE("/models/:modelAlias", models.Delete)
					admin.GET("/models", models.List)
				}
			}
		}
	}
}
