package render

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

func KillSwitchUnit(binaryPath string) (string, error) {
	if err := validateBinaryPath(binaryPath); err != nil {
		return "", err
	}

	return fmt.Sprintf(`[Unit]
Description=WSL outbound kill switch
After=network-online.target
Wants=network-online.target
Before=wsl-netguard-sing-box.service

[Service]
Type=oneshot
ExecStart=%s service apply-killswitch
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`, binaryPath), nil
}

func SingBoxUnit(binaryPath string, singBoxPath string) (string, error) {
	if err := validateBinaryPath(binaryPath); err != nil {
		return "", err
	}
	if err := validateBinaryPath(singBoxPath); err != nil {
		return "", err
	}

	return fmt.Sprintf(`[Unit]
Description=sing-box TUN proxy for WSL
After=wsl-netguard-killswitch.service
Requires=wsl-netguard-killswitch.service

[Service]
Type=simple
ExecStartPre=%s service render-sing-box
ExecStart=%s run -c /run/wsl-netguard/sing-box.json
Restart=always
RestartSec=3
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`, binaryPath, singBoxPath), nil
}

func validateBinaryPath(binaryPath string) error {
	if binaryPath == "" {
		return fmt.Errorf("binary path must not be empty")
	}
	if !filepath.IsAbs(binaryPath) {
		return fmt.Errorf("binary path %q must be absolute", binaryPath)
	}
	if strings.ContainsFunc(binaryPath, unicode.IsSpace) {
		return fmt.Errorf("binary path %q must not contain whitespace", binaryPath)
	}
	return nil
}
