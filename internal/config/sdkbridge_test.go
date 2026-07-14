package config

import "testing"

func TestSDKConfigMapsServerAddress(t *testing.T) {
	cfg := Default()
	cfg.Server.Addr = "127.0.0.1:8088"
	sdkCfg, err := cfg.SDKConfig()
	if err != nil || sdkCfg.Host != "127.0.0.1" || sdkCfg.Port != 8088 {
		t.Fatalf("SDKConfig() = %+v, %v", sdkCfg, err)
	}
}
