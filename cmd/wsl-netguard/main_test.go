package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/shanlvb/wsl-netguard/internal/cli"
)

func TestRunDelegatesToCLI(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	code := run([]string{"version"}, &stdout, &stderr)

	if code != cli.ExitOK {
		t.Fatalf("run exit code = %d; want %d", code, cli.ExitOK)
	}
	if !strings.Contains(stdout.String(), "wsl-netguard version") {
		t.Fatalf("stdout missing version output:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want empty", stderr.String())
	}
}
