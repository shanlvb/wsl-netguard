package render

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"time"
)

type RuntimeState struct {
	WindowsHostIP    netip.Addr
	GatewayInterface string
	TUNInterface     string
	RenderedAt       time.Time
}

type runtimeStateDocument struct {
	WindowsHostIP    string `json:"windows_host_ip"`
	GatewayInterface string `json:"gateway_interface"`
	TUNInterface     string `json:"tun_interface"`
	RenderedAt       string `json:"rendered_at"`
}

func StateJSON(state RuntimeState) ([]byte, error) {
	if err := validateState(state); err != nil {
		return nil, err
	}

	doc := runtimeStateDocument{
		WindowsHostIP:    state.WindowsHostIP.String(),
		GatewayInterface: state.GatewayInterface,
		TUNInterface:     state.TUNInterface,
		RenderedAt:       state.RenderedAt.UTC().Format(time.RFC3339),
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal runtime state: %w", err)
	}
	return append(data, '\n'), nil
}

func validateState(state RuntimeState) error {
	if !state.WindowsHostIP.IsValid() {
		return fmt.Errorf("windows host IP must be set")
	}
	if !state.WindowsHostIP.Is4() {
		return fmt.Errorf("windows host IP %q must be IPv4", state.WindowsHostIP)
	}
	if state.GatewayInterface == "" {
		return fmt.Errorf("gateway interface must be set")
	}
	if state.TUNInterface == "" {
		return fmt.Errorf("tun interface must be set")
	}
	if state.RenderedAt.IsZero() {
		return fmt.Errorf("rendered_at must be set")
	}
	return nil
}
