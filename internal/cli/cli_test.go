package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shanlvb/wsl-netguard/internal/config"
	"github.com/shanlvb/wsl-netguard/internal/diagnostic"
	"github.com/shanlvb/wsl-netguard/internal/installfiles"
	"github.com/shanlvb/wsl-netguard/internal/recovery"
	"github.com/shanlvb/wsl-netguard/internal/runtimefiles"
)

func TestRunShowsHelpForNoArgs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run(nil, &stdout, &stderr)

	if code != ExitOK {
		t.Fatalf("Run(nil) exit code = %d; want %d", code, ExitOK)
	}
	if !strings.Contains(stdout.String(), "Usage: wsl-netguard <command>") {
		t.Fatalf("stdout does not contain usage:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestRunShowsVersionForVersionFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "version command", args: []string{"version"}},
		{name: "version flag", args: []string{"--version"}},
		{name: "short version flag", args: []string{"-v"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := Run(tt.args, &stdout, &stderr)

			if code != ExitOK {
				t.Fatalf("Run(%v) exit code = %d; want %d", tt.args, code, ExitOK)
			}
			for _, want := range []string{
				"wsl-netguard version",
				"commit:",
				"built:",
			} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("version output missing %q:\n%s", want, stdout.String())
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q; want empty", stderr.String())
			}
		})
	}
}

func TestVersionRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"version", "--json"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("version with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "version does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestRunShowsHelpForHelpFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "help command", args: []string{"help"}},
		{name: "short help flag", args: []string{"-h"}},
		{name: "long help flag", args: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			code := Run(tt.args, &stdout, &stderr)

			if code != ExitOK {
				t.Fatalf("Run(%v) exit code = %d; want %d", tt.args, code, ExitOK)
			}
			for _, command := range []string{"install", "enable", "disable", "uninstall", "status", "rescue", "version"} {
				if !strings.Contains(stdout.String(), command) {
					t.Fatalf("help output missing command %q:\n%s", command, stdout.String())
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q; want empty", stderr.String())
			}
		})
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := Run([]string{"unknown"}, &stdout, &stderr)

	if code != ExitUsage {
		t.Fatalf("Run unknown exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unknown command "unknown"`) {
		t.Fatalf("stderr missing unknown command message:\n%s", stderr.String())
	}
}

func TestInstallRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"install"})

	if code != ExitError {
		t.Fatalf("install exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if writeCalled {
		t.Fatal("artifact writer was called without root")
	}
}

func TestInstallWritesManagedFilesWithoutEnabling(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var writtenArtifacts []runtimefiles.Artifact
	runner := &recordingRunner{}
	executablePath := writeTestExecutable(t, []byte("wsl-netguard-binary"))
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithExecutablePath(func() (string, error) {
			return executablePath, nil
		}),
		WithArtifactWriter(func(artifacts []runtimefiles.Artifact) error {
			writtenArtifacts = append([]runtimefiles.Artifact(nil), artifacts...)
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"install"})

	if code != ExitOK {
		t.Fatalf("install exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if len(writtenArtifacts) != 5 {
		t.Fatalf("written artifact length = %d; want 5", len(writtenArtifacts))
	}
	if writtenArtifacts[0].Path != installfiles.BinaryPath {
		t.Fatalf("binary artifact path = %q; want %q", writtenArtifacts[0].Path, installfiles.BinaryPath)
	}
	if string(writtenArtifacts[0].Data) != "wsl-netguard-binary" {
		t.Fatalf("binary artifact data = %q; want executable bytes", string(writtenArtifacts[0].Data))
	}
	if writtenArtifacts[0].Mode != 0o755 {
		t.Fatalf("binary artifact mode = %#o; want 0755", writtenArtifacts[0].Mode)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if !strings.Contains(stdout.String(), "installed managed files") {
		t.Fatalf("stdout missing install message:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "network interception is not enabled") {
		t.Fatalf("stdout missing no-enable message:\n%s", stdout.String())
	}
}

func TestInstallRejectsUnsupportedArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"install", "--force"})

	if code != ExitUsage {
		t.Fatalf("install with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: wsl-netguard install [--enable]") {
		t.Fatalf("stderr missing install usage:\n%s", stderr.String())
	}
}

func TestInstallReportsExecutableLocationError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithExecutablePath(func() (string, error) {
			return "", errors.New("missing executable path")
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"install"})

	if code != ExitError {
		t.Fatalf("install exit code = %d; want %d", code, ExitError)
	}
	if writeCalled {
		t.Fatal("artifact writer was called after executable location failed")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "plan binary install artifact") {
		t.Fatalf("stderr missing binary planning context:\n%s", stderr.String())
	}
}

