package fileedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/gofrs/flock"
)

// lockAll acquires one advisory lock per canonical identity in sorted order and
// returns a release function.
func lockAll(ctx context.Context, lockDir string, targets []target) (func(), error) {
	if err := prepareLockDir(lockDir); err != nil {
		return nil, err
	}
	identities := make([]string, 0, 2*len(targets))
	for _, target := range targets {
		identities = append(identities, target.source)
		if target.dest != "" {
			identities = append(identities, target.dest)
		}
	}
	sort.Strings(identities)
	locks := make([]*flock.Flock, 0, len(identities))
	release := func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
	}
	for _, identity := range identities {
		if err := ctx.Err(); err != nil {
			release()
			return nil, fmt.Errorf("acquire edit locks: %w", err)
		}
		lock := flock.New(lockPath(lockDir, identity), flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
		locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
		if err != nil || !locked {
			lock.Close()
			release()
			if err != nil {
				return nil, fmt.Errorf("lock %s for editing: %w", identity, err)
			}
			return nil, fmt.Errorf("lock %s for editing: lock unavailable", identity)
		}
		info, err := lock.Stat()
		if err != nil || !info.Mode().IsRegular() {
			lock.Close()
			release()
			return nil, errors.New("edit lock must be a regular file")
		}
		locks = append(locks, lock)
	}
	return release, nil
}

func prepareLockDir(dir string) error {
	if dir == "" {
		return errors.New("edit lock directory is required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create edit lock directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect edit lock directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("edit lock directory %s must be a directory", dir)
	}
	return checkLockDir(dir, info)
}

// lockPath derives the lock file from the canonical identity so that unrelated
// paths cannot collide in one directory.
func lockPath(dir, identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return filepath.Join(dir, hex.EncodeToString(sum[:16])+".lock")
}
