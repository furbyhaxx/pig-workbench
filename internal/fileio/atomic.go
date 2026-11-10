package fileio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic replaces path with data through a synced temporary file in the same directory,
// created with mode. It reports whether the replacement landed: true with an error means the
// new content is in place but the directory could not be synced.
func WriteAtomic(ctx context.Context, path string, data []byte, mode os.FileMode) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return false, fmt.Errorf("create temporary file: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	closed := false
	defer func() {
		if !closed {
			file.Close()
		}
	}()
	if err := file.Chmod(mode.Perm()); err != nil {
		return false, fmt.Errorf("set temporary file permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return false, fmt.Errorf("write: %w", err)
	}
	if err := file.Sync(); err != nil {
		return false, fmt.Errorf("flush: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, fmt.Errorf("close: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := os.Rename(name, path); err != nil {
		return false, fmt.Errorf("replace: %w", err)
	}
	if err := SyncDirectory(filepath.Dir(path)); err != nil {
		return true, fmt.Errorf("saved but directory durability is uncertain: %w", err)
	}
	return true, nil
}
