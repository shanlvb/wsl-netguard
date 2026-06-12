package runtimefiles

import (
	"fmt"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/netdetect"
	"github.com/shanlvb/wsl-netguard/internal/render"
)

const (
	RuntimeDir        = "/run/wsl-netguard"
	SingBoxPath       = RuntimeDir + "/sing-box.json"
	NFTablesPath      = RuntimeDir + "/nftables.nft"
	NFTablesCheckPath = RuntimeDir + "/nftables.check.nft"
	StatePath         = RuntimeDir + "/state.json"
)

type Artifact struct {
	Path string
	Data []byte
	Mode uint32
}

func PlanArtifacts(cfg config.Config, gateway netdetect.Gateway, renderedAt time.Time) ([]Artifact, error) {
	singBoxConfig, err := render.SingBoxJSON(cfg, gateway.Address)
	if err != nil {
		return nil, fmt.Errorf("render sing-box config: %w", err)
	}

	nftablesRules, err := render.NFTablesRules(cfg, gateway.Address)
	if err != nil {
		return nil, fmt.Errorf("render nftables rules: %w", err)
	}
	nftablesCheckRules, err := render.NFTablesCheckRules(cfg, gateway.Address)
	if err != nil {
		return nil, fmt.Errorf("render nftables check rules: %w", err)
	}

	stateJSON, err := render.StateJSON(render.RuntimeState{
		WindowsHostIP:    gateway.Address,
		GatewayInterface: gateway.Interface,
		TUNInterface:     cfg.TUNInterface,
		RenderedAt:       renderedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("render runtime state: %w", err)
	}

	return []Artifact{
		{Path: SingBoxPath, Data: singBoxConfig, Mode: 0o644},
		{Path: NFTablesPath, Data: []byte(nftablesRules), Mode: 0o644},
		{Path: NFTablesCheckPath, Data: []byte(nftablesCheckRules), Mode: 0o644},
		{Path: StatePath, Data: stateJSON, Mode: 0o644},
	}, nil
}
