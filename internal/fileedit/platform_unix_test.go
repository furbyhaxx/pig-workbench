//go:build unix

package fileedit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitRequiresPrivateLockDir(t *testing.T) {
	dir := t.TempDir()
	locks := filepath.Join(dir, "locks")
	if err := os.Mkdir(locks, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(locks, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	path := filepath.Join(dir, "file.txt")
	revision := writeFile(t, path, "content", 0o644)

	_, err := Commit(context.Background(), locks, []Effect{
		{Path: path, Revision: revision, Bytes: []byte("next")},
	})
	if err == nil || !strings.Contains(err.Error(), "must not be group or world accessible") {
		t.Fatalf("Commit error = %v, want a lock directory rejection", err)
	}
	if got := readFile(t, path); got != "content" {
		t.Fatalf("file = %q", got)
	}
}

func TestCommitLockFilesArePrivate(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	path := filepath.Join(dir, "file.txt")
	revision := writeFile(t, path, "content", 0o644)

	if _, err := Commit(context.Background(), locks, []Effect{
		{Path: path, Revision: revision, Bytes: []byte("next")},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	entries, err := os.ReadDir(locks)
	if err != nil {
		t.Fatalf("read lock dir: %v", err)
	}
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), ".lock") {
		t.Fatalf("lock entries = %v, want one .lock file", entries)
	}
	info, err := os.Lstat(filepath.Join(locks, entries[0].Name()))
	if err != nil {
		t.Fatalf("lstat lock: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("lock file mode = %v, want a regular 0600 file", info.Mode())
	}
}
