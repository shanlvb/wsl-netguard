package recovery

import (
	"reflect"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

func TestDisableActionsTargetOnlyManagedResources(t *testing.T) {
	cfg := config.Default()
	cfg.TUNInterface = "custom-tun"

	actions, err := DisableActions(cfg)
	if err != nil {
		t.Fatalf("DisableActions returned error: %v", err)
	}

	want := []Action{
		{Summary: "stop sing-box service", Command: []string{"systemctl", "stop", "wsl-netguard-sing-box.service"}},
		{Summary: "stop kill switch service", Command: []string{"systemctl", "stop", "wsl-netguard-killswitch.service"}},
		{Summary: "delete managed nftables table", Command: []string{"nft", "delete", "table", "inet", "wsl_netguard"}},
		{Summary: "delete managed TUN interface", Command: []string{"ip", "link", "delete", "custom-tun"}},
		{Summary: "remove runtime directory", Command: []string{"rm", "-rf", "/run/wsl-netguard"}},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("DisableActions = %+v; want %+v", actions, want)
	}
	assertRecoveryPlanSafety(t, actions)
}

func TestDisableActionsRejectsInvalidConfig(t *testing.T) {
	cfg := config.Default()
	cfg.TUNInterface = "bad interface"

	if _, err := DisableActions(cfg); err == nil {
		t.Fatal("DisableActions returned nil error; want invalid config error")
	}
}

func TestRescueActionsDoNotRequireConfig(t *testing.T) {
	actions := RescueActions()

	want := []Action{
		{Summary: "stop sing-box service", Command: []string{"systemctl", "stop", "wsl-netguard-sing-box.service"}},
		{Summary: "stop kill switch service", Command: []string{"systemctl", "stop", "wsl-netguard-killswitch.service"}},
		{Summary: "delete managed nftables table", Command: []string{"nft", "delete", "table", "inet", "wsl_netguard"}},
		{Summary: "delete managed TUN interface", Command: []string{"ip", "link", "delete", "wng-tun"}},
		{Summary: "remove runtime directory", Command: []string{"rm", "-rf", "/run/wsl-netguard"}},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("RescueActions = %+v; want %+v", actions, want)
	}
	assertRecoveryPlanSafety(t, actions)
}

func assertRecoveryPlanSafety(t *testing.T, actions []Action) {
	t.Helper()

	for _, action := range actions {
		joined := strings.Join(action.Command, " ")
		if strings.Contains(joined, "flush ruleset") {
			t.Fatalf("action uses forbidden flush ruleset: %+v", action)
		}
		if strings.Contains(joined, "nft delete table") && joined != "nft delete table inet wsl_netguard" {
			t.Fatalf("action deletes unexpected nftables resource: %+v", action)
		}
	}
}
