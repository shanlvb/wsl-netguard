package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/diagnostic"
	"github.com/shanlvb/wsl-netguard/internal/installfiles"
	"github.com/shanlvb/wsl-netguard/internal/netdetect"
	"github.com/shanlvb/wsl-netguard/internal/privilege"
	"github.com/shanlvb/wsl-netguard/internal/recovery"
	"github.com/shanlvb/wsl-netguard/internal/runtimefiles"
)

func (app *App) runRecoveryCommand(name string, args []string) int {
	if len(args) != 0 {
		fmt.Fprintf(app.stderr, "%s does not accept arguments\n", name)
		return ExitUsage
	}
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	actions, err := app.recoveryActions(name)
	if err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	if !app.executeRecoveryActions(actions) {
		return ExitError
	}
	return ExitOK
}

func (app *App) executeRecoveryActions(actions []recovery.Action) bool {
	results := recovery.Execute(context.Background(), app.runner, actions)
	for _, result := range results {
		if result.Err != nil {
			fmt.Fprintf(app.stderr, "failed: %v\n", result.Err)
			continue
		}
		fmt.Fprintf(app.stdout, "ok: %s\n", result.Action.Summary)
	}
	if recovery.HasFailures(results) {
		return false
	}
	return true
}

func (app *App) recoveryActions(name string) ([]recovery.Action, error) {
	if name == "rescue" {
		return recovery.RescueActions(), nil
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return recovery.DisableActions(cfg)
}

func (app *App) runEnableCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "enable does not accept arguments")
		return ExitUsage
	}
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	return app.enableNetworkInterception()
}

func (app *App) enableNetworkInterception() int {
	ctx := context.Background()
	cfg, gateway, err := app.enablePreflight(ctx)
	if err != nil {
		fmt.Fprintln(app.stderr, err)
		var unavailable *enablePreflightError
		if errors.As(err, &unavailable) {
			return ExitUnavailable
		}
		return ExitError
	}

	if err := app.writeRuntimeArtifacts(cfg, gateway, true); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}
	if err := app.applyKillSwitchRules(ctx); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}
	if err := app.runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		fmt.Fprintf(app.stderr, "reload systemd units: %v\n", err)
		return ExitError
	}
	if err := app.runner.Run(ctx, "systemctl", "enable", "--now", recovery.KillSwitchService); err != nil {
		fmt.Fprintf(app.stderr, "enable kill switch service: %v\n", err)
		return ExitError
	}
	if err := app.runner.Run(ctx, "systemctl", "enable", "--now", recovery.SingBoxService); err != nil {
		fmt.Fprintf(app.stderr, "enable sing-box service: %v\n", err)
		return ExitError
	}

	fmt.Fprintln(app.stdout, "enabled network interception")
	return ExitOK
}

type enablePreflightError struct {
	reason string
}

func (err *enablePreflightError) Error() string {
	return "preflight failed: " + err.reason
}

func (app *App) enablePreflight(ctx context.Context) (config.Config, netdetect.Gateway, error) {
	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		return config.Config{}, netdetect.Gateway{}, fmt.Errorf("load config: %w", err)
	}

	dependencyChecks := diagnostic.DependencyChecks(ctx, app.output)
	if reason := missingRequiredDependency(dependencyChecks); reason != "" {
		return config.Config{}, netdetect.Gateway{}, &enablePreflightError{reason: reason}
	}
	environmentChecks := diagnostic.EnvironmentChecks(ctx, app.output)
	if reason := missingRequiredEnvironment(environmentChecks); reason != "" {
		return config.Config{}, netdetect.Gateway{}, &enablePreflightError{reason: reason}
	}

	gateway, err := netdetect.DetectDefaultGateway(ctx, app.output, cfg.TUNInterface)
	if err != nil {
		return config.Config{}, netdetect.Gateway{}, fmt.Errorf("detect windows host IP: %w", err)
	}

	proxyCheck := diagnostic.ConfiguredProxyCheck(ctx, app.tcpProbe, gateway.Address.String(), cfg)
	if reason := unreachableConfiguredProxy(proxyCheck); reason != "" {
		return config.Config{}, netdetect.Gateway{}, &enablePreflightError{reason: reason}
	}

	return cfg, gateway, nil
}

func missingRequiredDependency(checks []diagnostic.Check) string {
	for _, check := range checks {
		if check.Detail == "required" && check.Status == "missing" {
			return fmt.Sprintf("%s is missing", check.Name)
		}
	}
	return ""
}

func missingRequiredEnvironment(checks []diagnostic.Check) string {
	for _, check := range checks {
		switch check.Name {
		case "tun_device", "dns_config":
			if check.Status == "missing" {
				return fmt.Sprintf("%s is missing (%s)", check.Name, check.Detail)
			}
		}
	}
	return ""
}

