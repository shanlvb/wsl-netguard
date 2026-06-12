package netdetect

import (
	"net/netip"
	"strings"
	"testing"
)

func TestParseDefaultGateway(t *testing.T) {
	tests := []struct {
		name              string
		output            string
		excludedInterface string
		want              Gateway
	}{
		{
			name:              "wsl default route",
			output:            "default via 172.25.80.1 dev eth0 proto kernel\n",
			excludedInterface: "wng-tun",
			want: Gateway{
				Address:   netip.MustParseAddr("172.25.80.1"),
				Interface: "eth0",
			},
		},
		{
			name: "skips managed tun default route",
			output: strings.Join([]string{
				"default dev wng-tun scope link",
				"default via 172.28.96.1 dev eth0 proto kernel",
			}, "\n"),
			excludedInterface: "wng-tun",
			want: Gateway{
				Address:   netip.MustParseAddr("172.28.96.1"),
				Interface: "eth0",
			},
		},
		{
			name:              "interface exclusion is exact",
			output:            "default via 172.28.96.1 dev wng-tun-extra proto kernel\n",
			excludedInterface: "wng-tun",
			want: Gateway{
				Address:   netip.MustParseAddr("172.28.96.1"),
				Interface: "wng-tun-extra",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDefaultGateway(tt.output, tt.excludedInterface)
			if err != nil {
				t.Fatalf("ParseDefaultGateway returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseDefaultGateway = %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestParseDefaultGatewayRejectsInvalidRoutes(t *testing.T) {
	tests := []struct {
		name              string
		output            string
		excludedInterface string
		wantErr           string
	}{
		{
			name:              "empty output",
			output:            "",
			excludedInterface: "wng-tun",
			wantErr:           "default gateway",
		},
		{
			name:              "only excluded tun route",
			output:            "default via 172.19.0.2 dev wng-tun\n",
			excludedInterface: "wng-tun",
			wantErr:           "default gateway",
		},
		{
			name:              "missing via",
			output:            "default dev eth0 proto kernel\n",
			excludedInterface: "wng-tun",
			wantErr:           "gateway address",
		},
		{
			name:              "missing dev",
			output:            "default via 172.25.80.1 proto kernel\n",
			excludedInterface: "wng-tun",
			wantErr:           "interface",
		},
		{
			name:              "invalid ip",
			output:            "default via not-an-ip dev eth0\n",
			excludedInterface: "wng-tun",
			wantErr:           "parse gateway address",
		},
		{
			name:              "ipv6 route",
			output:            "default via fe80::1 dev eth0\n",
			excludedInterface: "wng-tun",
			wantErr:           "IPv4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDefaultGateway(tt.output, tt.excludedInterface)
			if err == nil {
				t.Fatal("ParseDefaultGateway returned nil error; want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseDefaultGateway error = %q; want mention %q", err.Error(), tt.wantErr)
			}
		})
	}
}
