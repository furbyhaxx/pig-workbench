package host

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/fileedit"
)

func TestAtomicWriteFileRefusesRacingDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "created.txt")
	if err := os.WriteFile(path, []byte("concurrent writer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	landed, err := atomicWriteFile(context.Background(), path, []byte("replacement\n"), 0o600)
	if landed || !errors.Is(err, fileedit.ErrDestinationExists) {
		t.Fatalf("got %v; want destination-exists refusal", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "concurrent writer\n" {
		t.Fatalf("racing destination was overwritten: %q", content)
	}
}
