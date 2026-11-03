package endpoint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsFailsClosedWithoutACLValidation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "registry")
	_, err := Open(dir)
	var unsupported *PlatformUnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Feature != "endpoint registry" || unsupported.Missing != "ACL-based private-access validation" {
		t.Fatalf("registry did not report unsupported privacy validation: %v", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported registry created a directory: %v", err)
	}
	_, err = pidAlive(1)
	var probe *PIDProbeUnsupportedError
	if !errors.As(err, &probe) {
		t.Fatalf("unsupported PID probe not typed: %v", err)
	}
}
