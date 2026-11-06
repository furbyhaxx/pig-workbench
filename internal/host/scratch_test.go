package host

import (
	"path/filepath"
	"testing"
)

func TestSessionScratchKeysDoNotCollide(t *testing.T) {
	a := sanitizeSegment("session/one")
	b := sanitizeSegment("session/two")
	if a == b {
		t.Fatal("distinct session ids collapsed to one scratch segment")
	}
	root := t.TempDir()
	left := filepath.Join(root, a)
	right := filepath.Join(root, b)
	if left == right {
		t.Fatal("scratch paths are identical")
	}
}
