package recovery

import (
	"fmt"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

const (
	SingBoxService      = "wsl-netguard-sing-box.service"
	KillSwitchService   = "wsl-netguard-killswitch.service"
	NFTablesFamily      = "inet"
	NFTablesTable       = "wsl_netguard"
	RuntimeDir          = "/run/wsl-netguard"
	DefaultTUNInterface = "wng-tun"
)

type Action struct {
	Summary string
	Command []string
}

func DisableActions(cfg config.Config) ([]Action, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config for disable: %w", err)
	}
	return recoveryActions(cfg.TUNInterface), nil
}

func RescueActions() []Action {
	return recoveryActions(DefaultTUNInterface)
}

func recoveryActions(tunInterface string) []Action {
	return []Action{
		{
			Summary: "stop sing-box service",
			Command: []string{"systemctl", "stop", SingBoxService},
		},
		{
			Summary: "stop kill switch service",
			Command: []string{"systemctl", "stop", KillSwitchService},
		},
		{
			Summary: "delete managed nftables table",
			Command: []string{"nft", "delete", "table", NFTablesFamily, NFTablesTable},
		},
		{
			Summary: "delete managed TUN interface",
			Command: []string{"ip", "link", "delete", tunInterface},
		},
		{
			Summary: "remove runtime directory",
			Command: []string{"rm", "-rf", RuntimeDir},
		},
	}
}
