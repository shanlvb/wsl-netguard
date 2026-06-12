package render

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

func SingBoxJSON(cfg config.Config, windowsHost netip.Addr) ([]byte, error) {
	if err := validateRuntimeInput(cfg, windowsHost); err != nil {
		return nil, err
	}

	return marshalSingBoxDocument(singBoxDocumentFor(cfg, windowsHost.String()))
}

func SingBoxTemplateJSON(cfg config.Config) ([]byte, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return marshalSingBoxDocument(singBoxDocumentFor(cfg, "__WIN_HOST__"))
}

func singBoxDocumentFor(cfg config.Config, server string) singBoxDocument {
	proxyType, proxyPort := proxyOutbound(cfg)
	return singBoxDocument{
		Log: singBoxLog{
			Level:     "info",
			Timestamp: true,
		},
		DNS: singBoxDNS{
			Servers: []singBoxDNSServer{
				{
					Type:       "https",
					Tag:        "remote-doh",
					Server:     "1.1.1.1",
					ServerPort: 443,
					Path:       "/dns-query",
					Detour:     "proxy",
				},
			},
			Final:    "remote-doh",
			Strategy: cfg.DNSStrategy,
		},
		Inbounds: []singBoxInbound{
			{
				Type:          "tun",
				Tag:           "tun-in",
				InterfaceName: cfg.TUNInterface,
				Address:       []string{cfg.TUNAddress},
				MTU:           9000,
				AutoRoute:     true,
				StrictRoute:   true,
				Stack:         "system",
			},
		},
		Outbounds: []singBoxOutbound{
			{
				Type:       proxyType,
				Tag:        "proxy",
				Server:     server,
				ServerPort: proxyPort,
				Version:    socksVersion(proxyType),
			},
		},
		Route: singBoxRoute{
			Rules: []singBoxRouteRule{
				{
					Action: "sniff",
				},
				{
					Protocol: "dns",
					Action:   "hijack-dns",
				},
			},
			AutoDetectInterface: true,
			Final:               "proxy",
		},
	}
}

func marshalSingBoxDocument(doc singBoxDocument) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal sing-box config: %w", err)
	}
	return append(data, '\n'), nil
}

func NFTablesRules(cfg config.Config, windowsHost netip.Addr) (string, error) {
	return nftablesRules(cfg, windowsHost, "wsl_netguard")
}

func NFTablesCheckRules(cfg config.Config, windowsHost netip.Addr) (string, error) {
	return nftablesRules(cfg, windowsHost, "wsl_netguard_check")
}

func nftablesRules(cfg config.Config, windowsHost netip.Addr, tableName string) (string, error) {
	if err := validateRuntimeInput(cfg, windowsHost); err != nil {
		return "", err
	}
	if tableName == "" {
		return "", fmt.Errorf("nftables table name must not be empty")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "table inet %s {\n", tableName)
	fmt.Fprintln(&b, "  chain output {")
	fmt.Fprintln(&b, "    type filter hook output priority 0; policy drop;")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "    ct state established,related accept")
	fmt.Fprintln(&b, `    oifname "lo" accept`)
	fmt.Fprintf(&b, "    oifname %q accept\n", cfg.TUNInterface)

	switch cfg.Mode {
	case config.ModeDev:
		fmt.Fprintf(&b, "    ip daddr %s accept\n", windowsHost)
	case config.ModeStrict:
		fmt.Fprintf(&b, "    ip daddr %s tcp dport %s accept\n", windowsHost, strictTCPPortSet(cfg))
		fmt.Fprintf(&b, "    ip daddr %s udp dport %d accept\n", windowsHost, cfg.SocksPort)
	default:
		return "", fmt.Errorf("unsupported mode %q", cfg.Mode)
	}

	fmt.Fprintln(&b, "  }")
	fmt.Fprintln(&b, "}")
	return b.String(), nil
}

func validateRuntimeInput(cfg config.Config, windowsHost netip.Addr) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	if !windowsHost.IsValid() {
		return fmt.Errorf("windows host IP must be set")
	}
	if !windowsHost.Is4() {
		return fmt.Errorf("windows host IP %q must be IPv4", windowsHost)
	}
	return nil
}

func proxyOutbound(cfg config.Config) (string, int) {
	if cfg.ProxyProtocol == config.ProxyProtocolHTTP {
		return "http", cfg.HTTPPort
	}
	return "socks", cfg.SocksPort
}

func socksVersion(proxyType string) string {
	if proxyType == "socks" {
		return "5"
	}
	return ""
}

func strictTCPPortSet(cfg config.Config) string {
	ports := []int{cfg.SocksPort, cfg.HTTPPort}
	sort.Ints(ports)
	if ports[0] == ports[1] {
		return fmt.Sprintf("%d", ports[0])
	}
	return fmt.Sprintf("{ %d, %d }", ports[0], ports[1])
}

type singBoxDocument struct {
	Log       singBoxLog        `json:"log"`
	DNS       singBoxDNS        `json:"dns"`
	Inbounds  []singBoxInbound  `json:"inbounds"`
	Outbounds []singBoxOutbound `json:"outbounds"`
	Route     singBoxRoute      `json:"route"`
}

type singBoxLog struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type singBoxDNS struct {
	Servers  []singBoxDNSServer `json:"servers"`
	Final    string             `json:"final"`
	Strategy string             `json:"strategy"`
}

type singBoxDNSServer struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Path       string `json:"path"`
	Detour     string `json:"detour"`
}

type singBoxInbound struct {
	Type          string   `json:"type"`
	Tag           string   `json:"tag"`
	InterfaceName string   `json:"interface_name"`
	Address       []string `json:"address"`
	MTU           int      `json:"mtu"`
	AutoRoute     bool     `json:"auto_route"`
	StrictRoute   bool     `json:"strict_route"`
	Stack         string   `json:"stack"`
}

type singBoxOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	Version    string `json:"version,omitempty"`
}

type singBoxRoute struct {
	Rules               []singBoxRouteRule `json:"rules"`
	AutoDetectInterface bool               `json:"auto_detect_interface"`
	Final               string             `json:"final"`
}

type singBoxRouteRule struct {
	Protocol string `json:"protocol,omitempty"`
	Action   string `json:"action,omitempty"`
	Outbound string `json:"outbound,omitempty"`
}
