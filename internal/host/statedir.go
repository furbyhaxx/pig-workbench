package host

import (
	"os"
	"path/filepath"
)

// stateDir returns the per-user XDG state directory. Session-scoped records that
// must outlive one runtime (job process identities) live here, not in the cache
// and not in the agent configuration directory.
func stateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "pig-workbench"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "pig-workbench"), nil
}

// jobRegistryBase is the directory holding one subdirectory per session; the
// registry itself creates and sweeps the session directories.
func jobRegistryBase() (string, error) {
	base, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "jobs"), nil
}
