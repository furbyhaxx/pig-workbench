package fileedit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// writeTemp writes data and flushes it to a private temporary file in dir.
func writeTemp(dir, base string, data []byte, mode os.FileMode) (string, error) {
	file, err := os.CreateTemp(dir, "."+base+".*")
	if err != nil {
		return "", fmt.Errorf("create temporary file in %s: %w", dir, err)
	}
	name := file.Name()
	closed := false
	defer func() {
		if !closed {
			file.Close()
		}
	}()
	if err := file.Chmod(mode.Perm()); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("set temporary file mode for %s: %w", base, err)
	}
	if _, err := file.Write(data); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("write temporary file for %s: %w", filepath.Join(dir, base), err)
	}
	if err := file.Sync(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("flush temporary file for %s: %w", filepath.Join(dir, base), err)
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close temporary file for %s: %w", filepath.Join(dir, base), err)
	}
	closed = true
	return name, nil
}

// replaceFile atomically replaces target with data in the same directory. The
// bool reports whether the replacement landed, including when only directory
// durability failed afterwards.
func replaceFile(target string, data []byte, mode os.FileMode) (bool, error) {
	dir := dirOf(target)
	name, err := writeTemp(dir, filepath.Base(target), data, mode)
	if err != nil {
		return false, err
	}
	defer os.Remove(name)
	if err := renamePath(name, target); err != nil {
		return false, fmt.Errorf("replace %s: %w", target, err)
	}
	if err := syncDir(dir); err != nil {
		return true, fmt.Errorf("replaced %s but directory durability is uncertain: %w", target, err)
	}
	return true, nil
}

// Create publishes a new absolute path without replacing a racing destination.
// The boolean reports that creation landed, including when only directory
// durability failed afterwards.
func Create(ctx context.Context, path string, data []byte, mode os.FileMode) (bool, error) {
	if !filepath.IsAbs(path) {
		return false, errors.New("create path must be absolute")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := publishFile(path, data, mode); err != nil {
		return false, err
	}
	if err := syncDir(dirOf(path)); err != nil {
		return true, fmt.Errorf("created %s but directory durability is uncertain: %w", path, err)
	}
	return true, nil
}

// publishFile installs data at dest without replacing a racing destination.
func publishFile(dest string, data []byte, mode os.FileMode) error {
	name, err := writeTemp(dirOf(dest), filepath.Base(dest), data, mode)
	if err != nil {
		return err
	}
	defer os.Remove(name)
	return linkNoReplace(name, dest)
}

// linkNoReplace publishes another name for src at dst, so a racing destination
// is never overwritten.
func linkNoReplace(src, dst string) error {
	if err := linkPath(src, dst); err != nil {
		switch {
		case errors.Is(err, os.ErrExist):
			return fmt.Errorf("%w: %s appeared during commit; choose another destination and retry", ErrDestinationExists, dst)
		case isCrossDevice(err):
			return fmt.Errorf("%w: publishing %s at %s across filesystems needs an explicit coordinator: %v", ErrUnsupported, src, dst, err)
		default:
			return fmt.Errorf("publish %s at %s: %w", src, dst, err)
		}
	}
	return nil
}
