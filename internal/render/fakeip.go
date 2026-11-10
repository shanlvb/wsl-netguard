package render

import (
	"fmt"
	"net/netip"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

const (
	StateDirectory  = "/var/lib/wsl-netguard"
	FakeIPCachePath = StateDirectory + "/cache.db"
)

// Keep recovery able to load older configurations; reject unsupported families
// only when generating a new interception configuration.
func validateFakeIPConfig(cfg config.Config) error {
	if cfg.DNSStrategy != config.DNSStrategyIPv4Only {
		return fmt.Errorf("FakeIP currently requires dns_strategy ipv4_only; IPv6 TUN interception is not supported")
	}
	if netip.MustParsePrefix("198.18.0.0/15").Overlaps(netip.MustParsePrefix(cfg.TUNAddress)) {
		return fmt.Errorf("tun_address overlaps the reserved FakeIP range 198.18.0.0/15")
	}
	return nil
}

type singBoxDNSRule struct {
	QueryType []string `json:"query_type"`
	Action    string   `json:"action"`
	Server    string   `json:"server"`
}

type singBoxExperimental struct {
	CacheFile singBoxCacheFile `json:"cache_file"`
}
type singBoxCacheFile struct {
	Enabled     bool   `json:"enabled"`
	Path        string `json:"path"`
	StoreFakeIP bool   `json:"store_fakeip"`
}
