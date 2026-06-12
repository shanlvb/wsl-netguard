package diagnostic

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/installfiles"
)

func TestProxyPortChecksProbeConfiguredV2rayNPorts(t *testing.T) {
	cfg := config.Default()
	cfg.SocksPort = 20808
	cfg.HTTPPort = 20809
	var probed []string
	probe := func(_ context.Context, address string) error {
		probed = append(probed, address)
		if strings.HasSuffix(address, ":20809") {
			return errors.New("connection refused")
		}
		return nil
	}

	checks := ProxyPortChecks(context.Background(), probe, "172.25.80.1", cfg)

	wantChecks := []Check{
		{Name: "v2rayn_socks", Status: "reachable", Detail: "172.25.80.1:20808"},
		{Name: "v2rayn_http", Status: "unreachable", Detail: "172.25.80.1:20809"},
	}
	if !reflect.DeepEqual(checks, wantChecks) {
		t.Fatalf("ProxyPortChecks = %+v; want %+v", checks, wantChecks)
	}
	wantProbed := []string{"172.25.80.1:20808", "172.25.80.1:20809"}
	if !reflect.DeepEqual(probed, wantProbed) {
		t.Fatalf("probed = %+v; want %+v", probed, wantProbed)
	}
}

func TestConfiguredProxyCheckProbesOnlyConfiguredProtocol(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(config.Config) config.Config
		wantCheck  Check
		wantProbed []string
	}{
		{
			name: "socks",
			configure: func(cfg config.Config) config.Config {
				cfg.ProxyProtocol = config.ProxyProtocolSocks
				cfg.SocksPort = 20808
				cfg.HTTPPort = 20809
				return cfg
			},
			wantCheck:  Check{Name: "v2rayn_socks", Status: "reachable", Detail: "172.25.80.1:20808"},
			wantProbed: []string{"172.25.80.1:20808"},
		},
		{
			name: "http",
			configure: func(cfg config.Config) config.Config {
				cfg.ProxyProtocol = config.ProxyProtocolHTTP
				cfg.SocksPort = 20808
				cfg.HTTPPort = 20809
				return cfg
			},
			wantCheck:  Check{Name: "v2rayn_http", Status: "reachable", Detail: "172.25.80.1:20809"},
			wantProbed: []string{"172.25.80.1:20809"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var probed []string
			probe := func(_ context.Context, address string) error {
				probed = append(probed, address)
				return nil
			}

			check := ConfiguredProxyCheck(context.Background(), probe, "172.25.80.1", tt.configure(config.Default()))

			if check != tt.wantCheck {
				t.Fatalf("ConfiguredProxyCheck = %+v; want %+v", check, tt.wantCheck)
			}
			if !reflect.DeepEqual(probed, tt.wantProbed) {
				t.Fatalf("probed = %+v; want %+v", probed, tt.wantProbed)
			}
		})
	}
}

