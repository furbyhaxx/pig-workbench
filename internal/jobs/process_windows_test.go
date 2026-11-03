package jobs

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestWindowsProcessGroupUnsupported(t *testing.T) {
	if os.Getenv("PIG_JOB_TEST_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	p, err := StartProcess(Spec{
		Command: []string{os.Args[0], "-test.run=^TestWindowsProcessGroupUnsupported$"},
		Env:     append(os.Environ(), "PIG_JOB_TEST_CHILD=1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Kill(0); !errors.Is(err, ErrGroupKillUnsupported) {
		t.Fatalf("kill error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := p.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.Close(); !errors.Is(err, ErrGroupKillUnsupported) {
		t.Fatalf("close error = %v", err)
	}
}
