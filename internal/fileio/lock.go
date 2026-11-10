package fileio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gofrs/flock"
)

// ErrLockReplaced reports that the lock file on disk is not the one that was locked.
var ErrLockReplaced = errors.New("lock file changed while it was being acquired")

// Lock takes the exclusive lock file at path, created private, and waits until ctx ends.
// The held file must be regular and still be the file at path; the caller closes the lock.
func Lock(ctx context.Context, path string) (*flock.Flock, error) {
	lock := flock.New(path, flock.SetFlag(LockFlags()), flock.SetPermissions(0600))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !locked {
		lock.Close()
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = errors.New("held elsewhere")
		}
		return nil, fmt.Errorf("lock unavailable: %w", err)
	}
	held, err := lock.Stat()
	if err != nil || !held.Mode().IsRegular() {
		lock.Close()
		return nil, errors.New("lock must be a regular file")
	}
	current, err := os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(held, current) {
		lock.Close()
		return nil, ErrLockReplaced
	}
	return lock, nil
}
