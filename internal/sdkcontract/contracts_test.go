// Package sdkcontract_test проверяет публичную границу SDK, от которой зависит бизнес-слой.
package sdkcontract_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ilyast91/CLIProxyNew/internal/access"
	"github.com/ilyast91/CLIProxyNew/internal/auth/selector"
	"github.com/ilyast91/CLIProxyNew/internal/httpapi"
	"github.com/ilyast91/CLIProxyNew/internal/modelregistry"
	"github.com/ilyast91/CLIProxyNew/internal/store"
	"github.com/ilyast91/CLIProxyNew/internal/usage"
	"github.com/ilyast91/CLIProxyNew/internal/watcher"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
	sdkapi "github.com/router-for-me/CLIProxyAPI/v7/sdk/api"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	cliproxy "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

var (
	_ coreauth.Store                                              = (*store.CoreAuthStore)(nil)
	_ coreauth.Selector                                           = (*selector.Selector)(nil)
	_ coreauth.Hook                                               = (*usage.Hook)(nil)
	_ sdkusage.Plugin                                             = (*usage.BufferedPlugin)(nil)
	_ sdkaccess.Provider                                          = (*access.Provider)(nil)
	_ cliproxy.WatcherFactory                                     = watcher.NoopFactory
	_ cliproxy.ModelRegistryHook                                  = (*modelregistry.Hook)(nil)
	_ func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config) = httpapi.RouterConfigurator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
)

func TestPublicSDKContractsCompile(t *testing.T) {
	options := []sdkapi.ServerOption{
		sdkapi.WithMiddleware(httpapi.RequestIDMiddleware()),
		sdkapi.WithRouterConfigurator(httpapi.RouterConfigurator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)),
	}
	if len(options) != 2 {
		t.Fatalf("server options = %d, want 2", len(options))
	}
}
