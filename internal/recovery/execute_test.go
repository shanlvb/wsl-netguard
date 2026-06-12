package recovery

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestExecuteRunsActionsInOrder(t *testing.T) {
	actions := []Action{
		{Summary: "first", Command: []string{"systemctl", "stop", SingBoxService}},
		{Summary: "second", Command: []string{"nft", "delete", "table", NFTablesFamily, NFTablesTable}},
	}
	runner := &fakeRunner{}

	results := Execute(context.Background(), runner, actions)

	wantCommands := [][]string{
		{"systemctl", "stop", SingBoxService},
		{"nft", "delete", "table", NFTablesFamily, NFTablesTable},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %+v; want %+v", runner.commands, wantCommands)
	}
	if HasFailures(results) {
		t.Fatalf("HasFailures = true; want false: %+v", results)
	}
}

func TestExecuteContinuesAfterFailure(t *testing.T) {
	wantErr := errors.New("permission denied")
	actions := []Action{
		{Summary: "first", Command: []string{"systemctl", "stop", SingBoxService}},
		{Summary: "second", Command: []string{"nft", "delete", "table", NFTablesFamily, NFTablesTable}},
	}
	runner := &fakeRunner{errors: map[int]error{0: wantErr}}

	results := Execute(context.Background(), runner, actions)

	if len(results) != len(actions) {
		t.Fatalf("results length = %d; want %d", len(results), len(actions))
	}
	if !errors.Is(results[0].Err, wantErr) {
		t.Fatalf("first result error = %v; want %v", results[0].Err, wantErr)
	}
	if results[1].Err != nil {
		t.Fatalf("second result error = %v; want nil", results[1].Err)
	}
	if !HasFailures(results) {
		t.Fatal("HasFailures = false; want true")
	}

	wantCommands := [][]string{
		{"systemctl", "stop", SingBoxService},
		{"nft", "delete", "table", NFTablesFamily, NFTablesTable},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %+v; want %+v", runner.commands, wantCommands)
	}
}

func TestExecuteIgnoresMissingManagedResources(t *testing.T) {
	actions := recoveryActions(DefaultTUNInterface)
	runner := &fakeRunner{errors: map[int]error{
		0: errors.New("Unit wsl-netguard-sing-box.service not loaded."),
		1: errors.New("Unit wsl-netguard-killswitch.service could not be found."),
		2: errors.New("Error: Could not process rule: No such file or directory"),
		3: errors.New("Cannot find device \"wng-tun\""),
	}}

	results := Execute(context.Background(), runner, actions)

	if HasFailures(results) {
		t.Fatalf("HasFailures = true; want false: %+v", results)
	}
	for i, result := range results {
		if result.Err != nil {
			t.Fatalf("result[%d] error = %v; want nil", i, result.Err)
		}
	}
}

func TestExecuteDoesNotIgnoreMissingCommandBinary(t *testing.T) {
	wantErr := errors.New(`exec: "systemctl": executable file not found in $PATH`)
	actions := []Action{
		{Summary: "stop sing-box service", Command: []string{"systemctl", "stop", SingBoxService}},
	}
	runner := &fakeRunner{errors: map[int]error{0: wantErr}}

	results := Execute(context.Background(), runner, actions)

	if !HasFailures(results) {
		t.Fatal("HasFailures = false; want true")
	}
	if !errors.Is(results[0].Err, wantErr) {
		t.Fatalf("result error = %v; want %v", results[0].Err, wantErr)
	}
}

func TestExecuteHandlesMalformedAction(t *testing.T) {
	results := Execute(context.Background(), &fakeRunner{}, []Action{{Summary: "empty"}})

	if len(results) != 1 {
		t.Fatalf("results length = %d; want 1", len(results))
	}
	if results[0].Err == nil {
		t.Fatal("malformed action error = nil; want error")
	}
}

type fakeRunner struct {
	commands [][]string
	errors   map[int]error
}

func (runner *fakeRunner) Run(_ context.Context, name string, args ...string) error {
	command := append([]string{name}, args...)
	runner.commands = append(runner.commands, command)

	if runner.errors == nil {
		return nil
	}
	return runner.errors[len(runner.commands)-1]
}
