package fileedit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// lockDir returns a user-private lock directory; t.TempDir is group readable.
func lockDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "locks")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
	return digest(content)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

// names returns the sorted directory entries; a temporary file left behind by a
// commit shows up here.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	list := make([]string, 0, len(entries))
	for _, entry := range entries {
		list = append(list, entry.Name())
	}
	slices.Sort(list)
	return list
}

func wantNames(t *testing.T, dir string, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := names(t, dir); !slices.Equal(got, want) {
		t.Fatalf("directory %s entries = %v, want %v", dir, got, want)
	}
}

func mustNotContain(t *testing.T, text, secret string) {
	t.Helper()
	if strings.Contains(text, secret) {
		t.Fatalf("diagnostic leaked source content %q: %s", secret, text)
	}
}

func TestCommitStaleRevisionChangesNothing(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
	firstRev := writeFile(t, first, "MARKER-FIRST-CONTENT", 0o644)
	writeFile(t, second, "MARKER-SECOND-CONTENT", 0o644)

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: first, Revision: firstRev, Bytes: []byte("replacement one")},
		{Path: second, Revision: digest("some other revision"), Bytes: []byte("replacement two")},
	})
	if !errors.Is(err, ErrRevisionChanged) {
		t.Fatalf("Commit error = %v, want ErrRevisionChanged", err)
	}
	mustNotContain(t, err.Error(), "MARKER-FIRST-CONTENT")
	mustNotContain(t, err.Error(), "MARKER-SECOND-CONTENT")
	if got := readFile(t, first); got != "MARKER-FIRST-CONTENT" {
		t.Fatalf("first file = %q after failed preflight", got)
	}
	if got := readFile(t, second); got != "MARKER-SECOND-CONTENT" {
		t.Fatalf("second file = %q after failed preflight", got)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %d, want 2", len(outcomes))
	}
	for i, outcome := range outcomes {
		if outcome.Committed {
			t.Fatalf("outcome %d committed during a failed preflight", i)
		}
		if !strings.HasPrefix(outcome.Error, "not attempted:") {
			t.Fatalf("outcome %d error = %q, want an unattempted report", i, outcome.Error)
		}
	}
	wantNames(t, dir, "first.txt", "second.txt")
}

func TestCommitReplacementThroughSymlinkPreservesLinkAndMode(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	target, link := filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt")
	revision := writeFile(t, target, "original", 0o640)
	if err := os.Symlink("real.txt", link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: link, Revision: revision, Bytes: []byte("replacement")},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	want := digest("replacement")
	if !outcomes[0].Committed {
		t.Fatalf("outcome not committed: %+v", outcomes[0])
	}
	if outcomes[0].Path != target {
		t.Fatalf("outcome path = %q, want the canonical target %q", outcomes[0].Path, target)
	}
	if outcomes[0].BeforeRevision != revision || outcomes[0].AfterRevision != want {
		t.Fatalf("revisions = %q -> %q, want %q -> %q", outcomes[0].BeforeRevision, outcomes[0].AfterRevision, revision, want)
	}
	if got := readFile(t, target); got != "replacement" {
		t.Fatalf("target content = %q", got)
	}
	if got := fileMode(t, target); got != 0o640 {
		t.Fatalf("target mode = %o, want 640", got)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat link: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replacement replaced the symlink instead of its target")
	}
	if destination, err := os.Readlink(link); err != nil || destination != "real.txt" {
		t.Fatalf("readlink = %q, %v", destination, err)
	}
	wantNames(t, dir, "real.txt", "link.txt")
}

func TestCommitInjectedFailureKeepsFirstCommit(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	first, second, third := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt"), filepath.Join(dir, "third.txt")
	firstRev := writeFile(t, first, "first original", 0o644)
	secondRev := writeFile(t, second, "second original", 0o644)
	thirdRev := writeFile(t, third, "third original", 0o644)

	original := renamePath
	t.Cleanup(func() { renamePath = original })
	calls := 0
	renamePath = func(from, to string) error {
		calls++
		if calls == 2 {
			return errors.New("injected rename failure")
		}
		return os.Rename(from, to)
	}

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: first, Revision: firstRev, Bytes: []byte("first replacement")},
		{Path: second, Revision: secondRev, Bytes: []byte("second replacement")},
		{Path: third, Revision: thirdRev, Bytes: []byte("third replacement")},
	})
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("Commit error = %v, want the injected failure", err)
	}
	if !strings.Contains(err.Error(), "after 1 committed") {
		t.Fatalf("Commit error = %v, want an accurate committed count", err)
	}
	if !outcomes[0].Committed || outcomes[0].Error != "" {
		t.Fatalf("first outcome = %+v, want committed", outcomes[0])
	}
	if outcomes[1].Committed || !strings.Contains(outcomes[1].Error, "injected rename failure") {
		t.Fatalf("second outcome = %+v, want a failed report", outcomes[1])
	}
	if outcomes[2].Committed || !strings.HasPrefix(outcomes[2].Error, "not attempted:") {
		t.Fatalf("third outcome = %+v, want an explicit unattempted report", outcomes[2])
	}
	if got := readFile(t, first); got != "first replacement" {
		t.Fatalf("first file = %q after a partial batch; the commit was rolled back", got)
	}
	if got := readFile(t, second); got != "second original" {
		t.Fatalf("second file = %q", got)
	}
	if got := readFile(t, third); got != "third original" {
		t.Fatalf("third file = %q", got)
	}
	wantNames(t, dir, "first.txt", "second.txt", "third.txt")
}

