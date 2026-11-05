package location

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreCannotActivateAfterLookupCancels(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	var cancelLookup context.CancelFunc
	manager, err := New(context.Background(), root, func(context.Context, string) (string, error) {
		if cancelLookup != nil {
			cancelLookup()
		}
		return root, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	original := manager.Current()
	restored := original
	restored.Cwd = child
	restored.Revision++
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelLookup = cancel
	if err := manager.RestoreContext(ctx, restored); !errors.Is(err, context.Canceled) {
		t.Fatalf("late cancellation ignored: %v", err)
	}
	if manager.Current() != original {
		t.Fatal("cancelled restore activated state")
	}
}

func TestProjectLookupMustReturnAnAbsoluteRoot(t *testing.T) {
	if _, err := New(context.Background(), t.TempDir(), func(context.Context, string) (string, error) { return "", nil }); err == nil {
		t.Fatal("empty lookup result silently became a project root")
	}
}
