package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/diagnostic"
	"github.com/shanlvb/wsl-netguard/internal/netdetect"
)

type diagnosticsExport struct {
	GeneratedAt time.Time                `json:"generated_at"`
	Config      config.Config            `json:"config"`
	Gateway     diagnosticsGatewayExport `json:"gateway"`
	Checks      diagnosticsChecksExport  `json:"checks"`
}

type diagnosticsGatewayExport struct {
	WindowsHostIP    string `json:"windows_host_ip,omitempty"`
	GatewayInterface string `json:"gateway_interface,omitempty"`
	Error            string `json:"error,omitempty"`
}

type diagnosticsChecksExport struct {
	Dependencies []diagnostic.Check `json:"dependencies"`
	Environment  []diagnostic.Check `json:"environment"`
	ProxyPorts   []diagnostic.Check `json:"proxy_ports,omitempty"`
	Runtime      []diagnostic.Check `json:"runtime"`
}

func (app *App) runExportDiagnosticsCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "export-diagnostics does not accept arguments")
		return ExitUsage
	}

	doc, unavailable, err := app.collectDiagnosticsExport(context.Background())
	if err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(app.stderr, "render diagnostics export: %v\n", err)
		return ExitError
	}
	fmt.Fprintln(app.stdout, string(data))

	if unavailable {
		return ExitUnavailable
	}
	return ExitOK
}

func (app *App) collectDiagnosticsExport(ctx context.Context) (diagnosticsExport, bool, error) {
	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		return diagnosticsExport{}, false, fmt.Errorf("load config: %w", err)
	}

	doc := diagnosticsExport{
		GeneratedAt: app.now(),
		Config:      cfg,
		Checks: diagnosticsChecksExport{
			Dependencies: diagnostic.DependencyChecks(ctx, app.output),
			Environment:  diagnostic.EnvironmentChecks(ctx, app.output),
		},
	}

	gateway, err := netdetect.DetectDefaultGateway(ctx, app.output, cfg.TUNInterface)
	if err != nil {
		doc.Gateway.Error = err.Error()
		doc.Checks.Runtime = diagnostic.RuntimeChecks(ctx, app.output, cfg)
		return doc, true, nil
	}

	doc.Gateway = diagnosticsGatewayExport{
		WindowsHostIP:    gateway.Address.String(),
		GatewayInterface: gateway.Interface,
	}
	doc.Checks.ProxyPorts = diagnostic.ProxyPortChecks(ctx, app.tcpProbe, gateway.Address.String(), cfg)
	doc.Checks.Runtime = diagnostic.RuntimeChecks(ctx, app.output, cfg)

	return doc, doctorHasFailures(cfg, doc.Checks.Dependencies, doc.Checks.Environment, doc.Checks.ProxyPorts), nil
}