func TestCommitDestinationExistsFailsPreflight(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	source, destination := filepath.Join(dir, "source.txt"), filepath.Join(dir, "destination.txt")
	revision := writeFile(t, source, "source content", 0o644)
	writeFile(t, destination, "existing content", 0o644)

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: source, Revision: revision, MoveTo: destination},
	})
	if !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("Commit error = %v, want ErrDestinationExists", err)
	}
	if outcomes[0].Committed {
		t.Fatal("existing destination was replaced")
	}
	if got := readFile(t, destination); got != "existing content" {
		t.Fatalf("destination = %q", got)
	}
	if got := readFile(t, source); got != "source content" {
		t.Fatalf("source = %q", got)
	}
}

func TestCommitMoveNeverOverwritesRacingDestination(t *testing.T) {
	original := linkPath
	t.Cleanup(func() { linkPath = original })

	for name, bytes := range map[string][]byte{
		"move keeps bytes":  nil,
		"move writes bytes": []byte("new payload"),
	} {
		t.Run(name, func(t *testing.T) {
			dir, locks := t.TempDir(), lockDir(t)
			source, destination := filepath.Join(dir, "source.txt"), filepath.Join(dir, "destination.txt")
			revision := writeFile(t, source, "source content", 0o644)
			linkPath = func(from, to string) error {
				if err := os.WriteFile(to, []byte("racer content"), 0o644); err != nil {
					return err
				}
				return os.Link(from, to)
			}
			outcomes, err := Commit(context.Background(), locks, []Effect{
				{Path: source, Revision: revision, Bytes: bytes, MoveTo: destination},
			})
			if !errors.Is(err, ErrDestinationExists) {
				t.Fatalf("Commit error = %v, want ErrDestinationExists", err)
			}
			if outcomes[0].Committed {
				t.Fatal("racing destination was overwritten")
			}
			if got := readFile(t, destination); got != "racer content" {
				t.Fatalf("destination = %q, want the racing content", got)
			}
			if got := readFile(t, source); got != "source content" {
				t.Fatalf("source = %q, want it untouched", got)
			}
			wantNames(t, dir, "source.txt", "destination.txt")
		})
	}
}

