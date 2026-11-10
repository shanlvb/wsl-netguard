package render

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

// Limit forwarding policy to Docker's standard bridge interfaces. Other
// forwarding users keep their own rules; no Docker-owned table is modified.
func writeDockerForwardRules(b *strings.Builder, cfg config.Config, host netip.Addr) {
	fmt.Fprintln(b, "  chain docker_egress {")
	// Published-port replies must retain their original return route. Original
	// outbound connections must still pass the egress checks after TUN failure.
	fmt.Fprintln(b, "    ct direction reply ct state established,related accept")
	fmt.Fprintf(b, "    oifname %q accept\n", cfg.TUNInterface)
	if cfg.Mode == config.ModeDev {
		writeDevModeLocalBridgeRules(b)
		fmt.Fprintf(b, "    ip daddr %s accept\n", host)
	} else {
		fmt.Fprintf(b, "    ip daddr %s tcp dport %s accept\n", host, strictTCPPortSet(cfg))
		fmt.Fprintf(b, "    ip daddr %s udp dport %d accept\n", host, cfg.SocksPort)
	}
	fmt.Fprintln(b, "    counter drop")
	fmt.Fprintln(b, "  }")
	fmt.Fprintln(b, "  chain forward {")
	fmt.Fprintln(b, "    type filter hook forward priority 0; policy accept;")
	for _, name := range []string{"docker0", "br-*"} {
		fmt.Fprintf(b, "    iifname %q jump docker_egress\n", name)
	}
	fmt.Fprintln(b, "  }")
}
