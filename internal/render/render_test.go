package render

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

func TestSingBoxJSONRendersSOCKSProxy(t *testing.T) {
	cfg := config.Default()
	host := netip.MustParseAddr("172.25.80.1")

	data, err := SingBoxJSON(cfg, host)
	if err != nil {
		t.Fatalf("SingBoxJSON returned error: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("SingBoxJSON returned invalid JSON:\n%s", string(data))
	}
	mustNotContain(t, string(data), `"sniff":`)
	mustNotContain(t, string(data), `"protocol": ""`)

	got := decodeSingBoxConfig(t, data)
	if got.DNS.Strategy != cfg.DNSStrategy {
		t.Fatalf("DNS strategy = %q; want %q", got.DNS.Strategy, cfg.DNSStrategy)
	}
	if len(got.DNS.Servers) != 1 {
		t.Fatalf("DNS servers length = %d; want 1", len(got.DNS.Servers))
	}
	dnsServer := got.DNS.Servers[0]
	if dnsServer.Type != "https" {
		t.Fatalf("DNS server type = %q; want https", dnsServer.Type)
	}
	if dnsServer.Tag != "remote-doh" {
		t.Fatalf("DNS server tag = %q; want remote-doh", dnsServer.Tag)
	}
	if dnsServer.Server != "1.1.1.1" {
		t.Fatalf("DNS server = %q; want 1.1.1.1", dnsServer.Server)
	}
	if dnsServer.ServerPort != 443 {
		t.Fatalf("DNS server_port = %d; want 443", dnsServer.ServerPort)
	}
	if dnsServer.Path != "/dns-query" {
		t.Fatalf("DNS server path = %q; want /dns-query", dnsServer.Path)
	}
	if dnsServer.Detour != "proxy" {
		t.Fatalf("DNS server detour = %q; want proxy", dnsServer.Detour)
	}
	if dnsServer.Address != "" {
		t.Fatalf("DNS server uses legacy address field %q; want empty", dnsServer.Address)
	}
	if len(got.Inbounds) != 1 {
		t.Fatalf("inbounds length = %d; want 1", len(got.Inbounds))
	}
	inbound := got.Inbounds[0]
	if inbound.Type != "tun" {
		t.Fatalf("inbound type = %q; want tun", inbound.Type)
	}
	if inbound.InterfaceName != cfg.TUNInterface {
		t.Fatalf("interface_name = %q; want %q", inbound.InterfaceName, cfg.TUNInterface)
	}
	if len(inbound.Address) != 1 || inbound.Address[0] != cfg.TUNAddress {
		t.Fatalf("inbound address = %v; want [%q]", inbound.Address, cfg.TUNAddress)
	}
	if !inbound.AutoRoute || !inbound.StrictRoute {
		t.Fatalf("auto_route/strict_route = %v/%v; want true/true", inbound.AutoRoute, inbound.StrictRoute)
	}

	if len(got.Outbounds) != 1 {
		t.Fatalf("outbounds length = %d; want 1", len(got.Outbounds))
	}
	proxy := findOutbound(t, got.Outbounds, "proxy")
	if proxy.Type != "socks" {
		t.Fatalf("proxy type = %q; want socks", proxy.Type)
	}
	if proxy.Server != host.String() {
		t.Fatalf("proxy server = %q; want %q", proxy.Server, host.String())
	}
	if proxy.ServerPort != cfg.SocksPort {
		t.Fatalf("proxy server_port = %d; want %d", proxy.ServerPort, cfg.SocksPort)
	}
	if proxy.Version != "5" {
		t.Fatalf("proxy version = %q; want 5", proxy.Version)
	}
	if got.Route.Final != "proxy" {
		t.Fatalf("route final = %q; want proxy", got.Route.Final)
	}
	if len(got.Route.Rules) != 2 {
		t.Fatalf("route rules length = %d; want 2", len(got.Route.Rules))
	}
	if got.Route.Rules[0].Action != "sniff" {
		t.Fatalf("first route rule action = %q; want sniff", got.Route.Rules[0].Action)
	}
	dnsRule := got.Route.Rules[1]
	if dnsRule.Protocol != "dns" {
		t.Fatalf("route rule protocol = %q; want dns", dnsRule.Protocol)
	}
	if dnsRule.Action != "hijack-dns" {
		t.Fatalf("route rule action = %q; want hijack-dns", dnsRule.Action)
	}
	if dnsRule.Outbound != "" {
		t.Fatalf("route rule uses legacy outbound = %q; want empty", dnsRule.Outbound)
	}
	if !got.Route.AutoDetectInterface {
		t.Fatal("route auto_detect_interface = false; want true")
	}
}

func TestSingBoxJSONRendersHTTPProxy(t *testing.T) {
	cfg := config.Default()
	cfg.ProxyProtocol = config.ProxyProtocolHTTP
	cfg.HTTPPort = 20809
	host := netip.MustParseAddr("172.25.80.1")

	data, err := SingBoxJSON(cfg, host)
	if err != nil {
		t.Fatalf("SingBoxJSON returned error: %v", err)
	}

	got := decodeSingBoxConfig(t, data)
	proxy := findOutbound(t, got.Outbounds, "proxy")
	if proxy.Type != "http" {
		t.Fatalf("proxy type = %q; want http", proxy.Type)
	}
	if proxy.ServerPort != cfg.HTTPPort {
		t.Fatalf("proxy server_port = %d; want %d", proxy.ServerPort, cfg.HTTPPort)
	}
	if proxy.Version != "" {
		t.Fatalf("proxy version = %q; want empty", proxy.Version)
	}
}

func TestSingBoxTemplateJSONUsesHostPlaceholder(t *testing.T) {
	cfg := config.Default()
	cfg.SocksPort = 20808
	cfg.TUNInterface = "custom-tun"

	data, err := SingBoxTemplateJSON(cfg)
	if err != nil {
		t.Fatalf("SingBoxTemplateJSON returned error: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("SingBoxTemplateJSON returned invalid JSON:\n%s", string(data))
	}

	got := decodeSingBoxConfig(t, data)
	proxy := findOutbound(t, got.Outbounds, "proxy")
	if proxy.Server != "__WIN_HOST__" {
		t.Fatalf("template proxy server = %q; want __WIN_HOST__", proxy.Server)
	}
	if proxy.ServerPort != 20808 {
		t.Fatalf("template proxy server_port = %d; want 20808", proxy.ServerPort)
	}
	if got.Inbounds[0].InterfaceName != "custom-tun" {
		t.Fatalf("template interface = %q; want custom-tun", got.Inbounds[0].InterfaceName)
	}
}

func TestNFTablesDevRulesAllowWindowsHost(t *testing.T) {
	cfg := config.Default()
	cfg.TUNInterface = "custom-tun"
	host := netip.MustParseAddr("172.25.80.1")

	rules, err := NFTablesRules(cfg, host)
	if err != nil {
		t.Fatalf("NFTablesRules returned error: %v", err)
	}

	mustContain(t, rules, "table inet wsl_netguard")
	mustContain(t, rules, "type filter hook output priority 0; policy drop;")
	mustContain(t, rules, `oifname "lo" accept`)
	mustContain(t, rules, `oifname "custom-tun" accept`)
	mustContain(t, rules, "ct state established,related accept")
	mustContain(t, rules, "ip daddr 172.25.80.1 accept")
	mustNotContain(t, rules, "flush ruleset")
	mustNotContain(t, rules, "tcp dport")
}

func TestNFTablesDevRulesAllowDockerBridgeInterfaces(t *testing.T) {
	cfg := config.Default()
	host := netip.MustParseAddr("172.25.80.1")

	rules, err := NFTablesRules(cfg, host)
	if err != nil {
		t.Fatalf("NFTablesRules returned error: %v", err)
	}

	mustContain(t, rules, `oifname "docker0" accept`)
	mustContain(t, rules, `oifname "br-*" accept`)
}

func TestNFTablesStrictRulesWhitelistProxyPorts(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeStrict
	host := netip.MustParseAddr("172.25.80.1")

	rules, err := NFTablesRules(cfg, host)
	if err != nil {
		t.Fatalf("NFTablesRules returned error: %v", err)
	}

	mustContain(t, rules, "table inet wsl_netguard")
	mustContain(t, rules, `oifname "wng-tun" accept`)
	mustContain(t, rules, "ip daddr 172.25.80.1 tcp dport { 10808, 10809 } accept")
	mustContain(t, rules, "ip daddr 172.25.80.1 udp dport 10808 accept")
	mustNotContain(t, rules, "flush ruleset")
	mustNotContain(t, rules, `oifname "docker0" accept`)
	mustNotContain(t, rules, `oifname "br-*" accept`)
	mustNotContain(t, rules, "ip daddr 172.25.80.1 accept")
}

func TestNFTablesCheckRulesUseValidationTable(t *testing.T) {
	cfg := config.Default()
	host := netip.MustParseAddr("172.25.80.1")

	rules, err := NFTablesCheckRules(cfg, host)
	if err != nil {
		t.Fatalf("NFTablesCheckRules returned error: %v", err)
	}

	mustContain(t, rules, "table inet wsl_netguard_check")
	mustContain(t, rules, `oifname "wng-tun" accept`)
	mustContain(t, rules, "ip daddr 172.25.80.1 accept")
	mustNotContain(t, rules, "table inet wsl_netguard {")
	mustNotContain(t, rules, "flush ruleset")
}

func TestRenderRejectsInvalidInput(t *testing.T) {
	cfg := config.Default()
	host := netip.MustParseAddr("fe80::1")

	if _, err := SingBoxJSON(cfg, host); err == nil {
		t.Fatal("SingBoxJSON returned nil error for IPv6 host; want error")
	}
	if _, err := NFTablesRules(cfg, host); err == nil {
		t.Fatal("NFTablesRules returned nil error for IPv6 host; want error")
	}

	cfg.TUNInterface = "bad interface"
	ipv4Host := netip.MustParseAddr("172.25.80.1")
	if _, err := SingBoxJSON(cfg, ipv4Host); err == nil {
		t.Fatal("SingBoxJSON returned nil error for invalid config; want error")
	}
	if _, err := NFTablesRules(cfg, ipv4Host); err == nil {
		t.Fatal("NFTablesRules returned nil error for invalid config; want error")
	}
	if _, err := SingBoxTemplateJSON(cfg); err == nil {
		t.Fatal("SingBoxTemplateJSON returned nil error for invalid config; want error")
	}
}

type singBoxConfig struct {
	DNS struct {
		Servers  []testDNSServer `json:"servers"`
		Strategy string          `json:"strategy"`
	} `json:"dns"`
	Inbounds []struct {
		Type          string   `json:"type"`
		InterfaceName string   `json:"interface_name"`
		Address       []string `json:"address"`
		AutoRoute     bool     `json:"auto_route"`
		StrictRoute   bool     `json:"strict_route"`
	} `json:"inbounds"`
	Outbounds []testOutbound `json:"outbounds"`
	Route     struct {
		Rules               []testRouteRule `json:"rules"`
		AutoDetectInterface bool            `json:"auto_detect_interface"`
		Final               string          `json:"final"`
	} `json:"route"`
}

type testDNSServer struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Path       string `json:"path"`
	Detour     string `json:"detour"`
	Address    string `json:"address"`
}

type testOutbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Version    string `json:"version"`
}

type testRouteRule struct {
	Protocol string `json:"protocol"`
	Action   string `json:"action"`
	Outbound string `json:"outbound"`
}

func decodeSingBoxConfig(t *testing.T, data []byte) singBoxConfig {
	t.Helper()

	var got singBoxConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal sing-box config: %v", err)
	}
	return got
}

func findOutbound(t *testing.T, outbounds []testOutbound, tag string) testOutbound {
	t.Helper()

	for _, outbound := range outbounds {
		if outbound.Tag == tag {
			return outbound
		}
	}
	t.Fatalf("outbound %q not found", tag)
	return testOutbound{}
}

func mustContain(t *testing.T, value string, needle string) {
	t.Helper()
	if !strings.Contains(value, needle) {
		t.Fatalf("value missing %q:\n%s", needle, value)
	}
}

func mustNotContain(t *testing.T, value string, needle string) {
	t.Helper()
	if strings.Contains(value, needle) {
		t.Fatalf("value contains %q:\n%s", needle, value)
	}
}
