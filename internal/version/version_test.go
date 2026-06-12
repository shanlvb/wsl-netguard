package version

import "testing"

func TestCurrentReturnsBuildMetadata(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalDate := Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = "v1.2.3"
	Commit = "abc123"
	Date = "2026-06-13T00:00:00Z"

	got := Current()

	if got.Version != "v1.2.3" {
		t.Fatalf("version = %q; want v1.2.3", got.Version)
	}
	if got.Commit != "abc123" {
		t.Fatalf("commit = %q; want abc123", got.Commit)
	}
	if got.Date != "2026-06-13T00:00:00Z" {
		t.Fatalf("date = %q; want 2026-06-13T00:00:00Z", got.Date)
	}
}
