package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/command"
	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/diagnostic"
	"github.com/shanlvb/wsl-netguard/internal/installfiles"
	"github.com/shanlvb/wsl-netguard/internal/netdetect"
	"github.com/shanlvb/wsl-netguard/internal/privilege"
	"github.com/shanlvb/wsl-netguard/internal/recovery"
	"github.com/shanlvb/wsl-netguard/internal/render"
	"github.com/shanlvb/wsl-netguard/internal/runtimefiles"
	"github.com/shanlvb/wsl-netguard/internal/version"
)

const (
	ExitOK = iota
	ExitError
	ExitUsage
	ExitUnavailable
)

const strictModeWarning = "strict mode can affect IDE, debugger, and dev-server traffic"

type cliCommand struct {
	name    string
	summary string
}

var commands = []cliCommand{
	{name: "install", summary: "install managed files and systemd units"},
	{name: "enable", summary: "enable sing-box TUN and the nftables kill switch"},
	{name: "disable", summary: "stop interception and remove the managed nftables table"},
	{name: "uninstall", summary: "disable and remove managed files"},
	{name: "status", summary: "show network guard status"},
	{name: "rescue", summary: "best-effort emergency recovery"},
	{name: "mode", summary: "switch between dev and strict modes"},
	{name: "config", summary: "show or update configuration"},
	{name: "doctor", summary: "run environment diagnostics"},
	{name: "export-diagnostics", summary: "print diagnostics as JSON"},
	{name: "test", summary: "verify proxy and kill-switch behavior"},
	{name: "restart", summary: "rerender runtime config and restart services"},
	{name: "logs", summary: "show related journal logs"},
	{name: "render", summary: "render configuration without enabling services"},
	{name: "service", summary: "run internal systemd service hooks"},
	{name: "version", summary: "show build version information"},
}

var commandNames = func() map[string]struct{} {
	names := make(map[string]struct{}, len(commands))
	for _, cmd := range commands {
		names[cmd.name] = struct{}{}
	}
	return names
}()

type App struct {
	stdout         io.Writer
	stderr         io.Writer
	geteuid        func() int
	configPath     string
	loadConfig     func(string) (config.Config, error)
	saveConfig     func(string, config.Config) error
	runner         recovery.Runner
	output         netdetect.OutputRunner
	tcpProbe       diagnostic.TCPProbe
	writeArtifacts func([]runtimefiles.Artifact) error
	executablePath func() (string, error)
	now            func() time.Time
}

type Option func(*App)

