package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/gofrs/flock"
)

func resolveTarget(path string) (string, error) {
	var missing []string
	links := 0
	for range 4096 {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			missing = append(missing, filepath.Base(path))
			path = filepath.Dir(path)
			continue
		}
		if err != nil {
			return "", fmt.Errorf("resolve configuration target: %w", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			if len(missing) > 0 && !info.IsDir() {
				return "", errors.New("configuration parent is not a directory")
			}
			canonical, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			for i := len(missing) - 1; i >= 0; i-- {
				canonical = filepath.Join(canonical, missing[i])
			}
			return canonical, nil
		}
		links++
		if links > 40 {
			return "", errors.New("configuration symlink chain exceeds 40 links")
		}
		link, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(path), link)
		}
		path = filepath.Clean(link)
	}
	return "", errors.New("configuration path resolution exceeded its depth bound")
}

func lockTarget(ctx context.Context, path string) (string, func() error, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", nil, fmt.Errorf("create configuration directory: %w", err)
	}
	target, err := resolveTarget(path)
	if err != nil {
		return "", nil, err
	}
	lock := flock.New(target+".lock", flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !locked {
		lock.Close()
		if err == nil {
			err = ctx.Err()
		}
		return "", nil, fmt.Errorf("configuration lock unavailable: %w", err)
	}
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() {
		lock.Close()
		return "", nil, errors.New("configuration lock must be a regular file")
	}
	return target, lock.Close, nil
}

func atomicWrite(ctx context.Context, path string, data []byte) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return false, fmt.Errorf("create private settings temporary file: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	closed := false
	defer func() {
		if !closed {
			file.Close()
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return false, err
	}
	if _, err := file.Write(data); err != nil {
		return false, fmt.Errorf("write settings: %w", err)
	}
	if err := file.Sync(); err != nil {
		return false, fmt.Errorf("flush settings: %w", err)
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := os.Rename(name, path); err != nil {
		return false, fmt.Errorf("replace settings: %w", err)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return true, fmt.Errorf("settings saved but directory durability is uncertain: %w", err)
	}
	return true, nil
}