func unreachableConfiguredProxy(check diagnostic.Check) string {
	if check.Status != "reachable" {
		return fmt.Sprintf("%s is unreachable (%s)", check.Name, check.Detail)
	}
	return ""
}

func (app *App) applyKillSwitchRules(ctx context.Context) error {
	if err := app.runner.Run(ctx, "nft", "-c", "-f", runtimefiles.NFTablesCheckPath); err != nil {
		return fmt.Errorf("validate nftables kill switch: %w", err)
	}

	deleteCommand := []string{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable}
	if err := app.runner.Run(ctx, deleteCommand[0], deleteCommand[1:]...); err != nil {
		if !recovery.IsMissingManagedResourceError(deleteCommand, err) {
			return fmt.Errorf("delete existing managed nftables table: %w", err)
		}
	}

	if err := app.runner.Run(ctx, "nft", "-f", runtimefiles.NFTablesPath); err != nil {
		return fmt.Errorf("apply nftables kill switch: %w", err)
	}
	return nil
}

func (app *App) runRestartCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "restart does not accept arguments")
		return ExitUsage
	}
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	if err := app.planAndWriteRuntimeArtifacts(true); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}
	if err := app.runner.Run(context.Background(), "systemctl", "restart", recovery.KillSwitchService); err != nil {
		fmt.Fprintf(app.stderr, "restart kill switch service: %v\n", err)
		return ExitError
	}
	if err := app.runner.Run(context.Background(), "systemctl", "restart", recovery.SingBoxService); err != nil {
		fmt.Fprintf(app.stderr, "restart sing-box service: %v\n", err)
		return ExitError
	}

	fmt.Fprintln(app.stdout, "restarted network interception")
	return ExitOK
}

func (app *App) runLogsCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "logs does not accept arguments")
		return ExitUsage
	}

	if err := app.runner.Run(
		context.Background(),
		"journalctl",
		"-u", recovery.KillSwitchService,
		"-u", recovery.SingBoxService,
		"--no-pager",
		"-n", "200",
	); err != nil {
		fmt.Fprintf(app.stderr, "show logs: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func (app *App) runUninstallCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "uninstall does not accept arguments")
		return ExitUsage
	}
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	actions, err := app.recoveryActions("disable")
	if err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}
	if !app.executeRecoveryActions(actions) {
		return ExitError
	}

	for _, step := range uninstallSteps() {
		if err := app.runUninstallStep(step); err != nil {
			fmt.Fprintf(app.stderr, "failed: %v\n", err)
			return ExitError
		}
		fmt.Fprintf(app.stdout, "ok: %s\n", step.summary)
	}

	fmt.Fprintln(app.stdout, "uninstalled managed files")
	return ExitOK
}

type uninstallStep struct {
	summary string
	command []string
}

func uninstallSteps() []uninstallStep {
	return []uninstallStep{
		{
			summary: "disable sing-box service autostart",
			command: []string{"systemctl", "disable", recovery.SingBoxService},
		},
		{
			summary: "disable kill switch service autostart",
			command: []string{"systemctl", "disable", recovery.KillSwitchService},
		},
		{
			summary: "remove managed systemd unit files",
			command: []string{"rm", "-f", installfiles.SingBoxUnitPath, installfiles.KillSwitchUnitPath},
		},
		{
			summary: "remove managed install files",
			command: []string{"rm", "-f", installfiles.SingBoxTemplatePath, installfiles.ConfigPath},
		},
		{
			summary: "remove managed binary",
			command: []string{"rm", "-f", installfiles.BinaryPath},
		},
		{
			summary: "remove managed config directory",
			command: []string{"rmdir", installfiles.ConfigDir},
		},
		{
			summary: "reload systemd units",
			command: []string{"systemctl", "daemon-reload"},
		},
	}
}

func (app *App) runUninstallStep(step uninstallStep) error {
	if len(step.command) == 0 {
		return fmt.Errorf("%s: uninstall step has no command", step.summary)
	}

	err := app.runner.Run(context.Background(), step.command[0], step.command[1:]...)
	if err == nil {
		return nil
	}
	if recovery.IsMissingManagedResourceError(step.command, err) || isIgnorableConfigDirRemovalError(step.command, err) {
		return nil
	}
	return fmt.Errorf("%s: %w", step.summary, err)
}

func isIgnorableConfigDirRemovalError(command []string, err error) bool {
	if err == nil || len(command) != 2 || command[0] != "rmdir" || command[1] != installfiles.ConfigDir {
		return false
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "directory not empty")
}
