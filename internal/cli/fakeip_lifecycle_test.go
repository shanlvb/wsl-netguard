package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/config"
)

func TestRecoveryExplainsStaleFakeIPAndRetainsCache(t *testing.T) {
	for _, command := range []string{"disable", "rescue"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			runner := &recordingRunner{}
			app := NewApp(&stdout, &stderr, WithGeteuid(func() int { return 0 }), WithRunner(runner), WithConfigLoader(func(string) (config.Config, error) { return config.Default(), nil }))
			if code := app.Run([]string{command}); code != ExitOK {
				t.Fatalf("exit %d: %s", code, &stderr)
			}
			if !strings.Contains(stdout.String(), "FakeIP") || !strings.Contains(stdout.String(), "restart") {
				t.Fatalf("missing cache recovery guidance: %s", &stdout)
			}
			for _, cmd := range runner.commands {
				if strings.Contains(strings.Join(cmd, " "), "/var/lib/wsl-netguard") {
					t.Fatalf("recovery touches persistent state: %v", cmd)
				}
			}
		})
	}
}

func TestUninstallPreservesUnrelatedStateFiles(t *testing.T) {
	for _, message := range []string{"Directory not empty", "No such file or directory", "Permission denied"} {
		t.Run(message, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			stateStep := -1
			for i, step := range uninstallSteps() {
				if strings.Join(step.command, " ") == "rmdir /var/lib/wsl-netguard" {
					stateStep = i
					break
				}
			}
			if stateStep < 0 {
				t.Fatal("missing state directory cleanup")
			}
			runner := &recordingRunner{errors: map[int]error{stateStep: errors.New(message)}}
			app := NewApp(&stdout, &stderr, WithRunner(runner))
			var got error
			for _, step := range uninstallSteps() {
				if got = app.runUninstallStep(step); got != nil {
					break
				}
			}
			if (got != nil) != (message == "Permission denied") {
				t.Fatalf("cleanup error = %v", got)
			}
		})
	}
}
