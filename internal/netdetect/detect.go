package netdetect

import (
	"context"
	"fmt"
)

type OutputRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

func DetectDefaultGateway(ctx context.Context, runner OutputRunner, excludedInterface string) (Gateway, error) {
	output, err := runner.Output(ctx, "ip", "route", "show", "default")
	if err != nil {
		return Gateway{}, fmt.Errorf("show default routes: %w", err)
	}

	gateway, err := ParseDefaultGateway(string(output), excludedInterface)
	if err != nil {
		return Gateway{}, fmt.Errorf("parse default routes: %w", err)
	}
	return gateway, nil
}
