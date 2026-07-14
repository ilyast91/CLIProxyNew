package config

import (
	"fmt"
	"net"
	"strconv"

	sdkconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

// SDKConfig строит минимальную конфигурацию публичного SDK из business-config.
func (c *Config) SDKConfig() (*sdkconfig.Config, error) {
	if c == nil {
		return nil, fmt.Errorf("config is nil")
	}
	host, portText, err := net.SplitHostPort(c.Server.Addr)
	if err != nil {
		return nil, fmt.Errorf("parse server.addr: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("parse server.addr port %q", portText)
	}
	return &sdkconfig.Config{Host: host, Port: port, UsageStatisticsEnabled: true}, nil
}