func NewApp(stdout io.Writer, stderr io.Writer, opts ...Option) *App {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	execRunner := command.ExecRunner{
		Stdout: stdout,
		Stderr: stderr,
	}
	app := &App{
		stdout:         stdout,
		stderr:         stderr,
		geteuid:        os.Geteuid,
		configPath:     config.DefaultPath,
		loadConfig:     config.Load,
		saveConfig:     config.Save,
		runner:         execRunner,
		output:         execRunner,
		tcpProbe:       diagnostic.DefaultTCPProbe,
		writeArtifacts: runtimefiles.WriteArtifacts,
		executablePath: os.Executable,
		now:            time.Now,
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

func WithGeteuid(geteuid func() int) Option {
	return func(app *App) {
		app.geteuid = geteuid
	}
}

func WithRunner(runner recovery.Runner) Option {
	return func(app *App) {
		app.runner = runner
	}
}

func WithOutputRunner(output netdetect.OutputRunner) Option {
	return func(app *App) {
		app.output = output
	}
}

func WithTCPProbe(tcpProbe diagnostic.TCPProbe) Option {
	return func(app *App) {
		app.tcpProbe = tcpProbe
	}
}

func WithConfigLoader(loadConfig func(string) (config.Config, error)) Option {
	return func(app *App) {
		app.loadConfig = loadConfig
	}
}

func WithConfigSaver(saveConfig func(string, config.Config) error) Option {
	return func(app *App) {
		app.saveConfig = saveConfig
	}
}

func WithConfigPath(path string) Option {
	return func(app *App) {
		app.configPath = path
	}
}

func WithArtifactWriter(writeArtifacts func([]runtimefiles.Artifact) error) Option {
	return func(app *App) {
		app.writeArtifacts = writeArtifacts
	}
}

func WithExecutablePath(executablePath func() (string, error)) Option {
	return func(app *App) {
		app.executablePath = executablePath
	}
}

func WithNow(now func() time.Time) Option {
	return func(app *App) {
		app.now = now
	}
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return NewApp(stdout, stderr).Run(args)
}

func (app *App) Run(args []string) int {
	if len(args) == 0 || isHelpArg(args[0]) {
		printUsage(app.stdout)
		return ExitOK
	}
	if isVersionArg(args[0]) {
		return app.runVersionCommand(args[1:])
	}

	name := args[0]
	if _, ok := commandNames[name]; !ok {
		fmt.Fprintf(app.stderr, "unknown command %q\n\n", name)
		printUsage(app.stderr)
		return ExitUsage
	}

	switch name {
	case "config":
		return app.runConfigCommand(args[1:])
	case "disable", "rescue":
		return app.runRecoveryCommand(name, args[1:])
	case "doctor":
		return app.runDoctorCommand(args[1:])
	case "enable":
		return app.runEnableCommand(args[1:])
	case "export-diagnostics":
		return app.runExportDiagnosticsCommand(args[1:])
	case "install":
		return app.runInstallCommand(args[1:])
	case "logs":
		return app.runLogsCommand(args[1:])
	case "mode":
		return app.runModeCommand(args[1:])
	case "render":
		return app.runRenderCommand(args[1:])
	case "restart":
		return app.runRestartCommand(args[1:])
	case "service":
		return app.runServiceCommand(args[1:])
	case "status":
		return app.runStatusCommand(args[1:])
	case "test":
		return app.runTestCommand(args[1:])
	case "uninstall":
		return app.runUninstallCommand(args[1:])
	case "version":
		return app.runVersionCommand(args[1:])
	}

	fmt.Fprintf(app.stderr, "%s: not implemented yet\n", strings.Join(args, " "))
	return ExitUnavailable
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
}

func isVersionArg(arg string) bool {
	return arg == "-v" || arg == "--version"
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: wsl-netguard <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")

	sortedCommands := append([]cliCommand(nil), commands...)
	sort.Slice(sortedCommands, func(i, j int) bool {
		return sortedCommands[i].name < sortedCommands[j].name
	})

	for _, cmd := range sortedCommands {
		fmt.Fprintf(w, "  %-10s %s\n", cmd.name, cmd.summary)
	}
}

func (app *App) runVersionCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "version does not accept arguments")
		return ExitUsage
	}

	info := version.Current()
	fmt.Fprintf(app.stdout, "wsl-netguard version %s\n", info.Version)
	fmt.Fprintf(app.stdout, "commit: %s\n", info.Commit)
	fmt.Fprintf(app.stdout, "built: %s\n", info.Date)
	return ExitOK
}

func (app *App) runStatusCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "status does not accept arguments")
		return ExitUsage
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	gateway, err := netdetect.DetectDefaultGateway(context.Background(), app.output, cfg.TUNInterface)
	if err != nil {
		fmt.Fprintf(app.stderr, "detect windows host IP: %v\n", err)
		return ExitError
	}

	fmt.Fprintf(app.stdout, "mode: %s\n", cfg.Mode)
	fmt.Fprintf(app.stdout, "windows_host_ip: %s\n", gateway.Address)
	fmt.Fprintf(app.stdout, "gateway_interface: %s\n", gateway.Interface)
	fmt.Fprintf(app.stdout, "tun_interface: %s\n", cfg.TUNInterface)
	app.printChecks(diagnostic.ProxyPortChecks(context.Background(), app.tcpProbe, gateway.Address.String(), cfg))
	app.printChecks(diagnostic.RuntimeChecks(context.Background(), app.output, cfg))
	return ExitOK
}

func (app *App) printChecks(checks []diagnostic.Check) {
	for _, check := range checks {
		if check.Detail == "" {
			fmt.Fprintf(app.stdout, "%s: %s\n", check.Name, check.Status)
			continue
		}
		fmt.Fprintf(app.stdout, "%s: %s (%s)\n", check.Name, check.Status, check.Detail)
	}
}

