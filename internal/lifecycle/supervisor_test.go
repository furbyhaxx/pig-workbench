package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStopCancelsAndDrains(t *testing.T) {
	s := New()
	exited := make(chan struct{})
	if err := s.Start(func(ctx context.Context) { <-ctx.Done(); close(exited) }); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	select {
	case <-exited:
	default:
		t.Fatal("Stop returned before worker exited")
	}
	if _, _, err := s.Admit(context.Background()); !errors.Is(err, ErrClosed) {
		t.Fatalf("admitted after shutdown: %v", err)
	}
	s.Stop()
}

func TestRequestCancellationAndRelease(t *testing.T) {
	s := New()
	parent, cancel := context.WithCancel(context.Background())
	ctx, release, err := s.Admit(parent)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancellation lost")
	}
	release()
	release()
	s.Stop()
}
