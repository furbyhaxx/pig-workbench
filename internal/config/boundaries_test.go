package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCancelledRuntimeSaveAndRetargetedSymlink(t *testing.T) {
	m, paths := setup(t, "[read]\nmax-lines = 100\n", "")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	base := current(t, m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Save(ctx, Runtime, base, []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled save applied: %v", err)
	}
	if current(t, m) != base {
		t.Fatal("cancelled save changed active snapshot")
	}
	target := filepath.Join(filepath.Dir(paths.User), "other.toml")
	original := "[read]\nmax-lines = 100\n"
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(paths.User); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, paths.User); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(context.Background(), User, base, []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil); err == nil {
		t.Fatal("changed canonical target accepted against old approval")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatal("new target changed before reauthorization")
	}
}

func TestQuotedDottedKeyCannotActivateKnownSetting(t *testing.T) {
	m, _ := setup(t, `"read.max-lines" = 100
[future]
data = "keep"
`, "")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if current(t, m).Read().MaxLines != 2000 {
		t.Fatal("quoted literal key was interpreted as a nested setting")
	}
	if len(current(t, m).Warnings()) != 2 {
		t.Fatal("unknown keys were hidden")
	}
}
