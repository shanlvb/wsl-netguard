package render

import (
	"strings"
	"testing"
)

func TestKillSwitchUnitRendersDocumentedOrdering(t *testing.T) {
	unit, err := KillSwitchUnit("/usr/local/bin/wsl-netguard")
	if err != nil {
		t.Fatalf("KillSwitchUnit returned error: %v", err)
	}

	for _, want := range []string{
		"[Unit]",
		"Description=WSL outbound kill switch",
		"After=network-online.target",
		"Wants=network-online.target",
		"Before=wsl-netguard-sing-box.service",
		"[Service]",
		"Type=oneshot",
		"ExecStart=/usr/local/bin/wsl-netguard service apply-killswitch",
		"RemainAfterExit=yes",
		"[Install]",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("kill switch unit missing %q:\n%s", want, unit)
		}
	}
}

func TestSingBoxUnitRendersDocumentedDependencies(t *testing.T) {
	unit, err := SingBoxUnit("/usr/local/bin/wsl-netguard")
	if err != nil {
		t.Fatalf("SingBoxUnit returned error: %v", err)
	}

	for _, want := range []string{
		"[Unit]",
		"Description=sing-box TUN proxy for WSL",
		"After=wsl-netguard-killswitch.service",
		"Requires=wsl-netguard-killswitch.service",
		"[Service]",
		"Type=simple",
		"ExecStartPre=/usr/local/bin/wsl-netguard service render-sing-box",
		"ExecStart=/usr/bin/env sing-box run -c /run/wsl-netguard/sing-box.json",
		"Restart=always",
		"RestartSec=3",
		"AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW",
		"CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW",
		"NoNewPrivileges=true",
		"[Install]",
		"WantedBy=multi-user.target",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("sing-box unit missing %q:\n%s", want, unit)
		}
	}
}

func TestSystemdUnitsRejectRelativeBinaryPath(t *testing.T) {
	for _, renderUnit := range []struct {
		name string
		fn   func(string) (string, error)
	}{
		{name: "kill switch", fn: KillSwitchUnit},
		{name: "sing-box", fn: SingBoxUnit},
	} {
		t.Run(renderUnit.name, func(t *testing.T) {
			if _, err := renderUnit.fn("wsl-netguard"); err == nil {
				t.Fatal("unit renderer returned nil error for relative binary path; want error")
			}
		})
	}
}
