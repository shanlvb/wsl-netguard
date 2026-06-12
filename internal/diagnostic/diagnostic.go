package diagnostic

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/installfiles"
	"github.com/shanlvb/wsl-netguard/internal/recovery"
)

const tcpProbeTimeout = 2 * time.Second

const (
	proxyTestURL = "https://api.ipify.org"
	dnsTestURL   = "https://one.one.one.one/dns-query?name=example.com&type=A"
)

type OutputRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type TCPProbe func(ctx context.Context, address string) error

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func DefaultTCPProbe(ctx context.Context, address string) error {
	dialer := net.Dialer{Timeout: tcpProbeTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial %s: %w", address, err)
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("close %s: %w", address, err)
	}
	return nil
}

func ProxyPortChecks(ctx context.Context, probe TCPProbe, windowsHost string, cfg config.Config) []Check {
	return []Check{
		proxyPortCheck(ctx, probe, "v2rayn_socks", windowsHost, cfg.SocksPort),
		proxyPortCheck(ctx, probe, "v2rayn_http", windowsHost, cfg.HTTPPort),
	}
}

func ConfiguredProxyCheck(ctx context.Context, probe TCPProbe, windowsHost string, cfg config.Config) Check {
	if cfg.ProxyProtocol == config.ProxyProtocolHTTP {
		return proxyPortCheck(ctx, probe, "v2rayn_http", windowsHost, cfg.HTTPPort)
	}
	return proxyPortCheck(ctx, probe, "v2rayn_socks", windowsHost, cfg.SocksPort)
}

func RuntimeChecks(ctx context.Context, runner OutputRunner, cfg config.Config) []Check {
	return []Check{
		serviceCheck(ctx, runner, "kill_switch_service", recovery.KillSwitchService),
		serviceCheck(ctx, runner, "sing_box_service", recovery.SingBoxService),
		tunInterfaceCheck(ctx, runner, cfg.TUNInterface),
		nftablesTableCheck(ctx, runner),
	}
}

func DependencyChecks(ctx context.Context, runner OutputRunner) []Check {
	dependencies := []dependency{
		{name: "dependency_systemd", required: true, command: []string{"systemctl", "--version"}},
		{name: "dependency_nft", required: true, command: []string{"nft", "--version"}},
		{name: "dependency_sing_box", required: true, command: []string{installfiles.SingBoxBinaryPath, "version"}},
		{name: "dependency_ip", required: true, command: []string{"ip", "-Version"}},
		{name: "dependency_curl", required: true, command: []string{"curl", "--version"}},
		{name: "dependency_nc", required: false, command: []string{"nc", "-h"}},
		{name: "dependency_journalctl", required: false, command: []string{"journalctl", "--version"}},
		{name: "dependency_resolvectl", required: false, command: []string{"resolvectl", "--version"}},
	}

	checks := make([]Check, 0, len(dependencies))
	for _, dep := range dependencies {
		checks = append(checks, dependencyCheck(ctx, runner, dep))
	}
	return checks
}

func EnvironmentChecks(ctx context.Context, runner OutputRunner) []Check {
	return []Check{
		fileCheck(ctx, runner, "tun_device", []string{"test", "-c", "/dev/net/tun"}, "/dev/net/tun"),
		fileCheck(ctx, runner, "dns_config", []string{"test", "-s", "/etc/resolv.conf"}, "/etc/resolv.conf"),
	}
}

func NetworkTestChecks(ctx context.Context, runner OutputRunner, gatewayInterface string) []Check {
	return []Check{
		curlExpectedSuccessCheck(ctx, runner, "proxy_external", proxyTestURL),
		dnsQueryCheck(ctx, runner),
		directExternalCheck(ctx, runner, gatewayInterface),
	}
}

func proxyPortCheck(ctx context.Context, probe TCPProbe, name string, windowsHost string, port int) Check {
	address := net.JoinHostPort(windowsHost, fmt.Sprintf("%d", port))
	status := "reachable"
	if err := probe(ctx, address); err != nil {
		status = "unreachable"
	}
	return Check{Name: name, Status: status, Detail: address}
}

func serviceCheck(ctx context.Context, runner OutputRunner, name string, service string) Check {
	command := []string{"systemctl", "is-active", service}
	status := "inactive"
	output, err := runner.Output(ctx, command[0], command[1:]...)
	if err == nil {
		status = firstLine(output)
		if status == "" {
			status = "active"
		}
	}
	return Check{Name: name, Status: status, Detail: strings.Join(command, " ")}
}

func tunInterfaceCheck(ctx context.Context, runner OutputRunner, tunInterface string) Check {
	command := []string{"ip", "link", "show", "dev", tunInterface}
	status := "active"
	if _, err := runner.Output(ctx, command[0], command[1:]...); err != nil {
		status = "missing"
	}
	return Check{Name: "tun_status", Status: status, Detail: tunInterface}
}

func nftablesTableCheck(ctx context.Context, runner OutputRunner) Check {
	command := []string{"nft", "list", "table", recovery.NFTablesFamily, recovery.NFTablesTable}
	status := "active"
	if _, err := runner.Output(ctx, command[0], command[1:]...); err != nil {
		status = "missing"
	}
	return Check{Name: "nftables_table", Status: status, Detail: "inet wsl_netguard"}
}

func dependencyCheck(ctx context.Context, runner OutputRunner, dep dependency) Check {
	status := "installed"
	if _, err := runner.Output(ctx, dep.command[0], dep.command[1:]...); err != nil {
		status = "missing"
	}

	detail := "optional"
	if dep.required {
		detail = "required"
	}
	return Check{Name: dep.name, Status: status, Detail: detail}
}

func fileCheck(ctx context.Context, runner OutputRunner, name string, command []string, detail string) Check {
	status := "available"
	if _, err := runner.Output(ctx, command[0], command[1:]...); err != nil {
		status = "missing"
	}
	return Check{Name: name, Status: status, Detail: detail}
}

func curlExpectedSuccessCheck(ctx context.Context, runner OutputRunner, name string, url string) Check {
	status := "ok"
	command := curlCommand(url)
	if _, err := runner.Output(ctx, command[0], command[1:]...); err != nil {
		status = "failed"
	}
	return Check{Name: name, Status: status, Detail: url}
}

func dnsQueryCheck(ctx context.Context, runner OutputRunner) Check {
	status := "ok"
	command := append(curlCommand(""), "-H", "accept: application/dns-json", dnsTestURL)
	if _, err := runner.Output(ctx, command[0], command[1:]...); err != nil {
		status = "failed"
	}
	return Check{Name: "dns_query", Status: status, Detail: dnsTestURL}
}

func directExternalCheck(ctx context.Context, runner OutputRunner, gatewayInterface string) Check {
	status := "reachable"
	command := append(curlCommand(""), "--noproxy", "*", "--interface", gatewayInterface, proxyTestURL)
	if _, err := runner.Output(ctx, command[0], command[1:]...); err != nil {
		status = "blocked"
	}
	return Check{Name: "direct_external", Status: status, Detail: "interface " + gatewayInterface}
}

func curlCommand(url string) []string {
	command := []string{"curl", "--max-time", "8", "--silent", "--show-error", "--fail"}
	if url != "" {
		command = append(command, url)
	}
	return command
}

func firstLine(data []byte) string {
	value := strings.TrimSpace(string(data))
	if value == "" {
		return ""
	}
	line, _, _ := strings.Cut(value, "\n")
	return strings.TrimSpace(line)
}

type dependency struct {
	name     string
	required bool
	command  []string
}
