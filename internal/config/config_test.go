package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultMatchesPRDDefaults(t *testing.T) {
	cfg := Default()

	if cfg.Mode != ModeDev {
		t.Fatalf("Mode = %q; want %q", cfg.Mode, ModeDev)
	}
	if cfg.ProxyProtocol != ProxyProtocolSocks {
		t.Fatalf("ProxyProtocol = %q; want %q", cfg.ProxyProtocol, ProxyProtocolSocks)
	}
	if cfg.SocksPort != 10808 {
		t.Fatalf("SocksPort = %d; want 10808", cfg.SocksPort)
	}
	if cfg.HTTPPort != 10809 {
		t.Fatalf("HTTPPort = %d; want 10809", cfg.HTTPPort)
	}
	if cfg.TUNInterface != "wng-tun" {
		t.Fatalf("TUNInterface = %q; want wng-tun", cfg.TUNInterface)
	}
	if cfg.TUNAddress != "172.19.0.1/30" {
		t.Fatalf("TUNAddress = %q; want 172.19.0.1/30", cfg.TUNAddress)
	}
	if cfg.DNSStrategy != "ipv4_only" {
		t.Fatalf("DNSStrategy = %q; want ipv4_only", cfg.DNSStrategy)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "config.json")

	cfg, err := Load(path)

	if err != nil {
		t.Fatalf("Load missing file returned error: %v", err)
	}
	if cfg != Default() {
		t.Fatalf("Load missing file = %+v; want defaults %+v", cfg, Default())
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "config.json")
	want := Default()
	want.Mode = ModeStrict
	want.ProxyProtocol = ProxyProtocolHTTP
	want.SocksPort = 20808
	want.HTTPPort = 20809
	want.TUNInterface = "custom-tun"

	if err := Save(path, want); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("saved config is invalid JSON:\n%s", string(data))
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("saved config should end with newline:\n%s", string(data))
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Load = %+v; want %+v", got, want)
	}
}

func TestValidateRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(Config) Config
		wantErr string
	}{
		{
			name: "mode",
			mutate: func(cfg Config) Config {
				cfg.Mode = "prod"
				return cfg
			},
			wantErr: "mode",
		},
		{
			name: "proxy protocol",
			mutate: func(cfg Config) Config {
				cfg.ProxyProtocol = "shadowsocks"
				return cfg
			},
			wantErr: "proxy_protocol",
		},
		{
			name: "socks port",
			mutate: func(cfg Config) Config {
				cfg.SocksPort = 0
				return cfg
			},
			wantErr: "socks_port",
		},
		{
			name: "http port",
			mutate: func(cfg Config) Config {
				cfg.HTTPPort = 70000
				return cfg
			},
			wantErr: "http_port",
		},
		{
			name: "tun interface",
			mutate: func(cfg Config) Config {
				cfg.TUNInterface = ""
				return cfg
			},
			wantErr: "tun_interface",
		},
		{
			name: "tun interface too long",
			mutate: func(cfg Config) Config {
				cfg.TUNInterface = "interface-name-too-long"
				return cfg
			},
			wantErr: "tun_interface",
		},
		{
			name: "tun interface unsafe character",
			mutate: func(cfg Config) Config {
				cfg.TUNInterface = "wng tun"
				return cfg
			},
			wantErr: "tun_interface",
		},
		{
			name: "tun address",
			mutate: func(cfg Config) Config {
				cfg.TUNAddress = ""
				return cfg
			},
			wantErr: "tun_address",
		},
		{
			name: "tun address without prefix",
			mutate: func(cfg Config) Config {
				cfg.TUNAddress = "172.19.0.1"
				return cfg
			},
			wantErr: "tun_address",
		},
		{
			name: "tun address ipv6",
			mutate: func(cfg Config) Config {
				cfg.TUNAddress = "fd00::1/64"
				return cfg
			},
			wantErr: "tun_address",
		},
		{
			name: "dns strategy",
			mutate: func(cfg Config) Config {
				cfg.DNSStrategy = ""
				return cfg
			},
			wantErr: "dns_strategy",
		},
		{
			name: "unsupported dns strategy",
			mutate: func(cfg Config) Config {
				cfg.DNSStrategy = "auto"
				return cfg
			},
			wantErr: "dns_strategy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(Default()).Validate()
			if err == nil {
				t.Fatal("Validate returned nil; want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate error = %q; want mention %q", err.Error(), tt.wantErr)
			}
		})
	}
}
