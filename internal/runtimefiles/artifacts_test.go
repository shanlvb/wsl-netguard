package runtimefiles

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/netdetect"
)

func TestPlanArtifactsRendersRuntimeFiles(t *testing.T) {
	cfg := config.Default()
	gateway := netdetect.Gateway{
		Address:   netip.MustParseAddr("172.25.80.1"),
		Interface: "eth0",
	}
	renderedAt := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)

	artifacts, err := PlanArtifacts(cfg, gateway, renderedAt)
	if err != nil {
		t.Fatalf("PlanArtifacts returned error: %v", err)
	}

	if len(artifacts) != 4 {
		t.Fatalf("artifact length = %d; want 4", len(artifacts))
	}

	assertArtifact(t, artifacts[0], SingBoxPath, 0o644)
	assertArtifact(t, artifacts[1], NFTablesPath, 0o644)
	assertArtifact(t, artifacts[2], NFTablesCheckPath, 0o644)
	assertArtifact(t, artifacts[3], StatePath, 0o644)

	var singBox map[string]any
	if err := json.Unmarshal(artifacts[0].Data, &singBox); err != nil {
		t.Fatalf("sing-box artifact is invalid JSON: %v\n%s", err, string(artifacts[0].Data))
	}
	if !json.Valid(artifacts[3].Data) {
		t.Fatalf("state artifact is invalid JSON:\n%s", string(artifacts[3].Data))
	}
	if got := string(artifacts[1].Data); !strings.HasPrefix(got, "table inet") {
		t.Fatalf("nftables artifact should start with table declaration:\n%s", got)
	}
	if got := string(artifacts[2].Data); !strings.Contains(got, "table inet wsl_netguard_check") {
		t.Fatalf("nftables check artifact should use validation table:\n%s", got)
	}
}

func TestPlanArtifactsRejectsInvalidConfig(t *testing.T) {
	cfg := config.Default()
	cfg.TUNInterface = "bad interface"
	gateway := netdetect.Gateway{
		Address:   netip.MustParseAddr("172.25.80.1"),
		Interface: "eth0",
	}

	if _, err := PlanArtifacts(cfg, gateway, time.Now()); err == nil {
		t.Fatal("PlanArtifacts returned nil error for invalid config; want error")
	}
}

func TestPlanArtifactsRejectsInvalidGatewayState(t *testing.T) {
	cfg := config.Default()
	gateway := netdetect.Gateway{
		Address: netip.MustParseAddr("172.25.80.1"),
	}

	if _, err := PlanArtifacts(cfg, gateway, time.Now()); err == nil {
		t.Fatal("PlanArtifacts returned nil error for missing gateway interface; want error")
	}
}

func assertArtifact(t *testing.T, artifact Artifact, path string, mode uint32) {
	t.Helper()
	if artifact.Path != path {
		t.Fatalf("artifact path = %q; want %q", artifact.Path, path)
	}
	if artifact.Mode != mode {
		t.Fatalf("artifact mode = %#o; want %#o", artifact.Mode, mode)
	}
	if len(artifact.Data) == 0 {
		t.Fatalf("artifact %s data is empty", path)
	}
}
