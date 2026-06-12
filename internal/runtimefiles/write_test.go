package runtimefiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteArtifactsCreatesFilesWithModes(t *testing.T) {
	dir := t.TempDir()
	artifacts := []Artifact{
		{
			Path: filepath.Join(dir, "run", "sing-box.json"),
			Data: []byte(`{"ok":true}` + "\n"),
			Mode: 0o644,
		},
		{
			Path: filepath.Join(dir, "run", "nested", "state.json"),
			Data: []byte(`{"state":"ok"}` + "\n"),
			Mode: 0o600,
		},
	}

	if err := WriteArtifacts(artifacts); err != nil {
		t.Fatalf("WriteArtifacts returned error: %v", err)
	}

	for _, artifact := range artifacts {
		data, err := os.ReadFile(artifact.Path)
		if err != nil {
			t.Fatalf("read artifact %s: %v", artifact.Path, err)
		}
		if string(data) != string(artifact.Data) {
			t.Fatalf("artifact %s data = %q; want %q", artifact.Path, string(data), string(artifact.Data))
		}

		info, err := os.Stat(artifact.Path)
		if err != nil {
			t.Fatalf("stat artifact %s: %v", artifact.Path, err)
		}
		if got := info.Mode().Perm(); got != os.FileMode(artifact.Mode) {
			t.Fatalf("artifact %s mode = %#o; want %#o", artifact.Path, got, artifact.Mode)
		}
	}
}

func TestWriteArtifactsOverwritesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := WriteArtifacts([]Artifact{
		{Path: path, Data: []byte("new"), Mode: 0o644},
	})
	if err != nil {
		t.Fatalf("WriteArtifacts returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overwritten file: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("overwritten data = %q; want new", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat overwritten file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("overwritten mode = %#o; want 0644", got)
	}
}

func TestWriteArtifactsReportsDirectoryCreationError(t *testing.T) {
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("blocks mkdir"), 0o644); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}

	err := WriteArtifacts([]Artifact{
		{
			Path: filepath.Join(blockingFile, "state.json"),
			Data: []byte(`{"state":"ok"}` + "\n"),
			Mode: 0o644,
		},
	})

	if err == nil {
		t.Fatal("WriteArtifacts returned nil error; want directory creation error")
	}
}

func TestWriteArtifactsRejectsInvalidArtifact(t *testing.T) {
	tests := []struct {
		name     string
		artifact Artifact
	}{
		{name: "empty path", artifact: Artifact{Data: []byte("data"), Mode: 0o644}},
		{name: "empty data", artifact: Artifact{Path: filepath.Join(t.TempDir(), "file"), Mode: 0o644}},
		{name: "empty mode", artifact: Artifact{Path: filepath.Join(t.TempDir(), "file"), Data: []byte("data")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WriteArtifacts([]Artifact{tt.artifact}); err == nil {
				t.Fatal("WriteArtifacts returned nil error; want error")
			}
		})
	}
}
