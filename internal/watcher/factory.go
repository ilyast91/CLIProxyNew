package watcher

import (
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// NoopFactory отключает file-backed watcher SDK: auth records приходят из Postgres.
func NoopFactory(string, string, func(*config.Config)) (*cliproxy.WatcherWrapper, error) {
	return &cliproxy.WatcherWrapper{}, nil
}
