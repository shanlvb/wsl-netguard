package command

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type ExecRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (runner ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	var stderr bytes.Buffer
	// #nosec G204 -- command names and arguments are built by internal command planners; no shell is invoked.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = runner.Stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return commandError(name, args, err, stderr.String())
	}
	if runner.Stderr != nil && stderr.Len() > 0 {
		if _, err := io.Copy(runner.Stderr, &stderr); err != nil {
			return fmt.Errorf("forward stderr from %s: %w", commandString(name, args), err)
		}
	}
	return nil
}

func (runner ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	// #nosec G204 -- command names and arguments are built by internal diagnostic planners; no shell is invoked.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, commandError(name, args, err, stderr.String())
	}
	return output, nil
}

func commandString(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

func commandError(name string, args []string, err error, stderr string) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		return fmt.Errorf("run %s: %w", commandString(name, args), err)
	}
	return fmt.Errorf("run %s: %w: %s", commandString(name, args), err, message)
}
