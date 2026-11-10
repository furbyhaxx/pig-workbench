package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/gofrs/flock"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// retainScratch claims this session's scratch directory and removes the directories finished sessions left behind.
func (s *session) retainScratch(ctx sdk.Context) {
	if s.scratch == "" {
		return
	}
	claim, err := claimScratch(s.scratch)
	if err != nil {
		ctx.Notify("Workbench could not claim its scratch directory: "+err.Error(), "warning")
	} else {
		s.scratchClaim = claim
	}
	days := 7
	if snapshot, err := s.settings.Current(); err == nil {
		days = intValue(snapshot, "storage.finished-retention-days", days)
	}
	root, own := filepath.Dir(s.scratch), filepath.Base(s.scratch)
	_ = s.supervisor.Start(func(lifetime context.Context) {
		sweepScratch(lifetime, root, own, time.Duration(days)*24*time.Hour, time.Now())
	})
}

func scratchLock(scratch string) *flock.Flock {
	return flock.New(scratch+".lock", flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
}

func holdsCurrentFile(lock *flock.Flock) bool {
	held, err := lock.Stat()
	if err != nil {
		return false
	}
	current, err := os.Stat(lock.Path())
	return err == nil && os.SameFile(held, current)
}

// claimScratch marks a scratch directory as in use until the returned lock is closed.
// The lock is shared, because sessions without an ID use one directory.
func claimScratch(scratch string) (*flock.Flock, error) {
	for range 3 {
		lock := scratchLock(scratch)
		wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		locked, err := lock.TryRLockContext(wait, 20*time.Millisecond)
		cancel()
		if err != nil || !locked {
			_ = lock.Close()
			return nil, errors.New("local scratch is being cleaned up; retry")
		}
		// A sweeper may have unlinked the file between open and lock.
		if holdsCurrentFile(lock) {
			if err := os.MkdirAll(scratch, 0700); err != nil {
				_ = lock.Close()
				return nil, err
			}
			// The sweep reads these times, also after it created the lock file itself.
			now := time.Now()
			_ = os.Chtimes(lock.Path(), now, now)
			_ = os.Chtimes(scratch, now, now)
			return lock, nil
		}
		_ = lock.Close()
	}
	return nil, errors.New("local scratch lock kept changing; retry")
}

// sweepScratch removes scratch directories no session holds and nothing touched for maxAge.
func sweepScratch(ctx context.Context, root, own string, maxAge time.Duration, now time.Time) int {
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	var bases []string
	for _, entry := range entries {
		base := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".lock"), ".args")
		if base != "" && base != own && base == sanitizeSegment(base) {
			bases = append(bases, base)
		}
	}
	slices.Sort(bases)
	removed := 0
	for _, base := range slices.Compact(bases) {
		if ctx.Err() != nil {
			return removed
		}
		scratch := filepath.Join(root, base)
		if scratchFresh(maxAge, now, scratch, scratch+".lock", materializedRoot(scratch)) {
			continue
		}
		lock := scratchLock(scratch)
		if locked, err := lock.TryLock(); err != nil || !locked {
			_ = lock.Close()
			continue
		}
		if holdsCurrentFile(lock) && !scratchFresh(maxAge, now, scratch, materializedRoot(scratch)) {
			_ = os.RemoveAll(materializedRoot(scratch))
			if os.RemoveAll(scratch) == nil {
				removed++
				// Unlink before unlocking, so a late claim notices it holds a stale file.
				_ = os.Remove(lock.Path())
			}
		}
		_ = lock.Close()
	}
	return removed
}

func scratchFresh(maxAge time.Duration, now time.Time, paths ...string) bool {
	for _, path := range paths {
		if info, err := os.Lstat(path); err == nil && now.Sub(info.ModTime()) < maxAge {
			return true
		}
	}
	return false
}
