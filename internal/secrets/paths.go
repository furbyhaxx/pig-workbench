package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// VaultPath returns the platform data path, never the agent configuration directory.
func VaultPath() (string, error) {
	var data string
	switch runtime.GOOS {
	case "windows":
		data = os.Getenv("LOCALAPPDATA")
		if data == "" {
			return "", errors.New("LOCALAPPDATA is unavailable for private secret storage")
		}
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		data = filepath.Join(home, "Library", "Application Support")
	default:
		data = os.Getenv("XDG_DATA_HOME")
		if data == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			data = filepath.Join(home, ".local", "share")
		}
	}
	if !filepath.IsAbs(data) {
		return "", errors.New("secret data directory must be absolute")
	}
	return filepath.Join(data, "pig-workbench", "secrets", "vault.jsonl"), nil
}