func TestCommitCancellation(t *testing.T) {
	t.Run("before start", func(t *testing.T) {
		dir, locks := t.TempDir(), lockDir(t)
		path := filepath.Join(dir, "file.txt")
		revision := writeFile(t, path, "content", 0o644)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcomes, err := Commit(ctx, locks, []Effect{{Path: path, Revision: revision, Bytes: []byte("next")}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Commit error = %v, want context.Canceled", err)
		}
		if outcomes[0].Committed || !strings.HasPrefix(outcomes[0].Error, "not attempted:") {
			t.Fatalf("outcome = %+v, want unattempted", outcomes[0])
		}
		if got := readFile(t, path); got != "content" {
			t.Fatalf("file = %q after cancellation", got)
		}
	})

	t.Run("between commits", func(t *testing.T) {
		dir, locks := t.TempDir(), lockDir(t)
		first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
		firstRev := writeFile(t, first, "first original", 0o644)
		secondRev := writeFile(t, second, "second original", 0o644)
		ctx, cancel := context.WithCancel(context.Background())

		original := renamePath
		t.Cleanup(func() { renamePath = original })
		renamePath = func(from, to string) error {
			if err := os.Rename(from, to); err != nil {
				return err
			}
			cancel()
			return nil
		}

		outcomes, err := Commit(ctx, locks, []Effect{
			{Path: first, Revision: firstRev, Bytes: []byte("first replacement")},
			{Path: second, Revision: secondRev, Bytes: []byte("second replacement")},
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Commit error = %v, want context.Canceled", err)
		}
		if !outcomes[0].Committed {
			t.Fatalf("committed write was reported cancelled: %+v", outcomes[0])
		}
		if outcomes[1].Committed || !strings.HasPrefix(outcomes[1].Error, "not attempted:") {
			t.Fatalf("second outcome = %+v, want unattempted", outcomes[1])
		}
		if got := readFile(t, first); got != "first replacement" {
			t.Fatalf("first file = %q", got)
		}
		if got := readFile(t, second); got != "second original" {
			t.Fatalf("second file = %q", got)
		}
		wantNames(t, dir, "first.txt", "second.txt")
	})
}

func TestCommitIdenticalBytesReportsNoOp(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	path := filepath.Join(dir, "file.txt")
	revision := writeFile(t, path, "unchanged", 0o644)
	stamp := time.Unix(1_600_000_000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: path, Revision: revision, Bytes: []byte("unchanged")},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	outcome := outcomes[0]
	if outcome.Committed || outcome.Error != "" {
		t.Fatalf("outcome = %+v, want a silent no-op", outcome)
	}
	if outcome.BeforeRevision != revision || outcome.AfterRevision != revision {
		t.Fatalf("revisions = %q -> %q, want both %q", outcome.BeforeRevision, outcome.AfterRevision, revision)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.ModTime().Equal(stamp) {
		t.Fatalf("no-op rewrote the file: mtime = %v", info.ModTime())
	}
	wantNames(t, dir, "file.txt")
}

func TestCommitRejectsAliasedEffects(t *testing.T) {
	t.Run("symlink and target", func(t *testing.T) {
		dir, locks := t.TempDir(), lockDir(t)
		target, link := filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt")
		revision := writeFile(t, target, "original", 0o644)
		if err := os.Symlink("real.txt", link); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		_, err := Commit(context.Background(), locks, []Effect{
			{Path: link, Revision: revision, Bytes: []byte("one")},
			{Path: target, Revision: revision, Bytes: []byte("two")},
		})
		if !errors.Is(err, ErrAliasConflict) {
			t.Fatalf("Commit error = %v, want ErrAliasConflict", err)
		}
		if got := readFile(t, target); got != "original" {
			t.Fatalf("target = %q", got)
		}
		wantNames(t, dir, "real.txt", "link.txt")
	})

	t.Run("shared destination", func(t *testing.T) {
		dir, locks := t.TempDir(), lockDir(t)
		first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
		destination := filepath.Join(dir, "destination.txt")
		firstRev := writeFile(t, first, "first", 0o644)
		secondRev := writeFile(t, second, "second", 0o644)

		_, err := Commit(context.Background(), locks, []Effect{
			{Path: first, Revision: firstRev, MoveTo: destination},
			{Path: second, Revision: secondRev, MoveTo: destination},
		})
		if !errors.Is(err, ErrAliasConflict) {
			t.Fatalf("Commit error = %v, want ErrAliasConflict", err)
		}
		wantNames(t, dir, "first.txt", "second.txt")
	})
}

func TestCommitDeleteResolvesTarget(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	target, link := filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt")
	revision := writeFile(t, target, "doomed", 0o644)
	if err := os.Symlink("real.txt", link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	outcomes, err := Commit(context.Background(), locks, []Effect{{Path: link, Revision: revision, Delete: true}})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	outcome := outcomes[0]
	if !outcome.Committed || outcome.BeforeRevision != revision || outcome.AfterRevision != "" {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Path != target {
		t.Fatalf("outcome path = %q, want %q", outcome.Path, target)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target still exists: %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("delete removed the symlink entry: %v, %v", info, err)
	}
}

func TestCommitMove(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	source, destination := filepath.Join(dir, "source.txt"), filepath.Join(dir, "destination.txt")
	revision := writeFile(t, source, "payload", 0o640)

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: source, Revision: revision, MoveTo: destination},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	outcome := outcomes[0]
	if !outcome.Committed || outcome.Destination != destination {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.BeforeRevision != revision || outcome.AfterRevision != revision {
		t.Fatalf("revisions = %q -> %q, want unchanged content", outcome.BeforeRevision, outcome.AfterRevision)
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	if got := readFile(t, destination); got != "payload" {
		t.Fatalf("destination = %q", got)
	}
	if got := fileMode(t, destination); got != 0o640 {
		t.Fatalf("destination mode = %o, want 640", got)
	}
	wantNames(t, dir, "destination.txt")
}

func TestCommitMoveWithNewContent(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	source, destination := filepath.Join(dir, "source.txt"), filepath.Join(dir, "destination.txt")
	revision := writeFile(t, source, "payload", 0o640)

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: source, Revision: revision, Bytes: []byte("new payload"), MoveTo: destination},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	outcome := outcomes[0]
	if !outcome.Committed || outcome.AfterRevision != digest("new payload") {
		t.Fatalf("outcome = %+v", outcome)
	}
	if _, err := os.Lstat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still exists: %v", err)
	}
	if got := readFile(t, destination); got != "new payload" {
		t.Fatalf("destination = %q", got)
	}
	if got := fileMode(t, destination); got != 0o640 {
		t.Fatalf("destination mode = %o, want 640", got)
	}
	wantNames(t, dir, "destination.txt")
}

func TestCommitEmptyBatch(t *testing.T) {
	outcomes, err := Commit(context.Background(), lockDir(t), nil)
	if err != nil || len(outcomes) != 0 {
		t.Fatalf("Commit = %v, %v; want no outcomes and no error", outcomes, err)
	}
}

func TestCommitRejectsMalformedEffects(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	path := filepath.Join(dir, "file.txt")
	revision := writeFile(t, path, "content", 0o644)

	for name, effect := range map[string]Effect{
		"missing revision":  {Path: path, Bytes: []byte("next")},
		"no operation":      {Path: path, Revision: revision},
		"delete and move":   {Path: path, Revision: revision, Delete: true, MoveTo: filepath.Join(dir, "other.txt")},
		"delete and write":  {Path: path, Revision: revision, Delete: true, Bytes: []byte("next")},
		"missing source":    {Path: filepath.Join(dir, "absent.txt"), Revision: revision, Bytes: []byte("next")},
		"missing directory": {Path: filepath.Join(dir, "absent", "file.txt"), Revision: revision, Delete: true},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Commit(context.Background(), locks, []Effect{effect}); err == nil {
				t.Fatal("Commit accepted a malformed effect")
			}
		})
	}
	if got := readFile(t, path); got != "content" {
		t.Fatalf("file = %q", got)
	}
	wantNames(t, dir, "file.txt")
}

func TestCommitRejectsNonRegularSource(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	path := filepath.Join(dir, "directory")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Commit(context.Background(), locks, []Effect{
		{Path: path, Revision: digest(""), Delete: true},
	})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Commit error = %v, want a regular-file rejection", err)
	}
}

func TestCommitRejectsSourceReplacedDuringBatch(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
	firstRev := writeFile(t, first, "first original", 0o644)
	secondRev := writeFile(t, second, "second original", 0o644)

	original := renamePath
	t.Cleanup(func() { renamePath = original })
	renamePath = func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		// Same bytes, new inode: the revision still matches but the identity does
		// not, so the next effect must be refused.
		swap := filepath.Join(dir, "swap.txt")
		if err := os.WriteFile(swap, []byte("second original"), 0o644); err != nil {
			return err
		}
		return os.Rename(swap, second)
	}

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: first, Revision: firstRev, Bytes: []byte("first replacement")},
		{Path: second, Revision: secondRev, Bytes: []byte("second replacement")},
	})
	if !errors.Is(err, ErrIdentityChanged) {
		t.Fatalf("Commit error = %v, want ErrIdentityChanged", err)
	}
	if !outcomes[0].Committed {
		t.Fatalf("first outcome = %+v, want committed", outcomes[0])
	}
	if outcomes[1].Committed || !strings.Contains(outcomes[1].Error, ErrIdentityChanged.Error()) {
		t.Fatalf("second outcome = %+v, want an identity failure", outcomes[1])
	}
	if got := readFile(t, second); got != "second original" {
		t.Fatalf("second file = %q", got)
	}
	wantNames(t, dir, "first.txt", "second.txt")
}