func TestRuntimeChecksReportManagedState(t *testing.T) {
	cfg := config.Default()
	cfg.TUNInterface = "custom-tun"
	runner := &fakeOutputRunner{
		outputs: map[string][]byte{
			commandKey("systemctl", "is-active", "wsl-netguard-killswitch.service"): []byte("active\n"),
			commandKey("ip", "link", "show", "dev", "custom-tun"):                   []byte("12: custom-tun: <POINTOPOINT,UP>\n"),
			commandKey("nft", "list", "table", "inet", "wsl_netguard"):              []byte("table inet wsl_netguard {}\n"),
		},
		errors: map[string]error{
			commandKey("systemctl", "is-active", "wsl-netguard-sing-box.service"): errors.New("inactive"),
		},
	}

	checks := RuntimeChecks(context.Background(), runner, cfg)

	want := []Check{
		{Name: "kill_switch_service", Status: "active", Detail: "systemctl is-active wsl-netguard-killswitch.service"},
		{Name: "sing_box_service", Status: "inactive", Detail: "systemctl is-active wsl-netguard-sing-box.service"},
		{Name: "tun_status", Status: "active", Detail: "custom-tun"},
		{Name: "nftables_table", Status: "active", Detail: "inet wsl_netguard"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("RuntimeChecks = %+v; want %+v", checks, want)
	}

	wantCommands := [][]string{
		{"systemctl", "is-active", "wsl-netguard-killswitch.service"},
		{"systemctl", "is-active", "wsl-netguard-sing-box.service"},
		{"ip", "link", "show", "dev", "custom-tun"},
		{"nft", "list", "table", "inet", "wsl_netguard"},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %+v; want %+v", runner.commands, wantCommands)
	}
}

func TestRuntimeChecksReportMissingTunAndNFTablesTable(t *testing.T) {
	cfg := config.Default()
	runner := &fakeOutputRunner{
		errors: map[string]error{
			commandKey("systemctl", "is-active", "wsl-netguard-killswitch.service"): errors.New("inactive"),
			commandKey("systemctl", "is-active", "wsl-netguard-sing-box.service"):   errors.New("inactive"),
			commandKey("ip", "link", "show", "dev", "wng-tun"):                      errors.New("device not found"),
			commandKey("nft", "list", "table", "inet", "wsl_netguard"):              errors.New("no such file"),
		},
	}

	checks := RuntimeChecks(context.Background(), runner, cfg)

	want := []Check{
		{Name: "kill_switch_service", Status: "inactive", Detail: "systemctl is-active wsl-netguard-killswitch.service"},
		{Name: "sing_box_service", Status: "inactive", Detail: "systemctl is-active wsl-netguard-sing-box.service"},
		{Name: "tun_status", Status: "missing", Detail: "wng-tun"},
		{Name: "nftables_table", Status: "missing", Detail: "inet wsl_netguard"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("RuntimeChecks = %+v; want %+v", checks, want)
	}
}

func TestDependencyChecksReportRequiredAndOptionalCommands(t *testing.T) {
	runner := &fakeOutputRunner{
		outputs: map[string][]byte{
			commandKey("systemctl", "--version"):                  []byte("systemd 255\n"),
			commandKey("nft", "--version"):                        []byte("nftables v1.0.9\n"),
			commandKey(installfiles.SingBoxBinaryPath, "version"): []byte("sing-box 1.13.13\n"),
			commandKey("ip", "-Version"):                          []byte("ip utility\n"),
			commandKey("curl", "--version"):                       []byte("curl 8.0\n"),
			commandKey("journalctl", "--version"):                 []byte("systemd 255\n"),
		},
		errors: map[string]error{
			commandKey("nc", "-h"):                errors.New("missing"),
			commandKey("resolvectl", "--version"): errors.New("missing"),
		},
	}

	checks := DependencyChecks(context.Background(), runner)

	want := []Check{
		{Name: "dependency_systemd", Status: "installed", Detail: "required"},
		{Name: "dependency_nft", Status: "installed", Detail: "required"},
		{Name: "dependency_sing_box", Status: "installed", Detail: "required"},
		{Name: "dependency_ip", Status: "installed", Detail: "required"},
		{Name: "dependency_curl", Status: "installed", Detail: "required"},
		{Name: "dependency_nc", Status: "missing", Detail: "optional"},
		{Name: "dependency_journalctl", Status: "installed", Detail: "optional"},
		{Name: "dependency_resolvectl", Status: "missing", Detail: "optional"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("DependencyChecks = %+v; want %+v", checks, want)
	}
}

func TestEnvironmentChecksReportTunDeviceAndDNSConfig(t *testing.T) {
	runner := &fakeOutputRunner{
		outputs: map[string][]byte{
			commandKey("test", "-c", "/dev/net/tun"):     []byte{},
			commandKey("test", "-s", "/etc/resolv.conf"): []byte{},
		},
	}

	checks := EnvironmentChecks(context.Background(), runner)

	want := []Check{
		{Name: "tun_device", Status: "available", Detail: "/dev/net/tun"},
		{Name: "dns_config", Status: "available", Detail: "/etc/resolv.conf"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("EnvironmentChecks = %+v; want %+v", checks, want)
	}

	wantCommands := [][]string{
		{"test", "-c", "/dev/net/tun"},
		{"test", "-s", "/etc/resolv.conf"},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %+v; want %+v", runner.commands, wantCommands)
	}
}

func TestEnvironmentChecksReportMissingTunDeviceAndDNSConfig(t *testing.T) {
	runner := &fakeOutputRunner{
		errors: map[string]error{
			commandKey("test", "-c", "/dev/net/tun"):     errors.New("missing tun"),
			commandKey("test", "-s", "/etc/resolv.conf"): errors.New("missing dns"),
		},
	}

	checks := EnvironmentChecks(context.Background(), runner)

	want := []Check{
		{Name: "tun_device", Status: "missing", Detail: "/dev/net/tun"},
		{Name: "dns_config", Status: "missing", Detail: "/etc/resolv.conf"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("EnvironmentChecks = %+v; want %+v", checks, want)
	}
}

func TestNetworkTestChecksReportProxyDNSAndDirectBlock(t *testing.T) {
	runner := &fakeOutputRunner{
		outputs: map[string][]byte{
			commandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "https://api.ipify.org"):                                                                           []byte("203.0.113.10\n"),
			commandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "-H", "accept: application/dns-json", "https://one.one.one.one/dns-query?name=example.com&type=A"): []byte(`{"Status":0}`),
		},
		errors: map[string]error{
			commandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "--noproxy", "*", "--interface", "eth0", "https://api.ipify.org"): errors.New("blocked"),
		},
	}

	checks := NetworkTestChecks(context.Background(), runner, "eth0")

	want := []Check{
		{Name: "proxy_external", Status: "ok", Detail: "https://api.ipify.org"},
		{Name: "dns_query", Status: "ok", Detail: "https://one.one.one.one/dns-query?name=example.com&type=A"},
		{Name: "direct_external", Status: "blocked", Detail: "interface eth0"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("NetworkTestChecks = %+v; want %+v", checks, want)
	}

	wantCommands := [][]string{
		{"curl", "--max-time", "8", "--silent", "--show-error", "--fail", "https://api.ipify.org"},
		{"curl", "--max-time", "8", "--silent", "--show-error", "--fail", "-H", "accept: application/dns-json", "https://one.one.one.one/dns-query?name=example.com&type=A"},
		{"curl", "--max-time", "8", "--silent", "--show-error", "--fail", "--noproxy", "*", "--interface", "eth0", "https://api.ipify.org"},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %+v; want %+v", runner.commands, wantCommands)
	}
}

func TestNetworkTestChecksReportFailuresAndDirectBypass(t *testing.T) {
	runner := &fakeOutputRunner{
		outputs: map[string][]byte{
			commandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "--noproxy", "*", "--interface", "eth0", "https://api.ipify.org"): []byte("198.51.100.20\n"),
		},
		errors: map[string]error{
			commandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "https://api.ipify.org"):                                                                           errors.New("proxy failed"),
			commandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "-H", "accept: application/dns-json", "https://one.one.one.one/dns-query?name=example.com&type=A"): errors.New("dns failed"),
		},
	}

	checks := NetworkTestChecks(context.Background(), runner, "eth0")

	want := []Check{
		{Name: "proxy_external", Status: "failed", Detail: "https://api.ipify.org"},
		{Name: "dns_query", Status: "failed", Detail: "https://one.one.one.one/dns-query?name=example.com&type=A"},
		{Name: "direct_external", Status: "reachable", Detail: "interface eth0"},
	}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("NetworkTestChecks = %+v; want %+v", checks, want)
	}
}

type fakeOutputRunner struct {
	commands [][]string
	outputs  map[string][]byte
	errors   map[string]error
}

func (runner *fakeOutputRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	command := append([]string{name}, args...)
	runner.commands = append(runner.commands, command)
	key := strings.Join(command, "\x00")

	if err, ok := runner.errors[key]; ok {
		return nil, err
	}
	if output, ok := runner.outputs[key]; ok {
		return output, nil
	}
	return nil, errors.New("unexpected command: " + strings.Join(command, " "))
}

func commandKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}
