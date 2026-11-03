//go:build unix && !linux && !darwin

package jobs

import (
	"errors"
	"testing"
)

func TestUnsupportedWatcherFailsClosed(t *testing.T) {
	_, err := StartProcess(Spec{Command: []string{"true"}})
	if !errors.Is(err, ErrWatcherUnsupported) {
		t.Fatalf("start error = %v, want unsupported watcher", err)
	}
}
