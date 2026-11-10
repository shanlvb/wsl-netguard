package render

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

func TestFakeIPDNSContract(t *testing.T) {
	data, err := SingBoxJSON(config.Default(), netip.MustParseAddr("172.25.80.1"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	dns := doc["dns"].(map[string]any)
	if !strings.Contains(string(data), `"type": "fakeip"`) {
		t.Fatal("missing FakeIP server")
	}
	if dns["final"] != "remote-doh" {
		t.Fatal("non-address DNS must retain proxied DoH")
	}
	rules := dns["rules"].([]any)
	rule := rules[len(rules)-1].(map[string]any)
	if rule["server"] != "fakeip" || len(rule["query_type"].([]any)) != 2 {
		t.Fatalf("address rule: %v", rule)
	}
	cache := doc["experimental"].(map[string]any)["cache_file"].(map[string]any)
	if cache["enabled"] != true || cache["store_fakeip"] != true || cache["path"] != "/var/lib/wsl-netguard/cache.db" {
		t.Fatalf("cache: %v", cache)
	}
	mustNotContain(t, string(data), `"action": "resolve"`)
	mustNotContain(t, string(data), `"action": "sniff"`)
}

func TestFakeIPRejectsUnsupportedAddressFamilies(t *testing.T) {
	for _, strategy := range []string{"prefer_ipv4", "prefer_ipv6", "ipv6_only"} {
		t.Run(strategy, func(t *testing.T) {
			cfg := config.Default()
			cfg.DNSStrategy = strategy
			if _, err := SingBoxJSON(cfg, netip.MustParseAddr("172.25.80.1")); err == nil {
				t.Fatal("unsupported strategy accepted")
			}
		})
	}
}

func TestPersistentCacheUnit(t *testing.T) {
	unit, err := SingBoxUnit("/usr/local/bin/wsl-netguard", "/usr/local/lib/wsl-netguard/sing-box")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"StateDirectory=wsl-netguard", "StateDirectoryMode=0700", "UMask=0077"} {
		mustContain(t, unit, want)
	}
}

func TestDockerForwardProtection(t *testing.T) {
	for _, mode := range []string{config.ModeDev, config.ModeStrict} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.Default()
			cfg.Mode = mode
			rules, err := NFTablesRules(cfg, netip.MustParseAddr("172.25.80.1"))
			if err != nil {
				t.Fatal(err)
			}
			mustContain(t, rules, `iifname "docker0" jump docker_egress`)
			mustContain(t, rules, `iifname "br-*" jump docker_egress`)
			mustContain(t, rules, `ct direction reply ct state established,related accept`)
			mustContain(t, rules, `oifname "wng-tun" accept`)
			mustContain(t, rules, "    counter drop")
			mustNotContain(t, rules, "flush ruleset")
		})
	}
}

func TestFakeIPRejectsOverlappingTUN(t *testing.T) {
	for _, address := range []string{"198.18.0.1/30", "198.19.1.1/24", "198.0.0.1/8"} {
		t.Run(address, func(t *testing.T) {
			cfg := config.Default()
			cfg.TUNAddress = address
			if _, err := SingBoxJSON(cfg, netip.MustParseAddr("172.25.80.1")); err == nil {
				t.Fatal("overlapping runtime address accepted")
			}
			if _, err := SingBoxTemplateJSON(cfg); err == nil {
				t.Fatal("overlapping template address accepted")
			}
		})
	}
}
