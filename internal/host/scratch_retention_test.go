package host

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScratchSweepRemovesOnlyOldUnclaimedDirectories(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.Add(-8 * 24 * time.Hour)
	make := func(name string, at time.Time, withArgs bool) string {
		t.Helper()
		dir := filepath.Join(root, name)
		paths := []string{dir}
		if withArgs {
			paths = append(paths, materializedRoot(dir))
		}
		for _, path := range paths {
			if err := os.MkdirAll(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(path, "note"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(path, at, at); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	exists := func(path string) bool { _, err := os.Lstat(path); return err == nil }

	finished := make("finished", old, true)
	recent := make("recent", now.Add(-time.Hour), false)
	own := make("own", old, false)
	live := make("live", old, false)
	claim, err := claimScratch(live)
	if err != nil {
		t.Fatal(err)
	}
	// A long-running session's files are as old as its start; only the claim protects it.
	for _, path := range []string{live, live + ".lock"} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	if removed := sweepScratch(t.Context(), root, "own", 7*24*time.Hour, now); removed != 1 {
		t.Fatalf("removed %d directories, want exactly the finished one", removed)
	}
	if exists(finished) || exists(materializedRoot(finished)) || exists(finished+".lock") {
		t.Fatal("the finished session's scratch, arguments or lock file survived")
	}
	for name, path := range map[string]string{"recent": recent, "own": own, "live": live} {
		if !exists(filepath.Join(path, "note")) {
			t.Fatalf("the %s scratch directory was removed", name)
		}
	}

	if err := claim.Close(); err != nil {
		t.Fatal(err)
	}
	if removed := sweepScratch(t.Context(), root, "own", 7*24*time.Hour, now); removed != 1 || exists(live) {
		t.Fatalf("a released old directory must go at the next sweep: removed=%d", removed)
	}
	if removed := sweepScratch(t.Context(), root, "own", 0, now); removed != 1 || exists(recent) || !exists(own) {
		t.Fatalf("retention 0 must remove every unclaimed directory but this session's: removed=%d", removed)
	}
}

func TestScratchClaimIsSharedAndRecreatesTheDirectory(t *testing.T) {
	scratch := filepath.Join(t.TempDir(), "nosession")
	first, err := claimScratch(scratch)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := claimScratch(scratch)
	if err != nil {
		t.Fatalf("two sessions sharing one scratch directory must both claim it: %v", err)
	}
	defer second.Close()
	if info, err := os.Stat(scratch); err != nil || !info.IsDir() {
		t.Fatal("claiming did not create the scratch directory")
	}
	if removed := sweepScratch(t.Context(), filepath.Dir(scratch), "", 0, time.Now()); removed != 0 {
		t.Fatal("a claimed directory was swept")
	}
}