func (app *App) runDoctorCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "doctor does not accept arguments")
		return ExitUsage
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	ctx := context.Background()
	dependencyChecks := diagnostic.DependencyChecks(ctx, app.output)
	app.printChecks(dependencyChecks)
	environmentChecks := diagnostic.EnvironmentChecks(ctx, app.output)
	app.printChecks(environmentChecks)

	gateway, err := netdetect.DetectDefaultGateway(ctx, app.output, cfg.TUNInterface)
	if err != nil {
		fmt.Fprintf(app.stdout, "windows_host_ip: error (%v)\n", err)
		return ExitUnavailable
	}

	fmt.Fprintf(app.stdout, "mode: %s\n", cfg.Mode)
	fmt.Fprintf(app.stdout, "windows_host_ip: %s\n", gateway.Address)
	fmt.Fprintf(app.stdout, "gateway_interface: %s\n", gateway.Interface)
	fmt.Fprintf(app.stdout, "tun_interface: %s\n", cfg.TUNInterface)

	proxyChecks := diagnostic.ProxyPortChecks(ctx, app.tcpProbe, gateway.Address.String(), cfg)
	runtimeChecks := diagnostic.RuntimeChecks(ctx, app.output, cfg)
	app.printChecks(proxyChecks)
	app.printChecks(runtimeChecks)

	if doctorHasFailures(cfg, dependencyChecks, environmentChecks, proxyChecks) {
		return ExitUnavailable
	}
	return ExitOK
}

func doctorHasFailures(cfg config.Config, dependencyChecks []diagnostic.Check, environmentChecks []diagnostic.Check, proxyChecks []diagnostic.Check) bool {
	for _, check := range dependencyChecks {
		if check.Detail == "required" && check.Status == "missing" {
			return true
		}
	}
	if hasMissingRequiredEnvironment(environmentChecks) {
		return true
	}

	requiredProxyCheck := "v2rayn_socks"
	if cfg.ProxyProtocol == config.ProxyProtocolHTTP {
		requiredProxyCheck = "v2rayn_http"
	}
	for _, check := range proxyChecks {
		if check.Name == requiredProxyCheck && check.Status != "reachable" {
			return true
		}
	}
	return false
}

func (app *App) runTestCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "test does not accept arguments")
		return ExitUsage
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	ctx := context.Background()
	gateway, err := netdetect.DetectDefaultGateway(ctx, app.output, cfg.TUNInterface)
	if err != nil {
		fmt.Fprintf(app.stderr, "detect windows host IP: %v\n", err)
		return ExitError
	}

	fmt.Fprintf(app.stdout, "mode: %s\n", cfg.Mode)
	fmt.Fprintf(app.stdout, "windows_host_ip: %s\n", gateway.Address)
	fmt.Fprintf(app.stdout, "gateway_interface: %s\n", gateway.Interface)
	fmt.Fprintf(app.stdout, "tun_interface: %s\n", cfg.TUNInterface)

	proxyChecks := diagnostic.ProxyPortChecks(ctx, app.tcpProbe, gateway.Address.String(), cfg)
	networkChecks := diagnostic.NetworkTestChecks(ctx, app.output, gateway.Interface)
	app.printChecks(proxyChecks)
	app.printChecks(networkChecks)

	if doctorHasFailures(cfg, nil, nil, proxyChecks) || networkTestHasFailures(networkChecks) {
		return ExitUnavailable
	}
	return ExitOK
}

func hasMissingRequiredEnvironment(checks []diagnostic.Check) bool {
	for _, check := range checks {
		switch check.Name {
		case "tun_device", "dns_config":
			if check.Status == "missing" {
				return true
			}
		}
	}
	return false
}

func networkTestHasFailures(checks []diagnostic.Check) bool {
	for _, check := range checks {
		switch check.Name {
		case "proxy_external", "dns_query":
			if check.Status != "ok" {
				return true
			}
		case "direct_external":
			if check.Status != "blocked" {
				return true
			}
		}
	}
	return false
}