func TestCommitKeepsExactBytes(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	path := filepath.Join(dir, "file.txt")
	original := []byte("\xef\xbb\xbfline one\r\nline two\x00\r\n")
	replacement := []byte("\xef\xbb\xbfline one\r\nline two changed\x00\r\n")
	revision := writeFile(t, path, string(original), 0o644)

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: path, Revision: revision, Bytes: replacement},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if outcomes[0].AfterRevision != digest(string(replacement)) {
		t.Fatalf("AfterRevision = %q, want the exact-byte digest %q", outcomes[0].AfterRevision, digest(string(replacement)))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !slices.Equal(data, replacement) {
		t.Fatalf("file bytes = %q, want %q", data, replacement)
	}
}

func TestCommitPartialMoveReportsDestination(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	source, destination := filepath.Join(dir, "source.txt"), filepath.Join(dir, "destination.txt")
	revision := writeFile(t, source, "payload", 0o644)

	original := removePath
	t.Cleanup(func() { removePath = original })
	removePath = func(string) error { return errors.New("injected unlink failure") }

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: source, Revision: revision, MoveTo: destination},
	})
	if err == nil || !strings.Contains(err.Error(), "injected unlink failure") {
		t.Fatalf("Commit error = %v, want the injected failure", err)
	}
	outcome := outcomes[0]
	if !outcome.Committed {
		t.Fatalf("outcome = %+v, want the published destination reported", outcome)
	}
	if outcome.Destination != destination || outcome.AfterRevision != revision {
		t.Fatalf("outcome = %+v, want destination %q at revision %q", outcome, destination, revision)
	}
	if got := readFile(t, destination); got != "payload" {
		t.Fatalf("destination = %q", got)
	}
	if got := readFile(t, source); got != "payload" {
		t.Fatalf("source = %q, want it still present", got)
	}
}

