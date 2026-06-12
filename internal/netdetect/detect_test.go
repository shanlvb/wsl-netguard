package netdetect

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestDetectDefaultGatewayRunsIPRouteCommand(t *testing.T) {
	runner := &fakeOutputRunner{
		output: []byte("default via 172.25.80.1 dev eth0 proto kernel\n"),
	}

	got, err := DetectDefaultGateway(context.Background(), runner, "wng-tun")

	if err != nil {
		t.Fatalf("DetectDefaultGateway returned error: %v", err)
	}
	want := Gateway{
		Address:   netip.MustParseAddr("172.25.80.1"),
		Interface: "eth0",
	}
	if got != want {
		t.Fatalf("DetectDefaultGateway = %+v; want %+v", got, want)
	}
	wantCommand := []string{"ip", "route", "show", "default"}
	if !reflect.DeepEqual(runner.command, wantCommand) {
		t.Fatalf("command = %+v; want %+v", runner.command, wantCommand)
	}
}

func TestDetectDefaultGatewayWrapsCommandError(t *testing.T) {
	wantErr := errors.New("ip missing")
	runner := &fakeOutputRunner{err: wantErr}

	_, err := DetectDefaultGateway(context.Background(), runner, "wng-tun")

	if err == nil {
		t.Fatal("DetectDefaultGateway returned nil error; want error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("DetectDefaultGateway error = %v; want %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), "show default routes") {
		t.Fatalf("DetectDefaultGateway error = %q; want command context", err.Error())
	}
}

type fakeOutputRunner struct {
	command []string
	output  []byte
	err     error
}

func (runner *fakeOutputRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	runner.command = append([]string{name}, args...)
	if runner.err != nil {
		return nil, runner.err
	}
	return runner.output, nil
}
