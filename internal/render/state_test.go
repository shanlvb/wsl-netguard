package render

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestStateJSONRendersRequiredFields(t *testing.T) {
	renderedAt := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	state := RuntimeState{
		WindowsHostIP:    netip.MustParseAddr("172.25.80.1"),
		GatewayInterface: "eth0",
		TUNInterface:     "wng-tun",
		RenderedAt:       renderedAt,
	}

	data, err := StateJSON(state)
	if err != nil {
		t.Fatalf("StateJSON returned error: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("StateJSON returned invalid JSON:\n%s", string(data))
	}

	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal state JSON: %v", err)
	}
	want := map[string]string{
		"windows_host_ip":   "172.25.80.1",
		"gateway_interface": "eth0",
		"tun_interface":     "wng-tun",
		"rendered_at":       "2026-06-12T09:30:00Z",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("state[%s] = %q; want %q\nstate=%s", key, got[key], value, string(data))
		}
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("state JSON should end with newline:\n%s", string(data))
	}
}

func TestStateJSONRejectsInvalidInput(t *testing.T) {
	validState := RuntimeState{
		WindowsHostIP:    netip.MustParseAddr("172.25.80.1"),
		GatewayInterface: "eth0",
		TUNInterface:     "wng-tun",
		RenderedAt:       time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC),
	}
	tests := []struct {
		name   string
		mutate func(RuntimeState) RuntimeState
	}{
		{
			name: "missing windows host",
			mutate: func(state RuntimeState) RuntimeState {
				state.WindowsHostIP = netip.Addr{}
				return state
			},
		},
		{
			name: "ipv6 windows host",
			mutate: func(state RuntimeState) RuntimeState {
				state.WindowsHostIP = netip.MustParseAddr("fe80::1")
				return state
			},
		},
		{
			name: "missing gateway interface",
			mutate: func(state RuntimeState) RuntimeState {
				state.GatewayInterface = ""
				return state
			},
		},
		{
			name: "missing tun interface",
			mutate: func(state RuntimeState) RuntimeState {
				state.TUNInterface = ""
				return state
			},
		},
		{
			name: "missing render time",
			mutate: func(state RuntimeState) RuntimeState {
				state.RenderedAt = time.Time{}
				return state
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := StateJSON(tt.mutate(validState)); err == nil {
				t.Fatal("StateJSON returned nil error; want error")
			}
		})
	}
}
