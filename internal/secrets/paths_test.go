package secrets

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestVaultUsesPrivateDataRoot(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Darwin uses platform application support path")
	}
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", root)
	} else {
		t.Setenv("XDG_DATA_HOME", root)
	}
	path, err := VaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(root, "pig-workbench", "secrets", "vault.jsonl") {
		t.Fatalf("wrong private data path: %s", path)
	}
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", "relative")
	} else {
		t.Setenv("XDG_DATA_HOME", "relative")
	}
	if _, err := VaultPath(); err == nil {
		t.Fatal("relative data root accepted")
	}
}