func (app *App) runRenderCommand(args []string) int {
	if len(args) != 0 {
		fmt.Fprintln(app.stderr, "render does not accept arguments")
		return ExitUsage
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	gateway, err := netdetect.DetectDefaultGateway(context.Background(), app.output, cfg.TUNInterface)
	if err != nil {
		fmt.Fprintf(app.stderr, "detect windows host IP: %v\n", err)
		return ExitError
	}

	singBoxConfig, err := render.SingBoxJSON(cfg, gateway.Address)
	if err != nil {
		fmt.Fprintf(app.stderr, "render sing-box config: %v\n", err)
		return ExitError
	}
	nftablesRules, err := render.NFTablesRules(cfg, gateway.Address)
	if err != nil {
		fmt.Fprintf(app.stderr, "render nftables rules: %v\n", err)
		return ExitError
	}

	fmt.Fprintln(app.stdout, "sing-box.json:")
	fmt.Fprint(app.stdout, string(singBoxConfig))
	fmt.Fprintln(app.stdout)
	fmt.Fprintln(app.stdout, "nftables.nft:")
	fmt.Fprint(app.stdout, nftablesRules)
	return ExitOK
}

func (app *App) runConfigCommand(args []string) int {
	if len(args) == 1 && args[0] == "show" {
		return app.runConfigShow()
	}
	if len(args) == 3 && args[0] == "set" {
		return app.runConfigSet(args[1], args[2])
	}

	fmt.Fprintln(app.stderr, "usage: wsl-netguard config show")
	fmt.Fprintln(app.stderr, "usage: wsl-netguard config set <mode|proxy-protocol|socks-port|http-port|tun-interface> <value>")
	return ExitUsage
}

func (app *App) runConfigShow() int {
	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(app.stderr, "render config: %v\n", err)
		return ExitError
	}
	fmt.Fprintln(app.stdout, string(data))
	return ExitOK
}

func (app *App) runConfigSet(key string, value string) int {
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	updatedConfig, err := updateConfigValue(cfg, key, value)
	if err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitUsage
	}

	if err := app.saveConfig(app.configPath, updatedConfig); err != nil {
		fmt.Fprintf(app.stderr, "save config: %v\n", err)
		return ExitError
	}

	if key == "mode" {
		app.warnIfStrictMode(updatedConfig.Mode)
	}
	fmt.Fprintf(app.stdout, "updated %s\n", key)
	return ExitOK
}

func updateConfigValue(cfg config.Config, key string, value string) (config.Config, error) {
	updatedConfig := cfg
	switch key {
	case "mode":
		updatedConfig.Mode = value
	case "proxy-protocol":
		updatedConfig.ProxyProtocol = value
	case "socks-port":
		port, err := parseConfigPort(key, value)
		if err != nil {
			return config.Config{}, err
		}
		updatedConfig.SocksPort = port
	case "http-port":
		port, err := parseConfigPort(key, value)
		if err != nil {
			return config.Config{}, err
		}
		updatedConfig.HTTPPort = port
	case "tun-interface":
		updatedConfig.TUNInterface = value
	default:
		return config.Config{}, fmt.Errorf("unsupported config key %q", key)
	}

	if err := updatedConfig.Validate(); err != nil {
		return config.Config{}, err
	}
	return updatedConfig, nil
}

func parseConfigPort(key string, value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number: %w", key, err)
	}
	return port, nil
}

func (app *App) runModeCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(app.stderr, "usage: wsl-netguard mode <dev|strict>")
		return ExitUsage
	}
	mode := args[0]

	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}

	updatedConfig, err := updateConfigValue(cfg, "mode", mode)
	if err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitUsage
	}
	if err := app.saveConfig(app.configPath, updatedConfig); err != nil {
		fmt.Fprintf(app.stderr, "save config: %v\n", err)
		return ExitError
	}

	app.warnIfStrictMode(mode)
	fmt.Fprintf(app.stdout, "mode: %s\n", mode)
	return ExitOK
}

func (app *App) warnIfStrictMode(mode string) {
	if mode == config.ModeStrict {
		fmt.Fprintln(app.stderr, strictModeWarning)
	}
}

