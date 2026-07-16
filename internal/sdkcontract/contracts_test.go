// Package sdkcontract_test проверяет публичную границу SDK, от которой зависит бизнес-слой.
package sdkcontract_test

import (
	"context"
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
	coreexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdkusage "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/usage"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var (
	_ coreauth.Store                                                                                                 = (*store.CoreAuthStore)(nil)
	_ coreauth.Selector                                                                                              = (*selector.Selector)(nil)
	_ coreauth.Hook                                                                                                  = (*usage.Hook)(nil)
	_ sdkusage.Plugin                                                                                                = (*usage.BufferedPlugin)(nil)
	_ sdkaccess.Provider                                                                                             = (*access.Provider)(nil)
	_ cliproxy.WatcherFactory                                                                                        = watcher.NoopFactory
	_ cliproxy.ModelRegistryHook                                                                                     = (*modelregistry.Hook)(nil)
	_ coreexecutor.RequestScopedError                                                                                = (*coreauth.Error)(nil)
	_ func(*gin.Engine, *handlers.BaseAPIHandler, *config.Config)                                                    = httpapi.RouterConfigurator(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_ func(*coreauth.Manager, context.Context, string, string, string, coreexecutor.Options) (*coreauth.Auth, error) = (*coreauth.Manager).SelectAuthByKind
	_ func(context.Context, bool) context.Context                                                                    = sdkusage.WithGenerate
	_ func(context.Context) bool                                                                                     = sdkusage.GenerateFromContext
	_ func(bool) *bool                                                                                               = sdkusage.GenerateFlag
	_ func(*bool) bool                                                                                               = sdkusage.GenerateEnabled
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

func TestSDKV7280PublicAdditionsCompile(t *testing.T) {
	result := cliproxy.APIKeyClientResult{XAIKeyCount: 1}
	if result.XAIKeyCount != 1 {
		t.Fatalf("xAI key count = %d", result.XAIKeyCount)
	}

	if sdkusage.GenerateEnabled(nil) != true {
		t.Fatal("nil usage generate flag must preserve enabled default")
	}
	generate := sdkusage.GenerateFlag(false)
	if sdkusage.GenerateEnabled(generate) {
		t.Fatal("explicit false usage generate flag must disable generation")
	}
	ctx := sdkusage.WithGenerate(context.Background(), false)
	if sdkusage.GenerateFromContext(ctx) || !sdkusage.GenerateFromContext(nil) {
		t.Fatal("usage generate context round-trip changed semantics")
	}

	if coreexecutor.GenerateMetadataKey != "generate" {
		t.Fatalf("generate metadata key = %q", coreexecutor.GenerateMetadataKey)
	}
	if sdkusage.AutoServiceTier != "auto" {
		t.Fatalf("auto service tier = %q", sdkusage.AutoServiceTier)
	}

	var xaiConfig config.XAIKey
	var xaiModel config.XAIModel
	pluginRecord := pluginapi.UsageRecord{Generate: true}
	if !pluginRecord.Generate {
		t.Fatal("plugin usage generate field changed semantics")
	}
	_ = xaiConfig
	_ = xaiModel
}
