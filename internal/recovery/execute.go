package recovery

import (
	"context"
	"fmt"
	"strings"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type Result struct {
	Action Action
	Err    error
}

func Execute(ctx context.Context, runner Runner, actions []Action) []Result {
	results := make([]Result, 0, len(actions))
	for _, action := range actions {
		results = append(results, executeOne(ctx, runner, action))
	}
	return results
}

func HasFailures(results []Result) bool {
	for _, result := range results {
		if result.Err != nil {
			return true
		}
	}
	return false
}

func executeOne(ctx context.Context, runner Runner, action Action) Result {
	if len(action.Command) == 0 {
		return Result{
			Action: action,
			Err:    fmt.Errorf("recovery action %q has no command", action.Summary),
		}
	}

	err := runner.Run(ctx, action.Command[0], action.Command[1:]...)
	if err != nil {
		if IsMissingManagedResourceError(action.Command, err) {
			return Result{
				Action: action,
				Err:    nil,
			}
		}
		err = fmt.Errorf("%s: %w", action.Summary, err)
	}
	return Result{
		Action: action,
		Err:    err,
	}
}

func IsMissingManagedResourceError(command []string, err error) bool {
	if err == nil || len(command) == 0 {
		return false
	}

	message := strings.ToLower(err.Error())
	switch {
	case isManagedSystemctlServiceCommand(command):
		return strings.Contains(message, "not loaded") ||
			strings.Contains(message, "could not be found") ||
			strings.Contains(message, "does not exist") ||
			(strings.Contains(message, "unit ") && strings.Contains(message, " not found"))
	case isNFTablesTableDelete(command):
		return strings.Contains(message, "no such file or directory")
	case isIPLinkDelete(command):
		return strings.Contains(message, "cannot find device")
	default:
		return false
	}
}

func isManagedSystemctlServiceCommand(command []string) bool {
	if len(command) != 3 || command[0] != "systemctl" {
		return false
	}
	if command[1] != "stop" && command[1] != "disable" {
		return false
	}
	return command[2] == SingBoxService || command[2] == KillSwitchService
}

func isNFTablesTableDelete(command []string) bool {
	return len(command) == 5 &&
		command[0] == "nft" &&
		command[1] == "delete" &&
		command[2] == "table" &&
		command[3] == NFTablesFamily &&
		command[4] == NFTablesTable
}

func isIPLinkDelete(command []string) bool {
	return len(command) == 4 &&
		command[0] == "ip" &&
		command[1] == "link" &&
		command[2] == "delete"
}