func TestCommitDurabilityErrorKeepsReplacement(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
	firstRev := writeFile(t, first, "first original", 0o644)
	secondRev := writeFile(t, second, "second original", 0o644)

	original := syncDir
	t.Cleanup(func() { syncDir = original })
	syncDir = func(string) error { return errors.New("injected fsync failure") }

	outcomes, err := Commit(context.Background(), locks, []Effect{
		{Path: first, Revision: firstRev, Bytes: []byte("first replacement")},
		{Path: second, Revision: secondRev, Bytes: []byte("second replacement")},
	})
	if err == nil || !strings.Contains(err.Error(), "injected fsync failure") {
		t.Fatalf("Commit error = %v, want the injected durability failure", err)
	}
	if !strings.Contains(err.Error(), "after 1 committed") {
		t.Fatalf("Commit error = %v, want the landed write in the committed count", err)
	}
	outcome := outcomes[0]
	if !outcome.Committed {
		t.Fatalf("outcome = %+v, want the landed replacement reported as committed", outcome)
	}
	if outcome.AfterRevision != digest("first replacement") {
		t.Fatalf("AfterRevision = %q, want the replacement digest", outcome.AfterRevision)
	}
	if !strings.Contains(outcome.Error, "durability") {
		t.Fatalf("outcome error = %q, want a durability report", outcome.Error)
	}
	if got := readFile(t, first); got != "first replacement" {
		t.Fatalf("first file = %q", got)
	}
	if outcomes[1].Committed || !strings.HasPrefix(outcomes[1].Error, "not attempted:") {
		t.Fatalf("second outcome = %+v, want unattempted", outcomes[1])
	}
	if got := readFile(t, second); got != "second original" {
		t.Fatalf("second file = %q", got)
	}
	wantNames(t, dir, "first.txt", "second.txt")
}

type cancelingReader struct {
	cancel context.CancelFunc
	chunk  []byte
	sent   bool
}

func (r *cancelingReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, io.EOF
	}
	r.sent = true
	n := copy(p, r.chunk)
	r.cancel()
	return n, nil
}

func TestDigestObservationHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelingReader{cancel: cancel, chunk: bytes.Repeat([]byte("x"), 32<<10)}

	if _, _, err := digestStream(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("digestStream error = %v, want context.Canceled", err)
	}
}

func TestCommitRejectsRelativePaths(t *testing.T) {
	dir, locks := t.TempDir(), lockDir(t)
	path := filepath.Join(dir, "file.txt")
	revision := writeFile(t, path, "content", 0o644)

	relative, err := filepath.Rel(mustGetwd(t), path)
	if err != nil {
		t.Fatalf("rel: %v", err)
	}
	for name, effects := range map[string][]Effect{
		"relative source": {{Path: relative, Revision: revision, Bytes: []byte("next")}},
		"relative move":   {{Path: path, Revision: revision, MoveTo: relative + ".moved"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Commit(context.Background(), locks, effects); err == nil || !strings.Contains(err.Error(), "must be absolute") {
				t.Fatalf("Commit error = %v, want an absolute-path rejection", err)
			}
		})
	}
	t.Run("relative lock directory", func(t *testing.T) {
		if _, err := Commit(context.Background(), "locks", []Effect{
			{Path: path, Revision: revision, Bytes: []byte("next")},
		}); err == nil || !strings.Contains(err.Error(), "must be absolute") {
			t.Fatalf("Commit error = %v, want an absolute-path rejection", err)
		}
	})
	if got := readFile(t, path); got != "content" {
		t.Fatalf("file = %q", got)
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return wd
}
