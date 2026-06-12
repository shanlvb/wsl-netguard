package installfiles

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

func TestPlanArtifactsIncludesManagedInstallFiles(t *testing.T) {
	artifacts, err := PlanArtifacts(config.Default())
	if err != nil {
		t.Fatalf("PlanArtifacts returned error: %v", err)
	}

	wantPaths := []string{
		ConfigPath,
		SingBoxTemplatePath,
		KillSwitchUnitPath,
		SingBoxUnitPath,
	}
	if len(artifacts) != len(wantPaths) {
		t.Fatalf("artifact length = %d; want %d", len(artifacts), len(wantPaths))
	}
	for i, wantPath := range wantPaths {
		if artifacts[i].Path != wantPath {
			t.Fatalf("artifact[%d] path = %q; want %q", i, artifacts[i].Path, wantPath)
		}
		if artifacts[i].Mode != 0o644 {
			t.Fatalf("artifact[%d] mode = %#o; want 0644", i, artifacts[i].Mode)
		}
		if len(artifacts[i].Data) == 0 {
			t.Fatalf("artifact[%d] data is empty", i)
		}
	}
}

func TestPlanArtifactsRendersConfigAndTemplates(t *testing.T) {
	cfg := config.Default()
	cfg.Mode = config.ModeStrict
	cfg.SocksPort = 20808

	artifacts, err := PlanArtifacts(cfg)
	if err != nil {
		t.Fatalf("PlanArtifacts returned error: %v", err)
	}

	var savedConfig config.Config
	if err := json.Unmarshal(artifacts[0].Data, &savedConfig); err != nil {
		t.Fatalf("config artifact is invalid JSON: %v\n%s", err, string(artifacts[0].Data))
	}
	if savedConfig != cfg {
		t.Fatalf("config artifact = %+v; want %+v", savedConfig, cfg)
	}

	template := string(artifacts[1].Data)
	if !strings.Contains(template, `"server": "__WIN_HOST__"`) {
		t.Fatalf("template missing host placeholder:\n%s", template)
	}
	if !strings.Contains(template, `"server_port": 20808`) {
		t.Fatalf("template missing configured SOCKS port:\n%s", template)
	}

	killSwitchUnit := string(artifacts[2].Data)
	if !strings.Contains(killSwitchUnit, "ExecStart=/usr/local/bin/wsl-netguard service apply-killswitch") {
		t.Fatalf("kill switch unit missing service hook:\n%s", killSwitchUnit)
	}

	singBoxUnit := string(artifacts[3].Data)
	if !strings.Contains(singBoxUnit, "ExecStartPre=/usr/local/bin/wsl-netguard service render-sing-box") {
		t.Fatalf("sing-box unit missing render hook:\n%s", singBoxUnit)
	}
}

func TestPlanArtifactsRejectsInvalidConfig(t *testing.T) {
	cfg := config.Default()
	cfg.TUNInterface = "bad interface"

	if _, err := PlanArtifacts(cfg); err == nil {
		t.Fatal("PlanArtifacts returned nil error for invalid config; want error")
	}
}

func TestPlanArtifactsRejectsUnsafeBinaryPath(t *testing.T) {
	if _, err := planArtifacts(config.Default(), "wsl-netguard"); err == nil {
		t.Fatal("planArtifacts returned nil error for relative binary path; want error")
	}
}
