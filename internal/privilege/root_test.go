package privilege

import (
	"errors"
	"strings"
	"testing"
)

func TestRequireRootAllowsRoot(t *testing.T) {
	if err := RequireRoot(0); err != nil {
		t.Fatalf("RequireRoot(0) returned error: %v", err)
	}
}

func TestRequireRootRejectsNonRootWithGuidance(t *testing.T) {
	err := RequireRoot(1000)
	if err == nil {
		t.Fatal("RequireRoot(1000) returned nil error; want error")
	}
	if !errors.Is(err, ErrRootRequired) {
		t.Fatalf("RequireRoot error = %v; want ErrRootRequired", err)
	}
	if !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("RequireRoot error = %q; want sudo guidance", err.Error())
	}
}
