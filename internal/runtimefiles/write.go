package runtimefiles

import (
	"fmt"
	"os"
	"path/filepath"
)

func WriteArtifacts(artifacts []Artifact) error {
	for _, artifact := range artifacts {
		if err := validateArtifact(artifact); err != nil {
			return err
		}
		if err := writeArtifact(artifact); err != nil {
			return err
		}
	}
	return nil
}

func validateArtifact(artifact Artifact) error {
	if artifact.Path == "" {
		return fmt.Errorf("artifact path must not be empty")
	}
	if len(artifact.Data) == 0 {
		return fmt.Errorf("artifact %s data must not be empty", artifact.Path)
	}
	if artifact.Mode == 0 {
		return fmt.Errorf("artifact %s mode must not be empty", artifact.Path)
	}
	return nil
}

func writeArtifact(artifact Artifact) error {
	dir := filepath.Dir(artifact.Path)
	// #nosec G301 -- managed system/runtime directories are intentionally searchable by non-root tools.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(artifact.Path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary artifact %s: %w", artifact.Path, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(artifact.Data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temporary artifact %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(os.FileMode(artifact.Mode)); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temporary artifact %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary artifact %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, artifact.Path); err != nil {
		return fmt.Errorf("install artifact %s: %w", artifact.Path, err)
	}
	return nil
}
