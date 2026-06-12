package netdetect

import (
	"fmt"
	"net/netip"
	"strings"
)

type Gateway struct {
	Address   netip.Addr
	Interface string
}

func ParseDefaultGateway(output string, excludedInterface string) (Gateway, error) {
	var skipped bool
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "default" {
			continue
		}

		iface, hasInterface := fieldAfter(fields, "dev")
		if hasInterface && iface == excludedInterface {
			skipped = true
			continue
		}

		gateway, hasGateway := fieldAfter(fields, "via")
		if !hasGateway {
			return Gateway{}, fmt.Errorf("default route is missing gateway address: %q", line)
		}
		if !hasInterface {
			return Gateway{}, fmt.Errorf("default route is missing interface: %q", line)
		}

		address, err := netip.ParseAddr(gateway)
		if err != nil {
			return Gateway{}, fmt.Errorf("parse gateway address %q: %w", gateway, err)
		}
		if !address.Is4() {
			return Gateway{}, fmt.Errorf("gateway address %q must be IPv4", gateway)
		}

		return Gateway{
			Address:   address,
			Interface: iface,
		}, nil
	}

	if skipped {
		return Gateway{}, fmt.Errorf("no default gateway found after excluding interface %q", excludedInterface)
	}
	return Gateway{}, fmt.Errorf("no default gateway found")
}

func fieldAfter(fields []string, name string) (string, bool) {
	for i, field := range fields {
		if field == name && i+1 < len(fields) {
			return fields[i+1], true
		}
	}
	return "", false
}
