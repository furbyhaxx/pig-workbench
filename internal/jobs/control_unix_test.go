//go:build unix

package jobs

import (
	"context"
	"testing"
	"time"
)

func TestManagerForwardsStdinAndEOF(t *testing.T) {
	manager, err := NewManager(ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := manager.Start(ctx, Spec{Command: []string{"cat"}, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := manager.Write(id, []byte("first\nsecond\n")); err != nil || n != 13 {
		t.Fatalf("write: %d %v", n, err)
	}
	if err := manager.EOF(id); err != nil {
		t.Fatal(err)
	}
	state, err := manager.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if state.Exit == nil || state.Exit.Code != 0 || state.Running {
		t.Fatalf("unexpected state: %+v", state)
	}
	output, _, _, _, err := manager.Output(id, 0, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "first\nsecond\n" {
		t.Fatalf("lost input: %q", output)
	}
}