func (app *App) runInstallCommand(args []string) int {
	enableAfterInstall := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--enable":
		enableAfterInstall = true
	default:
		fmt.Fprintln(app.stderr, "usage: wsl-netguard install [--enable]")
		return ExitUsage
	}
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		fmt.Fprintf(app.stderr, "load config: %v\n", err)
		return ExitError
	}
	artifacts, err := installfiles.PlanArtifacts(cfg)
	if err != nil {
		fmt.Fprintf(app.stderr, "plan install artifacts: %v\n", err)
		return ExitError
	}
	binaryArtifact, err := app.planBinaryArtifact()
	if err != nil {
		fmt.Fprintf(app.stderr, "plan binary install artifact: %v\n", err)
		return ExitError
	}
	artifacts = append([]runtimefiles.Artifact{binaryArtifact}, artifacts...)
	if err := app.writeArtifacts(artifacts); err != nil {
		fmt.Fprintf(app.stderr, "write install artifacts: %v\n", err)
		return ExitError
	}

	fmt.Fprintln(app.stdout, "installed managed files")
	if enableAfterInstall {
		return app.enableNetworkInterception()
	}
	fmt.Fprintln(app.stdout, "network interception is not enabled")
	return ExitOK
}

func (app *App) planBinaryArtifact() (runtimefiles.Artifact, error) {
	sourcePath, err := app.executablePath()
	if err != nil {
		return runtimefiles.Artifact{}, fmt.Errorf("locate current executable: %w", err)
	}

	// #nosec G304 -- sourcePath comes from os.Executable in production; tests inject temp executables.
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return runtimefiles.Artifact{}, fmt.Errorf("read current executable %s: %w", sourcePath, err)
	}
	if len(data) == 0 {
		return runtimefiles.Artifact{}, fmt.Errorf("current executable %s is empty", sourcePath)
	}

	return runtimefiles.Artifact{
		Path: installfiles.BinaryPath,
		Data: data,
		Mode: 0o755,
	}, nil
}

func (app *App) runServiceCommand(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(app.stderr, "usage: wsl-netguard service <render-sing-box|apply-killswitch>")
		return ExitUsage
	}
	switch args[0] {
	case "render-sing-box":
		return app.runServiceRenderSingBox()
	case "apply-killswitch":
		return app.runServiceApplyKillSwitch()
	default:
		fmt.Fprintln(app.stderr, "usage: wsl-netguard service <render-sing-box|apply-killswitch>")
		return ExitUsage
	}
}

func (app *App) runServiceRenderSingBox() int {
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	if err := app.planAndWriteRuntimeArtifacts(true); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	fmt.Fprintln(app.stdout, "rendered runtime artifacts")
	return ExitOK
}

func (app *App) runServiceApplyKillSwitch() int {
	if err := privilege.RequireRoot(app.geteuid()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	if err := app.planAndWriteRuntimeArtifacts(false); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}
	if err := app.applyKillSwitchRules(context.Background()); err != nil {
		fmt.Fprintln(app.stderr, err)
		return ExitError
	}

	fmt.Fprintln(app.stdout, "applied kill switch")
	return ExitOK
}

func (app *App) planAndWriteRuntimeArtifacts(validateSingBox bool) error {
	cfg, err := app.loadConfig(app.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	gateway, err := netdetect.DetectDefaultGateway(context.Background(), app.output, cfg.TUNInterface)
	if err != nil {
		return fmt.Errorf("detect windows host IP: %w", err)
	}

	return app.writeRuntimeArtifacts(cfg, gateway, validateSingBox)
}

func (app *App) writeRuntimeArtifacts(cfg config.Config, gateway netdetect.Gateway, validateSingBox bool) error {
	artifacts, err := runtimefiles.PlanArtifacts(cfg, gateway, app.now())
	if err != nil {
		return fmt.Errorf("plan runtime artifacts: %w", err)
	}
	if err := app.writeArtifacts(artifacts); err != nil {
		return fmt.Errorf("write runtime artifacts: %w", err)
	}
	if validateSingBox {
		if err := app.validateSingBoxConfig(context.Background()); err != nil {
			return err
		}
	}
	return nil
}

func (app *App) validateSingBoxConfig(ctx context.Context) error {
	if err := app.runner.Run(ctx, "sing-box", "check", "-c", runtimefiles.SingBoxPath); err != nil {
		return fmt.Errorf("validate sing-box config: %w", err)
	}
	return nil
}
