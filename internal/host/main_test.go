package host

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain keeps every in-process test away from the developer's real secret vault: the vault lives
// under the global data directory while the key lives in the agent directory, so a stray write
// there plants records the real key cannot read.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "workbench-host-test-")
	if err != nil {
		panic(err)
	}
	data := filepath.Join(root, "data")
	os.Setenv("XDG_DATA_HOME", data)
	os.Setenv("LOCALAPPDATA", data)
	code := m.Run()
	os.RemoveAll(root)
	os.Exit(code)
}
