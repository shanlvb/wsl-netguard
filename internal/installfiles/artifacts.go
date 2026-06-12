package installfiles

import (
	"encoding/json"
	"fmt"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/render"
	"github.com/shanlvb/wsl-netguard/internal/runtimefiles"
)

const (
	BinaryPath          = "/usr/local/bin/wsl-netguard"
	ManagedLibDir       = "/usr/local/lib/wsl-netguard"
	SingBoxBinaryPath   = ManagedLibDir + "/sing-box"
	ConfigDir           = "/etc/wsl-netguard"
	ConfigPath          = ConfigDir + "/config.json"
	SingBoxTemplatePath = ConfigDir + "/sing-box.template.json"
	KillSwitchUnitPath  = "/etc/systemd/system/wsl-netguard-killswitch.service"
	SingBoxUnitPath     = "/etc/systemd/system/wsl-netguard-sing-box.service"
)

func PlanArtifacts(cfg config.Config) ([]runtimefiles.Artifact, error) {
	return planArtifacts(cfg, BinaryPath)
}

func planArtifacts(cfg config.Config, binaryPath string) ([]runtimefiles.Artifact, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	configJSON = append(configJSON, '\n')

	singBoxTemplate, err := render.SingBoxTemplateJSON(cfg)
	if err != nil {
		return nil, fmt.Errorf("render sing-box template: %w", err)
	}
	killSwitchUnit, err := render.KillSwitchUnit(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("render kill switch unit: %w", err)
	}
	singBoxUnit, err := render.SingBoxUnit(binaryPath, SingBoxBinaryPath)
	if err != nil {
		return nil, fmt.Errorf("render sing-box unit: %w", err)
	}

	return []runtimefiles.Artifact{
		{Path: ConfigPath, Data: configJSON, Mode: 0o644},
		{Path: SingBoxTemplatePath, Data: singBoxTemplate, Mode: 0o644},
		{Path: KillSwitchUnitPath, Data: []byte(killSwitchUnit), Mode: 0o644},
		{Path: SingBoxUnitPath, Data: []byte(singBoxUnit), Mode: 0o644},
	}, nil
}
