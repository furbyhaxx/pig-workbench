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

	"github.com/gofrs/flock"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
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
		lock, err := lockOne(ctx, lockPath(lockDir, identity), identity)
		if err != nil {
			release()
			return nil, err
		}
		locks = append(locks, lock)
	}
	return release, nil
}

func lockOne(ctx context.Context, path, identity string) (*flock.Flock, error) {
	for range 3 {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("acquire edit locks: %w", err)
		}
		lock := flock.New(path, flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
		locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
		if err != nil || !locked {
			lock.Close()
			if err != nil {
				return nil, fmt.Errorf("lock %s for editing: %w", identity, err)
			}
			return nil, fmt.Errorf("lock %s for editing: lock unavailable", identity)
		}
		info, err := lock.Stat()
		if err != nil || !info.Mode().IsRegular() {
			lock.Close()
			return nil, errors.New("edit lock must be a regular file")
		}
		// SweepLocks may have unlinked the file between open and lock.
		if current, err := os.Stat(path); err == nil && os.SameFile(info, current) {
			now := time.Now()
			_ = os.Chtimes(path, now, now)
			return lock, nil
		}
		lock.Close()
	}
	return nil, fmt.Errorf("lock %s for editing: lock file kept changing", identity)
}

// SweepLocks removes lock files in dir that nothing holds and no edit used for maxAge, and reports how many it removed.
func SweepLocks(ctx context.Context, dir string, maxAge time.Duration, now time.Time) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if ctx.Err() != nil {
			break
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || filepath.Ext(entry.Name()) != ".lock" || now.Sub(info.ModTime()) < maxAge {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		lock := flock.New(path, flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
		if locked, err := lock.TryLock(); err == nil && locked {
			// Unlink while holding: lockOne detects the replaced file and retries.
			if held, err := lock.Stat(); err == nil && now.Sub(held.ModTime()) >= maxAge && os.Remove(path) == nil {
				removed++
			}
		}
		lock.Close()
	}
	return removed
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
