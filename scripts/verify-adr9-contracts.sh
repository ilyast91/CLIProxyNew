#!/bin/sh
set -eu

# Compile-time compatibility публичных extension points SDK.
go test -race ./internal/sdkcontract

# Поведенческая матрица семи контрактов ADR-9.
go test -race -run 'TestProviderAuthenticatesBearerTokenForActiveSource|TestProviderRejectsMissingAndInvalidCredentials' ./internal/access
go test -race -run 'TestSelectorPickUsesEnabledOverrideProvider|TestSelectorPickRejectsDisabledOrUnknownAlias|TestSelectorFailsClosedWhenExpiredCacheCannotBeReloaded' ./internal/auth/selector
go test -race -run 'TestHookCountsAuthLifecycleAndResults|TestPluginWritesUsageEventFromVersionedPrincipal' ./internal/usage
go test -race -run 'TestNoopFactoryReturnsUsableWatcherWrapper|TestRevisionPollerCallsShutdownAfterRevisionChange' ./internal/watcher
go test -race -run 'TestHookStoresCompleteModelSnapshot|TestHookDeletesSnapshotWhenModelsAreUnregistered' ./internal/modelregistry
go test -race -run 'TestRouterConfiguratorEnforcesManagementSessionAndRole|TestSystemRouterConfiguratorServesLivenessWithoutDatabase' ./internal/httpapi
go test -race -run '^TestIntegrationCoreAuthStoreContract$' ./internal/store
