package fileio

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteAtomicReplacesWithTheRequestedModeAndLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	replaced, err := WriteAtomic(context.Background(), path, []byte("new"), 0o600)
	if err != nil || !replaced {
		t.Fatalf("WriteAtomic = %v, %v", replaced, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "new" {
		t.Fatalf("content = %q, %v", data, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory holds %d entries, %v", len(entries), err)
	}
}

func TestWriteAtomicKeepsTheOldFileWhenCancelledOrBlocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if replaced, err := WriteAtomic(cancelled, path, []byte("new"), 0o600); err == nil || replaced {
		t.Fatalf("cancelled write = %v, %v", replaced, err)
	}
	blocked := filepath.Join(dir, "directory")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "child"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if replaced, err := WriteAtomic(context.Background(), blocked, []byte("new"), 0o600); err == nil || replaced {
		t.Fatalf("write over a non-empty directory = %v, %v", replaced, err)
	}
	data, _ := os.ReadFile(path)
	entries, _ := os.ReadDir(dir)
	if string(data) != "old" || len(entries) != 2 {
		t.Fatalf("old content %q, %d entries left", data, len(entries))
	}
}