func TestInstallWithEnableInstallsThenEnables(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var artifactBatchLengths []int
	runner := &recordingRunner{}
	executablePath := writeTestExecutable(t, []byte("wsl-netguard-binary"))
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithExecutablePath(func() (string, error) {
			return executablePath, nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func(artifacts []runtimefiles.Artifact) error {
			artifactBatchLengths = append(artifactBatchLengths, len(artifacts))
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"install", "--enable"})

	if code != ExitOK {
		t.Fatalf("install --enable exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantBatchLengths := []int{5, 4}
	if !reflect.DeepEqual(artifactBatchLengths, wantBatchLengths) {
		t.Fatalf("artifact batch lengths = %+v; want %+v", artifactBatchLengths, wantBatchLengths)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"nft", "-f", runtimefiles.NFTablesPath},
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", recovery.KillSwitchService},
		{"systemctl", "enable", "--now", recovery.SingBoxService},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	for _, want := range []string{
		"installed managed files",
		"enabled network interception",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "network interception is not enabled") {
		t.Fatalf("stdout should not include no-enable message when --enable is set:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestEnableRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	writeCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithRunner(runner),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"enable"})

	if code != ExitError {
		t.Fatalf("enable exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if writeCalled {
		t.Fatal("artifact writer was called without root")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
}

func TestEnableWritesRuntimeArtifactsAndStartsServicesInSafeOrder(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var writtenArtifacts []runtimefiles.Artifact
	runner := &recordingRunner{}
	renderedAt := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithNow(func() time.Time { return renderedAt }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func(artifacts []runtimefiles.Artifact) error {
			writtenArtifacts = append([]runtimefiles.Artifact(nil), artifacts...)
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitOK {
		t.Fatalf("enable exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if len(writtenArtifacts) != 4 {
		t.Fatalf("written artifact length = %d; want 4", len(writtenArtifacts))
	}
	if !strings.Contains(string(writtenArtifacts[0].Data), `"server": "172.25.80.1"`) {
		t.Fatalf("sing-box artifact missing gateway:\n%s", string(writtenArtifacts[0].Data))
	}
	if !strings.Contains(string(writtenArtifacts[3].Data), `"gateway_interface": "eth0"`) {
		t.Fatalf("state artifact missing gateway interface:\n%s", string(writtenArtifacts[3].Data))
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"nft", "-f", runtimefiles.NFTablesPath},
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", recovery.KillSwitchService},
		{"systemctl", "enable", "--now", recovery.SingBoxService},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stdout.String(), "enabled network interception") {
		t.Fatalf("stdout missing success message:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestEnableRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"enable", "--force"})

	if code != ExitUsage {
		t.Fatalf("enable with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "enable does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestEnableStopsBeforeSystemdWhenKillSwitchApplyFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{3: errors.New("nft failed")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitError {
		t.Fatalf("enable exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"nft", "-f", runtimefiles.NFTablesPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "apply nftables kill switch") {
		t.Fatalf("stderr missing nft context:\n%s", stderr.String())
	}
}

func TestEnableStopsBeforeKillSwitchWhenSingBoxValidationFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("invalid sing-box config")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitError {
		t.Fatalf("enable exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "validate sing-box config") {
		t.Fatalf("stderr missing sing-box validation context:\n%s", stderr.String())
	}
}

func TestEnableStopsBeforeDeletingManagedTableWhenKillSwitchValidationFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{1: errors.New("invalid nftables syntax")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitError {
		t.Fatalf("enable exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "validate nftables kill switch") {
		t.Fatalf("stderr missing validation context:\n%s", stderr.String())
	}
}

func TestEnableIgnoresMissingManagedTableBeforeApplyingKillSwitch(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{
		2: errors.New("Error: Could not process rule: No such file or directory"),
	}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitOK {
		t.Fatalf("enable exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"nft", "-f", runtimefiles.NFTablesPath},
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", recovery.KillSwitchService},
		{"systemctl", "enable", "--now", recovery.SingBoxService},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestEnableStopsBeforeWritingArtifactsWhenRequiredDependencyIsMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCalled := false
	runner := &recordingRunner{}
	outputRunner := doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")
	outputRunner.errorsByCommand[cliCommandKey("nft", "--version")] = errors.New("missing nft")
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(outputRunner),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitUnavailable {
		t.Fatalf("enable exit code = %d; want %d", code, ExitUnavailable)
	}
	if writeCalled {
		t.Fatal("artifact writer was called after preflight failure")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "preflight failed: dependency_nft is missing") {
		t.Fatalf("stderr missing dependency preflight failure:\n%s", stderr.String())
	}
}

func TestEnableStopsBeforeWritingArtifactsWhenConfiguredProxyIsUnreachable(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCalled := false
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")),
		WithTCPProbe(func(_ context.Context, address string) error {
			if strings.HasSuffix(address, ":10808") {
				return errors.New("connection refused")
			}
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitUnavailable {
		t.Fatalf("enable exit code = %d; want %d", code, ExitUnavailable)
	}
	if writeCalled {
		t.Fatal("artifact writer was called after preflight failure")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "preflight failed: v2rayn_socks is unreachable") {
		t.Fatalf("stderr missing proxy preflight failure:\n%s", stderr.String())
	}
}

func TestEnablePreflightProbesOnlyConfiguredProxyProtocol(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var probed []string
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			cfg := config.Default()
			cfg.HTTPPort = 20809
			return cfg, nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithTCPProbe(func(_ context.Context, address string) error {
			probed = append(probed, address)
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(&recordingRunner{}),
	)

	code := app.Run([]string{"enable"})

	if code != ExitOK {
		t.Fatalf("enable exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantProbed := []string{"172.25.80.1:10808"}
	if !reflect.DeepEqual(probed, wantProbed) {
		t.Fatalf("probed addresses = %+v; want %+v", probed, wantProbed)
	}
}

func TestEnableStopsBeforeWritingArtifactsWhenTUNDeviceIsMissing(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCalled := false
	runner := &recordingRunner{}
	outputRunner := doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")
	outputRunner.errorsByCommand[cliCommandKey("test", "-c", "/dev/net/tun")] = errors.New("missing tun")
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"enable"})

	if code != ExitUnavailable {
		t.Fatalf("enable exit code = %d; want %d", code, ExitUnavailable)
	}
	if writeCalled {
		t.Fatal("artifact writer was called after preflight failure")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if !strings.Contains(stderr.String(), "preflight failed: tun_device is missing") {
		t.Fatalf("stderr missing TUN preflight failure:\n%s", stderr.String())
	}
}

func TestRestartRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	writeCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithRunner(runner),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"restart"})

	if code != ExitError {
		t.Fatalf("restart exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if writeCalled {
		t.Fatal("artifact writer was called without root")
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
}

func TestRestartRendersRuntimeArtifactsAndRestartsInSafeOrder(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var writtenArtifacts []runtimefiles.Artifact
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func(artifacts []runtimefiles.Artifact) error {
			writtenArtifacts = append([]runtimefiles.Artifact(nil), artifacts...)
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"restart"})

	if code != ExitOK {
		t.Fatalf("restart exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if len(writtenArtifacts) != 4 {
		t.Fatalf("written artifact length = %d; want 4", len(writtenArtifacts))
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"systemctl", "restart", recovery.KillSwitchService},
		{"systemctl", "restart", recovery.SingBoxService},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stdout.String(), "restarted network interception") {
		t.Fatalf("stdout missing restart message:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestRestartStopsBeforeServicesWhenSingBoxValidationFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("invalid sing-box config")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"restart"})

	if code != ExitError {
		t.Fatalf("restart exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "validate sing-box config") {
		t.Fatalf("stderr missing sing-box validation context:\n%s", stderr.String())
	}
}

func TestRestartRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"restart", "--force"})

	if code != ExitUsage {
		t.Fatalf("restart with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "restart does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestRestartStopsBeforeSingBoxWhenKillSwitchRestartFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{1: errors.New("systemd failed")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"restart"})

	if code != ExitError {
		t.Fatalf("restart exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
		{"systemctl", "restart", recovery.KillSwitchService},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stderr.String(), "restart kill switch service") {
		t.Fatalf("stderr missing kill switch context:\n%s", stderr.String())
	}
}

func TestLogsRunsJournalctlForManagedUnits(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithRunner(runner),
	)

	code := app.Run([]string{"logs"})

	if code != ExitOK {
		t.Fatalf("logs exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantCommands := [][]string{
		{"journalctl", "-u", recovery.KillSwitchService, "-u", recovery.SingBoxService, "--no-pager", "-n", "200"},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
}

func TestLogsRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"logs", "--follow"})

	if code != ExitUsage {
		t.Fatalf("logs with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "logs does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestLogsReportsJournalctlError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("journal unavailable")}}
	app := NewApp(&stdout, &stderr,
		WithRunner(runner),
	)

	code := app.Run([]string{"logs"})

	if code != ExitError {
		t.Fatalf("logs exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "show logs") {
		t.Fatalf("stderr missing log context:\n%s", stderr.String())
	}
}

func TestUninstallRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithRunner(runner),
	)

	code := app.Run([]string{"uninstall"})

	if code != ExitError {
		t.Fatalf("uninstall exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
}

func TestUninstallRunsRecoveryBeforeRemovingManagedInstallFiles(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	cfg := config.Default()
	cfg.TUNInterface = "custom-tun"
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
	)

	code := app.Run([]string{"uninstall"})

	if code != ExitOK {
		t.Fatalf("uninstall exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantCommands := [][]string{
		{"systemctl", "stop", recovery.SingBoxService},
		{"systemctl", "stop", recovery.KillSwitchService},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"ip", "link", "delete", "custom-tun"},
		{"rm", "-rf", recovery.RuntimeDir},
		{"systemctl", "disable", recovery.SingBoxService},
		{"systemctl", "disable", recovery.KillSwitchService},
		{"rm", "-f", installfiles.SingBoxUnitPath, installfiles.KillSwitchUnitPath},
		{"rm", "-f", installfiles.SingBoxTemplatePath, installfiles.ConfigPath},
		{"rm", "-f", installfiles.BinaryPath},
		{"rmdir", installfiles.ConfigDir},
		{"systemctl", "daemon-reload"},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	for _, want := range []string{
		"ok: stop sing-box service",
		"ok: disable sing-box service autostart",
		"ok: remove managed install files",
		"ok: remove managed binary",
		"ok: reload systemd units",
		"uninstalled managed files",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestUninstallRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"uninstall", "--keep-config"})

	if code != ExitUsage {
		t.Fatalf("uninstall with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "uninstall does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestUninstallIgnoresMissingManagedResourcesAndFiles(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{
		0:  errors.New("Unit wsl-netguard-sing-box.service not loaded."),
		1:  errors.New("Unit wsl-netguard-killswitch.service could not be found."),
		2:  errors.New("Error: Could not process rule: No such file or directory"),
		3:  errors.New("Cannot find device \"wng-tun\""),
		5:  errors.New("Unit file wsl-netguard-sing-box.service does not exist."),
		6:  errors.New("Unit file wsl-netguard-killswitch.service does not exist."),
		10: errors.New("rmdir: failed to remove '/etc/wsl-netguard': No such file or directory"),
	}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
	)

	code := app.Run([]string{"uninstall"})

	if code != ExitOK {
		t.Fatalf("uninstall exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "uninstalled managed files") {
		t.Fatalf("stdout missing final success message:\n%s", stdout.String())
	}
}

func TestUninstallStopsBeforeRemovingFilesWhenRecoveryFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("permission denied")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
	)

	code := app.Run([]string{"uninstall"})

	if code != ExitError {
		t.Fatalf("uninstall exit code = %d; want %d", code, ExitError)
	}
	disableActions, err := recovery.DisableActions(config.Default())
	if err != nil {
		t.Fatalf("plan disable actions: %v", err)
	}
	if len(runner.commands) != len(disableActions) {
		t.Fatalf("runner commands length = %d; want only recovery actions", len(runner.commands))
	}
	if !strings.Contains(stderr.String(), "failed: stop sing-box service") {
		t.Fatalf("stderr missing recovery failure:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "uninstalled managed files") {
		t.Fatalf("stdout should not report uninstall success:\n%s", stdout.String())
	}
}

func TestModeRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	saveCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithConfigSaver(func(string, config.Config) error {
			saveCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"mode", "dev"})

	if code != ExitError {
		t.Fatalf("mode exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if saveCalled {
		t.Fatal("save was called without root")
	}
}

func TestModeUpdatesConfig(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantOutput string
		wantErr    string
	}{
		{name: "dev", mode: config.ModeDev, wantOutput: "mode: dev"},
		{name: "strict", mode: config.ModeStrict, wantOutput: "mode: strict", wantErr: "strict mode can affect IDE, debugger, and dev-server traffic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			var savedConfig config.Config
			baseConfig := config.Default()
			app := NewApp(&stdout, &stderr,
				WithGeteuid(func() int { return 0 }),
				WithConfigLoader(func(string) (config.Config, error) {
					return baseConfig, nil
				}),
				WithConfigSaver(func(_ string, cfg config.Config) error {
					savedConfig = cfg
					return nil
				}),
			)

			code := app.Run([]string{"mode", tt.mode})

			if code != ExitOK {
				t.Fatalf("mode exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
			}
			if savedConfig.Mode != tt.mode {
				t.Fatalf("saved mode = %q; want %q", savedConfig.Mode, tt.mode)
			}
			if !strings.Contains(stdout.String(), tt.wantOutput) {
				t.Fatalf("stdout missing %q:\n%s", tt.wantOutput, stdout.String())
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Fatalf("stderr missing warning %q:\n%s", tt.wantErr, stderr.String())
			}
			if tt.wantErr == "" && stderr.Len() != 0 {
				t.Fatalf("stderr = %q; want empty", stderr.String())
			}
		})
	}
}

func TestModeRejectsUnsupportedMode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	saveCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithConfigSaver(func(string, config.Config) error {
			saveCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"mode", "prod"})

	if code != ExitUsage {
		t.Fatalf("mode exit code = %d; want %d", code, ExitUsage)
	}
	if saveCalled {
		t.Fatal("save was called for invalid mode")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "mode") {
		t.Fatalf("stderr = %q; want mode validation error", stderr.String())
	}
}

func TestConfigShowPrintsEffectiveConfigJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cfg := config.Default()
	cfg.Mode = config.ModeStrict
	cfg.SocksPort = 20808
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
	)

	code := app.Run([]string{"config", "show"})

	if code != ExitOK {
		t.Fatalf("config show exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	for _, want := range []string{
		`"mode": "strict"`,
		`"socks_port": 20808`,
		`"tun_interface": "wng-tun"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("config show output missing %q:\n%s", want, stdout.String())
		}
	}
	if !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("config show output should end with newline:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestConfigRejectsUnsupportedArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"config", "get", "mode"})

	if code != ExitUsage {
		t.Fatalf("config set exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: wsl-netguard config show") {
		t.Fatalf("stderr missing config usage:\n%s", stderr.String())
	}
}

func TestConfigSetRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	loadCalled := false
	saveCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithConfigLoader(func(string) (config.Config, error) {
			loadCalled = true
			return config.Default(), nil
		}),
		WithConfigSaver(func(string, config.Config) error {
			saveCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"config", "set", "mode", "strict"})

	if code != ExitError {
		t.Fatalf("config set exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if loadCalled || saveCalled {
		t.Fatalf("loadCalled/saveCalled = %v/%v; want false/false", loadCalled, saveCalled)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
}

func TestConfigSetUpdatesDocumentedFields(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    func(config.Config) config.Config
		wantErr string
	}{
		{
			name:    "mode",
			args:    []string{"mode", "strict"},
			wantErr: "strict mode can affect IDE, debugger, and dev-server traffic",
			want: func(cfg config.Config) config.Config {
				cfg.Mode = config.ModeStrict
				return cfg
			},
		},
		{
			name: "proxy protocol",
			args: []string{"proxy-protocol", "http"},
			want: func(cfg config.Config) config.Config {
				cfg.ProxyProtocol = config.ProxyProtocolHTTP
				return cfg
			},
		},
		{
			name: "socks port",
			args: []string{"socks-port", "20808"},
			want: func(cfg config.Config) config.Config {
				cfg.SocksPort = 20808
				return cfg
			},
		},
		{
			name: "http port",
			args: []string{"http-port", "20809"},
			want: func(cfg config.Config) config.Config {
				cfg.HTTPPort = 20809
				return cfg
			},
		},
		{
			name: "tun interface",
			args: []string{"tun-interface", "custom-tun"},
			want: func(cfg config.Config) config.Config {
				cfg.TUNInterface = "custom-tun"
				return cfg
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			var savedPath string
			var savedConfig config.Config
			baseConfig := config.Default()
			app := NewApp(&stdout, &stderr,
				WithGeteuid(func() int { return 0 }),
				WithConfigPath("/tmp/test-config.json"),
				WithConfigLoader(func(path string) (config.Config, error) {
					if path != "/tmp/test-config.json" {
						t.Fatalf("load path = %q; want /tmp/test-config.json", path)
					}
					return baseConfig, nil
				}),
				WithConfigSaver(func(path string, cfg config.Config) error {
					savedPath = path
					savedConfig = cfg
					return nil
				}),
			)

			code := app.Run(append([]string{"config", "set"}, tt.args...))

			if code != ExitOK {
				t.Fatalf("config set exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
			}
			if savedPath != "/tmp/test-config.json" {
				t.Fatalf("saved path = %q; want /tmp/test-config.json", savedPath)
			}
			wantConfig := tt.want(baseConfig)
			if savedConfig != wantConfig {
				t.Fatalf("saved config = %+v; want %+v", savedConfig, wantConfig)
			}
			if !strings.Contains(stdout.String(), "updated "+tt.args[0]) {
				t.Fatalf("stdout missing update line:\n%s", stdout.String())
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Fatalf("stderr missing warning %q:\n%s", tt.wantErr, stderr.String())
			}
			if tt.wantErr == "" && stderr.Len() != 0 {
				t.Fatalf("stderr = %q; want empty", stderr.String())
			}
		})
	}
}

func TestConfigSetRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown key", args: []string{"unknown", "value"}, want: "unsupported config key"},
		{name: "bad mode", args: []string{"mode", "prod"}, want: "mode"},
		{name: "bad proxy protocol", args: []string{"proxy-protocol", "ftp"}, want: "proxy_protocol"},
		{name: "bad port", args: []string{"socks-port", "not-a-port"}, want: "socks-port"},
		{name: "unsafe tun interface", args: []string{"tun-interface", "bad interface"}, want: "tun_interface"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			saveCalled := false
			app := NewApp(&stdout, &stderr,
				WithGeteuid(func() int { return 0 }),
				WithConfigLoader(func(string) (config.Config, error) {
					return config.Default(), nil
				}),
				WithConfigSaver(func(string, config.Config) error {
					saveCalled = true
					return nil
				}),
			)

			code := app.Run(append([]string{"config", "set"}, tt.args...))

			if code != ExitUsage {
				t.Fatalf("config set exit code = %d; want %d", code, ExitUsage)
			}
			if saveCalled {
				t.Fatal("save was called for invalid input")
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q; want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q; want mention %q", stderr.String(), tt.want)
			}
		})
	}
}

func TestConfigShowReportsLoadError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Config{}, errors.New("bad config")
		}),
	)

	code := app.Run([]string{"config", "show"})

	if code != ExitError {
		t.Fatalf("config show exit code = %d; want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "load config") {
		t.Fatalf("stderr missing load context:\n%s", stderr.String())
	}
}

func TestRenderPrintsSingBoxAndNFTablesOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cfg := config.Default()
	outputRunner := &recordingOutputRunner{
		output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
	}
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
		WithOutputRunner(outputRunner),
	)

	code := app.Run([]string{"render"})

	if code != ExitOK {
		t.Fatalf("render exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	for _, want := range []string{
		"sing-box.json:",
		`"interface_name": "wng-tun"`,
		`"server": "172.25.80.1"`,
		"nftables.nft:",
		"table inet wsl_netguard",
		"ip daddr 172.25.80.1 accept",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("render output missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestRenderRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"render", "sing-box"})

	if code != ExitUsage {
		t.Fatalf("render with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "render does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestRenderReportsGatewayDetectionError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{err: errors.New("ip failed")}),
	)

	code := app.Run([]string{"render"})

	if code != ExitError {
		t.Fatalf("render exit code = %d; want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "detect windows host IP") {
		t.Fatalf("stderr missing detection context:\n%s", stderr.String())
	}
}

func TestServiceRenderSingBoxRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	writeCalled := false
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			writeCalled = true
			return nil
		}),
	)

	code := app.Run([]string{"service", "render-sing-box"})

	if code != ExitError {
		t.Fatalf("service render-sing-box exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if writeCalled {
		t.Fatal("artifact writer was called without root")
	}
}

func TestServiceRenderSingBoxWritesRuntimeArtifacts(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var writtenArtifacts []runtimefiles.Artifact
	runner := &recordingRunner{}
	renderedAt := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithNow(func() time.Time { return renderedAt }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func(artifacts []runtimefiles.Artifact) error {
			writtenArtifacts = append([]runtimefiles.Artifact(nil), artifacts...)
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "render-sing-box"})

	if code != ExitOK {
		t.Fatalf("service render-sing-box exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if len(writtenArtifacts) != 4 {
		t.Fatalf("written artifact length = %d; want 4", len(writtenArtifacts))
	}
	wantPaths := []string{
		runtimefiles.SingBoxPath,
		runtimefiles.NFTablesPath,
		runtimefiles.NFTablesCheckPath,
		runtimefiles.StatePath,
	}
	for i, wantPath := range wantPaths {
		if writtenArtifacts[i].Path != wantPath {
			t.Fatalf("artifact[%d] path = %q; want %q", i, writtenArtifacts[i].Path, wantPath)
		}
	}
	if !strings.Contains(string(writtenArtifacts[0].Data), `"server": "172.25.80.1"`) {
		t.Fatalf("sing-box artifact missing gateway:\n%s", string(writtenArtifacts[0].Data))
	}
	if !strings.Contains(string(writtenArtifacts[3].Data), `"rendered_at": "2026-06-12T09:30:00Z"`) {
		t.Fatalf("state artifact missing render time:\n%s", string(writtenArtifacts[3].Data))
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stdout.String(), "rendered runtime artifacts") {
		t.Fatalf("stdout missing success message:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestServiceRenderSingBoxReportsValidationError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("invalid sing-box config")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "render-sing-box"})

	if code != ExitError {
		t.Fatalf("service render-sing-box exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"sing-box", "check", "-c", runtimefiles.SingBoxPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "validate sing-box config") {
		t.Fatalf("stderr missing validation context:\n%s", stderr.String())
	}
}

func TestServiceRenderSingBoxReportsWriterError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return errors.New("write failed")
		}),
	)

	code := app.Run([]string{"service", "render-sing-box"})

	if code != ExitError {
		t.Fatalf("service render-sing-box exit code = %d; want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "write runtime artifacts") {
		t.Fatalf("stderr missing writer context:\n%s", stderr.String())
	}
}

func TestServiceRejectsUnsupportedHook(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"service", "unknown"})

	if code != ExitUsage {
		t.Fatalf("service unknown exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "usage: wsl-netguard service <render-sing-box|apply-killswitch>") {
		t.Fatalf("stderr missing service usage:\n%s", stderr.String())
	}
}

func TestServiceApplyKillSwitchRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "apply-killswitch"})

	if code != ExitError {
		t.Fatalf("service apply-killswitch exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
}

func TestServiceApplyKillSwitchWritesArtifactsThenRunsNFT(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var writtenArtifacts []runtimefiles.Artifact
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func(artifacts []runtimefiles.Artifact) error {
			writtenArtifacts = append([]runtimefiles.Artifact(nil), artifacts...)
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "apply-killswitch"})

	if code != ExitOK {
		t.Fatalf("service apply-killswitch exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if len(writtenArtifacts) != 4 {
		t.Fatalf("written artifact length = %d; want 4", len(writtenArtifacts))
	}
	wantCommands := [][]string{
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"nft", "-f", runtimefiles.NFTablesPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stdout.String(), "applied kill switch") {
		t.Fatalf("stdout missing success message:\n%s", stdout.String())
	}
}

func TestServiceApplyKillSwitchReportsNFTError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{2: errors.New("nft failed")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "apply-killswitch"})

	if code != ExitError {
		t.Fatalf("service apply-killswitch exit code = %d; want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "apply nftables kill switch") {
		t.Fatalf("stderr missing nft context:\n%s", stderr.String())
	}
}

func TestServiceApplyKillSwitchReportsNFTValidationError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("invalid nftables syntax")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "apply-killswitch"})

	if code != ExitError {
		t.Fatalf("service apply-killswitch exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "validate nftables kill switch") {
		t.Fatalf("stderr missing validation context:\n%s", stderr.String())
	}
}

func TestServiceApplyKillSwitchRejectsUnexpectedDeleteError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{1: errors.New("permission denied")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{
			output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
		}),
		WithArtifactWriter(func([]runtimefiles.Artifact) error {
			return nil
		}),
		WithRunner(runner),
	)

	code := app.Run([]string{"service", "apply-killswitch"})

	if code != ExitError {
		t.Fatalf("service apply-killswitch exit code = %d; want %d", code, ExitError)
	}
	wantCommands := [][]string{
		{"nft", "-c", "-f", runtimefiles.NFTablesCheckPath},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("runner commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stderr.String(), "delete existing managed nftables table") {
		t.Fatalf("stderr missing delete context:\n%s", stderr.String())
	}
}

func TestStatusPrintsBasicOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cfg := config.Default()
	cfg.Mode = config.ModeStrict
	outputRunner := &recordingOutputRunner{
		outputs: map[string][]byte{
			cliCommandKey("ip", "route", "show", "default"):                     []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
			cliCommandKey("systemctl", "is-active", recovery.KillSwitchService): []byte("active\n"),
			cliCommandKey("ip", "link", "show", "dev", "wng-tun"):               []byte("12: wng-tun: <POINTOPOINT,UP>\n"),
			cliCommandKey("nft", "list", "table", "inet", "wsl_netguard"):       []byte("table inet wsl_netguard {}\n"),
		},
		errorsByCommand: map[string]error{
			cliCommandKey("systemctl", "is-active", recovery.SingBoxService): errors.New("inactive"),
		},
	}
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(_ context.Context, address string) error {
			if strings.HasSuffix(address, ":10809") {
				return errors.New("connection refused")
			}
			return nil
		}),
	)

	code := app.Run([]string{"status"})

	if code != ExitOK {
		t.Fatalf("status exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	for _, want := range []string{
		"mode: strict",
		"windows_host_ip: 172.25.80.1",
		"gateway_interface: eth0",
		"tun_interface: wng-tun",
		"v2rayn_socks: reachable (172.25.80.1:10808)",
		"v2rayn_http: unreachable (172.25.80.1:10809)",
		"kill_switch_service: active (systemctl is-active wsl-netguard-killswitch.service)",
		"sing_box_service: inactive (systemctl is-active wsl-netguard-sing-box.service)",
		"tun_status: active (wng-tun)",
		"nftables_table: active (inet wsl_netguard)",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status output missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestStatusExcludesManagedTunDefaultRoute(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputRunner := &recordingOutputRunner{
		output: []byte(strings.Join([]string{
			"default dev wng-tun scope link",
			"default via 172.28.96.1 dev eth0 proto kernel",
		}, "\n")),
	}
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
	)

	code := app.Run([]string{"status"})

	if code != ExitOK {
		t.Fatalf("status exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "windows_host_ip: 172.28.96.1") {
		t.Fatalf("status output did not use non-TUN gateway:\n%s", stdout.String())
	}
}

func TestStatusReportsRouteDetectionError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	wantErr := errors.New("ip command failed")
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{err: wantErr}),
	)

	code := app.Run([]string{"status"})

	if code != ExitError {
		t.Fatalf("status exit code = %d; want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "detect windows host IP") {
		t.Fatalf("stderr missing detection context:\n%s", stderr.String())
	}
}

func TestDoctorPrintsDependenciesGatewayProxyAndRuntimeChecks(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cfg := config.Default()
	outputRunner := doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(_ context.Context, address string) error {
			if strings.HasSuffix(address, ":10809") {
				return errors.New("connection refused")
			}
			return nil
		}),
	)

	code := app.Run([]string{"doctor"})

	if code != ExitOK {
		t.Fatalf("doctor exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	for _, want := range []string{
		"dependency_systemd: installed (required)",
		"dependency_nft: installed (required)",
		"dependency_sing_box: installed (required)",
		"dependency_nc: missing (optional)",
		"tun_device: available (/dev/net/tun)",
		"dns_config: available (/etc/resolv.conf)",
		"windows_host_ip: 172.25.80.1",
		"v2rayn_socks: reachable (172.25.80.1:10808)",
		"v2rayn_http: unreachable (172.25.80.1:10809)",
		"kill_switch_service: active (systemctl is-active wsl-netguard-killswitch.service)",
		"tun_status: active (wng-tun)",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestDoctorReturnsUnavailableForMissingRequiredEnvironmentCheck(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputRunner := doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")
	outputRunner.errorsByCommand[cliCommandKey("test", "-c", "/dev/net/tun")] = errors.New("missing tun")
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
	)

	code := app.Run([]string{"doctor"})

	if code != ExitUnavailable {
		t.Fatalf("doctor exit code = %d; want %d", code, ExitUnavailable)
	}
	if !strings.Contains(stdout.String(), "tun_device: missing (/dev/net/tun)") {
		t.Fatalf("doctor output missing TUN failure:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestDoctorReturnsUnavailableForMissingRequiredDependency(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputRunner := doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")
	outputRunner.errorsByCommand[cliCommandKey("sing-box", "version")] = errors.New("missing")
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
	)

	code := app.Run([]string{"doctor"})

	if code != ExitUnavailable {
		t.Fatalf("doctor exit code = %d; want %d", code, ExitUnavailable)
	}
	if !strings.Contains(stdout.String(), "dependency_sing_box: missing (required)") {
		t.Fatalf("doctor output missing missing sing-box dependency:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestDoctorReturnsUnavailableForConfiguredProxyPortFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")),
		WithTCPProbe(func(_ context.Context, address string) error {
			if strings.HasSuffix(address, ":10808") {
				return errors.New("connection refused")
			}
			return nil
		}),
	)

	code := app.Run([]string{"doctor"})

	if code != ExitUnavailable {
		t.Fatalf("doctor exit code = %d; want %d", code, ExitUnavailable)
	}
	if !strings.Contains(stdout.String(), "v2rayn_socks: unreachable (172.25.80.1:10808)") {
		t.Fatalf("doctor output missing unreachable configured proxy:\n%s", stdout.String())
	}
}

func TestDoctorRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"doctor", "--verbose"})

	if code != ExitUsage {
		t.Fatalf("doctor with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "doctor does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestExportDiagnosticsPrintsJSONBundle(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	generatedAt := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	app := NewApp(&stdout, &stderr,
		WithNow(func() time.Time { return generatedAt }),
		WithConfigLoader(func(string) (config.Config, error) {
			cfg := config.Default()
			cfg.Mode = config.ModeStrict
			return cfg, nil
		}),
		WithOutputRunner(doctorOutputRunner("default via 172.25.80.1 dev eth0 proto kernel\n")),
		WithTCPProbe(func(_ context.Context, address string) error {
			if strings.HasSuffix(address, ":10809") {
				return errors.New("connection refused")
			}
			return nil
		}),
	)

	code := app.Run([]string{"export-diagnostics"})

	if code != ExitOK {
		t.Fatalf("export-diagnostics exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	doc := decodeDiagnosticsExport(t, stdout.Bytes())
	if doc.GeneratedAt != generatedAt {
		t.Fatalf("generated_at = %s; want %s", doc.GeneratedAt, generatedAt)
	}
	if doc.Config.Mode != config.ModeStrict {
		t.Fatalf("config mode = %q; want strict", doc.Config.Mode)
	}
	if doc.Gateway.WindowsHostIP != "172.25.80.1" || doc.Gateway.GatewayInterface != "eth0" || doc.Gateway.Error != "" {
		t.Fatalf("gateway = %+v; want detected eth0 gateway", doc.Gateway)
	}
	if len(doc.Checks.Dependencies) == 0 || len(doc.Checks.Environment) == 0 || len(doc.Checks.ProxyPorts) == 0 || len(doc.Checks.Runtime) == 0 {
		t.Fatalf("diagnostic checks should all be populated: %+v", doc.Checks)
	}
	if !hasCheck(doc.Checks.ProxyPorts, "v2rayn_socks", "reachable") {
		t.Fatalf("proxy checks missing reachable SOCKS entry: %+v", doc.Checks.ProxyPorts)
	}
}

func TestExportDiagnosticsReportsGatewayErrorInJSON(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(doctorOutputRunner("")),
		WithTCPProbe(func(context.Context, string) error {
			t.Fatal("proxy probe should not run when gateway detection fails")
			return nil
		}),
	)

	code := app.Run([]string{"export-diagnostics"})

	if code != ExitUnavailable {
		t.Fatalf("export-diagnostics exit code = %d; want %d; stderr=%q", code, ExitUnavailable, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	doc := decodeDiagnosticsExport(t, stdout.Bytes())
	if doc.Gateway.Error == "" {
		t.Fatalf("gateway error is empty in diagnostics export: %+v", doc.Gateway)
	}
	if len(doc.Checks.ProxyPorts) != 0 {
		t.Fatalf("proxy checks = %+v; want none without gateway", doc.Checks.ProxyPorts)
	}
	if len(doc.Checks.Runtime) == 0 {
		t.Fatalf("runtime checks should still be included when gateway detection fails")
	}
}

func TestExportDiagnosticsRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"export-diagnostics", "--verbose"})

	if code != ExitUsage {
		t.Fatalf("export-diagnostics with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "export-diagnostics does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestTestCommandReportsProxyDNSAndDirectBlock(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(networkTestOutputRunner(true)),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
	)

	code := app.Run([]string{"test"})

	if code != ExitOK {
		t.Fatalf("test exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	for _, want := range []string{
		"windows_host_ip: 172.25.80.1",
		"v2rayn_socks: reachable (172.25.80.1:10808)",
		"proxy_external: ok (https://api.ipify.org)",
		"dns_query: ok (https://one.one.one.one/dns-query?name=example.com&type=A)",
		"direct_external: blocked (interface eth0)",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("test output missing %q:\n%s", want, stdout.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestTestCommandReturnsUnavailableWhenDirectExternalIsReachable(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(networkTestOutputRunner(false)),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
	)

	code := app.Run([]string{"test"})

	if code != ExitUnavailable {
		t.Fatalf("test exit code = %d; want %d", code, ExitUnavailable)
	}
	if !strings.Contains(stdout.String(), "direct_external: reachable (interface eth0)") {
		t.Fatalf("test output missing direct reachable failure:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}

func TestTestCommandReturnsUnavailableWhenProxyExternalFails(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	outputRunner := networkTestOutputRunner(true)
	outputRunner.errorsByCommand[cliCommandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "https://api.ipify.org")] = errors.New("curl failed")
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(outputRunner),
		WithTCPProbe(func(context.Context, string) error {
			return nil
		}),
	)

	code := app.Run([]string{"test"})

	if code != ExitUnavailable {
		t.Fatalf("test exit code = %d; want %d", code, ExitUnavailable)
	}
	if !strings.Contains(stdout.String(), "proxy_external: failed (https://api.ipify.org)") {
		t.Fatalf("test output missing proxy failure:\n%s", stdout.String())
	}
}

func TestTestCommandRejectsArguments(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr)

	code := app.Run([]string{"test", "--quick"})

	if code != ExitUsage {
		t.Fatalf("test with args exit code = %d; want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "test does not accept arguments") {
		t.Fatalf("stderr missing argument error:\n%s", stderr.String())
	}
}

func TestTestCommandReportsGatewayDetectionError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := NewApp(&stdout, &stderr,
		WithConfigLoader(func(string) (config.Config, error) {
			return config.Default(), nil
		}),
		WithOutputRunner(&recordingOutputRunner{err: errors.New("ip failed")}),
	)

	code := app.Run([]string{"test"})

	if code != ExitError {
		t.Fatalf("test exit code = %d; want %d", code, ExitError)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "detect windows host IP") {
		t.Fatalf("stderr missing gateway context:\n%s", stderr.String())
	}
}

func TestDisableRequiresRoot(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 1000 }),
		WithRunner(runner),
	)

	code := app.Run([]string{"disable"})

	if code != ExitError {
		t.Fatalf("disable exit code = %d; want %d", code, ExitError)
	}
	if !strings.Contains(stderr.String(), "sudo") {
		t.Fatalf("stderr = %q; want sudo guidance", stderr.String())
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %+v; want none", runner.commands)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q; want empty", stdout.String())
	}
}

func TestDisableExecutesConfiguredRecoveryActions(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	cfg := config.Default()
	cfg.TUNInterface = "custom-tun"
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
	)

	code := app.Run([]string{"disable"})

	if code != ExitOK {
		t.Fatalf("disable exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantCommands := [][]string{
		{"systemctl", "stop", recovery.SingBoxService},
		{"systemctl", "stop", recovery.KillSwitchService},
		{"nft", "delete", "table", recovery.NFTablesFamily, recovery.NFTablesTable},
		{"ip", "link", "delete", "custom-tun"},
		{"rm", "-rf", recovery.RuntimeDir},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if !strings.Contains(stdout.String(), "ok: stop sing-box service") {
		t.Fatalf("stdout missing success line:\n%s", stdout.String())
	}
}

func TestDisableIgnoresMissingManagedResources(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{
		0: errors.New("Unit wsl-netguard-sing-box.service not loaded."),
		1: errors.New("Unit wsl-netguard-killswitch.service could not be found."),
		2: errors.New("Error: Could not process rule: No such file or directory"),
		3: errors.New("Cannot find device \"custom-tun\""),
	}}
	cfg := config.Default()
	cfg.TUNInterface = "custom-tun"
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
		WithConfigLoader(func(string) (config.Config, error) {
			return cfg, nil
		}),
	)

	code := app.Run([]string{"disable"})

	if code != ExitOK {
		t.Fatalf("disable exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "ok: delete managed TUN interface") {
		t.Fatalf("stdout missing TUN success line:\n%s", stdout.String())
	}
}

func TestRescueDoesNotLoadConfig(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
		WithConfigLoader(func(string) (config.Config, error) {
			t.Fatal("rescue must not load config")
			return config.Config{}, nil
		}),
	)

	code := app.Run([]string{"rescue"})

	if code != ExitOK {
		t.Fatalf("rescue exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	wantLastCommand := []string{"rm", "-rf", recovery.RuntimeDir}
	if !reflect.DeepEqual(runner.commands[len(runner.commands)-1], wantLastCommand) {
		t.Fatalf("last command = %+v; want %+v", runner.commands[len(runner.commands)-1], wantLastCommand)
	}
	wantTunCommand := []string{"ip", "link", "delete", recovery.DefaultTUNInterface}
	if !reflect.DeepEqual(runner.commands[3], wantTunCommand) {
		t.Fatalf("tun command = %+v; want %+v", runner.commands[3], wantTunCommand)
	}
}

func TestRescueReportsFailuresAfterTryingAllActions(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{0: errors.New("permission denied")}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
	)

	code := app.Run([]string{"rescue"})

	if code != ExitError {
		t.Fatalf("rescue exit code = %d; want %d", code, ExitError)
	}
	if len(runner.commands) != len(recovery.RescueActions()) {
		t.Fatalf("commands length = %d; want %d", len(runner.commands), len(recovery.RescueActions()))
	}
	if !strings.Contains(stderr.String(), "failed: stop sing-box service") {
		t.Fatalf("stderr missing failure line:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "ok: stop kill switch service") {
		t.Fatalf("stdout missing later success line:\n%s", stdout.String())
	}
}

func TestRescueIgnoresMissingManagedResources(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := &recordingRunner{errors: map[int]error{
		0: errors.New("Unit wsl-netguard-sing-box.service not loaded."),
		1: errors.New("Unit wsl-netguard-killswitch.service could not be found."),
		2: errors.New("Error: Could not process rule: No such file or directory"),
		3: errors.New("Cannot find device \"wng-tun\""),
	}}
	app := NewApp(&stdout, &stderr,
		WithGeteuid(func() int { return 0 }),
		WithRunner(runner),
	)

	code := app.Run([]string{"rescue"})

	if code != ExitOK {
		t.Fatalf("rescue exit code = %d; want %d; stderr=%q", code, ExitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
	for _, want := range []string{
		"ok: stop sing-box service",
		"ok: stop kill switch service",
		"ok: delete managed nftables table",
		"ok: delete managed TUN interface",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
}

type recordingRunner struct {
	commands [][]string
	errors   map[int]error
}

func (runner *recordingRunner) Run(_ context.Context, name string, args ...string) error {
	command := append([]string{name}, args...)
	runner.commands = append(runner.commands, command)
	if runner.errors == nil {
		return nil
	}
	return runner.errors[len(runner.commands)-1]
}

func doctorOutputRunner(defaultRoute string) *recordingOutputRunner {
	return &recordingOutputRunner{
		outputs: map[string][]byte{
			cliCommandKey("systemctl", "--version"):                             []byte("systemd 255\n"),
			cliCommandKey("nft", "--version"):                                   []byte("nftables v1.0.9\n"),
			cliCommandKey("sing-box", "version"):                                []byte("sing-box 1.10.0\n"),
			cliCommandKey("ip", "-Version"):                                     []byte("ip utility\n"),
			cliCommandKey("curl", "--version"):                                  []byte("curl 8.0\n"),
			cliCommandKey("journalctl", "--version"):                            []byte("systemd 255\n"),
			cliCommandKey("test", "-c", "/dev/net/tun"):                         []byte{},
			cliCommandKey("test", "-s", "/etc/resolv.conf"):                     []byte{},
			cliCommandKey("ip", "route", "show", "default"):                     defaultRouteBytes(defaultRoute),
			cliCommandKey("systemctl", "is-active", recovery.KillSwitchService): []byte("active\n"),
			cliCommandKey("systemctl", "is-active", recovery.SingBoxService):    []byte("active\n"),
			cliCommandKey("ip", "link", "show", "dev", "wng-tun"):               []byte("12: wng-tun: <POINTOPOINT,UP>\n"),
			cliCommandKey("nft", "list", "table", "inet", "wsl_netguard"):       []byte("table inet wsl_netguard {}\n"),
		},
		errorsByCommand: map[string]error{
			cliCommandKey("nc", "-h"):                errors.New("missing"),
			cliCommandKey("resolvectl", "--version"): errors.New("missing"),
		},
	}
}

func networkTestOutputRunner(directBlocked bool) *recordingOutputRunner {
	runner := &recordingOutputRunner{
		outputs: map[string][]byte{
			cliCommandKey("ip", "route", "show", "default"):                                                         []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
			cliCommandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "https://api.ipify.org"): []byte("203.0.113.10\n"),
			cliCommandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "-H", "accept: application/dns-json", "https://one.one.one.one/dns-query?name=example.com&type=A"): []byte(`{"Status":0}`),
		},
		errorsByCommand: map[string]error{},
	}

	directCommand := cliCommandKey("curl", "--max-time", "8", "--silent", "--show-error", "--fail", "--noproxy", "*", "--interface", "eth0", "https://api.ipify.org")
	if directBlocked {
		runner.errorsByCommand[directCommand] = errors.New("blocked")
		return runner
	}

	runner.outputs[directCommand] = []byte("198.51.100.20\n")
	return runner
}

func defaultRouteBytes(defaultRoute string) []byte {
	return []byte(defaultRoute)
}

type recordingOutputRunner struct {
	command         []string
	commands        [][]string
	output          []byte
	outputs         map[string][]byte
	err             error
	errorsByCommand map[string]error
}

func (runner *recordingOutputRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	command := append([]string{name}, args...)
	runner.command = command
	runner.commands = append(runner.commands, command)
	key := cliCommandKey(name, args...)
	if err, ok := runner.errorsByCommand[key]; ok {
		return nil, err
	}
	if runner.err != nil {
		return nil, runner.err
	}
	if output, ok := runner.outputs[key]; ok {
		return output, nil
	}
	return runner.output, nil
}

func cliCommandKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}

type diagnosticsExportForTest struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Config      config.Config `json:"config"`
	Gateway     struct {
		WindowsHostIP    string `json:"windows_host_ip"`
		GatewayInterface string `json:"gateway_interface"`
		Error            string `json:"error"`
	} `json:"gateway"`
	Checks struct {
		Dependencies []diagnostic.Check `json:"dependencies"`
		Environment  []diagnostic.Check `json:"environment"`
		ProxyPorts   []diagnostic.Check `json:"proxy_ports"`
		Runtime      []diagnostic.Check `json:"runtime"`
	} `json:"checks"`
}

func decodeDiagnosticsExport(t *testing.T, data []byte) diagnosticsExportForTest {
	t.Helper()

	var doc diagnosticsExportForTest
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode diagnostics export: %v\n%s", err, string(data))
	}
	return doc
}

func hasCheck(checks []diagnostic.Check, name string, status string) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}

func writeTestExecutable(t *testing.T, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "wsl-netguard")
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatalf("write test executable: %v", err)
	}
	return path
}
