package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
)

const (
	ModeDev    = "dev"
	ModeStrict = "strict"

	ProxyProtocolSocks = "socks"
	ProxyProtocolHTTP  = "http"

	DNSStrategyPreferIPv4 = "prefer_ipv4"
	DNSStrategyPreferIPv6 = "prefer_ipv6"
	DNSStrategyIPv4Only   = "ipv4_only"
	DNSStrategyIPv6Only   = "ipv6_only"

	DefaultPath = "/etc/wsl-netguard/config.json"
)

type Config struct {
	Mode          string `json:"mode"`
	ProxyProtocol string `json:"proxy_protocol"`
	SocksPort     int    `json:"socks_port"`
	HTTPPort      int    `json:"http_port"`
	TUNInterface  string `json:"tun_interface"`
	TUNAddress    string `json:"tun_address"`
	DNSStrategy   string `json:"dns_strategy"`
}

func Default() Config {
	return Config{
		Mode:          ModeDev,
		ProxyProtocol: ProxyProtocolSocks,
		SocksPort:     10808,
		HTTPPort:      10809,
		TUNInterface:  "wng-tun",
		TUNAddress:    "172.19.0.1/30",
		DNSStrategy:   DNSStrategyIPv4Only,
	}
}

func Load(path string) (Config, error) {
	// #nosec G304 -- production callers use the managed config path; tests inject temp paths.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %s: %w", path, err)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config before save: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	// #nosec G301 -- managed config directories are intentionally world-readable system config paths.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}
	// #nosec G306 -- config contains no secrets and is installed as readable system configuration.
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

func (cfg Config) Validate() error {
	if cfg.Mode != ModeDev && cfg.Mode != ModeStrict {
		return fmt.Errorf("mode must be %q or %q", ModeDev, ModeStrict)
	}
	if cfg.ProxyProtocol != ProxyProtocolSocks && cfg.ProxyProtocol != ProxyProtocolHTTP {
		return fmt.Errorf("proxy_protocol must be %q or %q", ProxyProtocolSocks, ProxyProtocolHTTP)
	}
	if !validPort(cfg.SocksPort) {
		return fmt.Errorf("socks_port must be between 1 and 65535")
	}
	if !validPort(cfg.HTTPPort) {
		return fmt.Errorf("http_port must be between 1 and 65535")
	}
	if !validInterfaceName(cfg.TUNInterface) {
		return fmt.Errorf("tun_interface must be 1-15 characters using letters, numbers, '.', '_', ':', or '-'")
	}
	if !validTUNAddress(cfg.TUNAddress) {
		return fmt.Errorf("tun_address must be an IPv4 CIDR address")
	}
	if !validDNSStrategy(cfg.DNSStrategy) {
		return fmt.Errorf("dns_strategy must be %q, %q, %q, or %q", DNSStrategyPreferIPv4, DNSStrategyPreferIPv6, DNSStrategyIPv4Only, DNSStrategyIPv6Only)
	}
	return nil
}

func validPort(port int) bool {
	return port > 0 && port <= 65535
}

func validInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '.', '_', ':', '-':
			continue
		default:
			return false
		}
	}
	return true
}

func validTUNAddress(address string) bool {
	prefix, err := netip.ParsePrefix(address)
	if err != nil {
		return false
	}
	return prefix.Addr().Is4() && prefix.Bits() > 0
}

func validDNSStrategy(strategy string) bool {
	switch strategy {
	case DNSStrategyPreferIPv4, DNSStrategyPreferIPv6, DNSStrategyIPv4Only, DNSStrategyIPv6Only:
		return true
	default:
		return false
	}
}
