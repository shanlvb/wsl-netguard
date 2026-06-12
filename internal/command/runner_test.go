package command

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestExecRunnerRunForwardsOutput(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := ExecRunner{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	err := runner.Run(context.Background(), "sh", "-c", "printf out; printf err >&2")

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if stdout.String() != "out" {
		t.Fatalf("stdout = %q; want out", stdout.String())
	}
	if stderr.String() != "err" {
		t.Fatalf("stderr = %q; want err", stderr.String())
	}
}

func TestExecRunnerRunWrapsFailure(t *testing.T) {
	runner := ExecRunner{}

	err := runner.Run(context.Background(), "sh", "-c", "exit 7")

	if err == nil {
		t.Fatal("Run returned nil error; want failure")
	}
	if !strings.Contains(err.Error(), "run sh -c exit 7") {
		t.Fatalf("Run error = %q; want command context", err.Error())
	}
}

func TestExecRunnerRunIncludesStderrOnFailure(t *testing.T) {
	var stderr bytes.Buffer
	runner := ExecRunner{
		Stderr: &stderr,
	}

	err := runner.Run(context.Background(), "sh", "-c", "printf boom >&2; exit 3")

	if err == nil {
		t.Fatal("Run returned nil error; want failure")
	}
	for _, want := range []string{"run sh -c printf boom >&2; exit 3", "boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Run error = %q; want mention %q", err.Error(), want)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q; want failure stderr captured in returned error only", stderr.String())
	}
}

func TestExecRunnerOutputReturnsStdout(t *testing.T) {
	runner := ExecRunner{}

	output, err := runner.Output(context.Background(), "sh", "-c", "printf out; printf err >&2")

	if err != nil {
		t.Fatalf("Output returned error: %v", err)
	}
	if string(output) != "out" {
		t.Fatalf("output = %q; want out", string(output))
	}
}

func TestExecRunnerOutputIncludesStderrOnFailure(t *testing.T) {
	runner := ExecRunner{}

	output, err := runner.Output(context.Background(), "sh", "-c", "printf boom >&2; exit 3")

	if err == nil {
		t.Fatal("Output returned nil error; want failure")
	}
	if len(output) != 0 {
		t.Fatalf("output = %q; want empty", string(output))
	}
	for _, want := range []string{"run sh -c printf boom >&2; exit 3", "boom"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Output error = %q; want mention %q", err.Error(), want)
		}
	}
}
