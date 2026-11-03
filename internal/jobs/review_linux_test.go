package jobs

import (
	"context"
	"sync"
	"testing"
	"time"
)

// holdPassiveObservation replaces the identity observation so a test knows a pending Wait is
// inside it. The returned function releases it; the observation then waits for real evidence.
func holdPassiveObservation(t *testing.T) (entered <-chan struct{}, release func()) {
	t.Helper()
	previous := observeIdentityExit
	started := make(chan struct{})
	open := make(chan struct{})
	var once sync.Once
	observeIdentityExit = func(owner *groupOwner, ctx context.Context) error {
		once.Do(func() { close(started) })
		select {
		case <-open:
		case <-ctx.Done():
			return ctx.Err()
		}
		return owner.observeExit(ctx)
	}
	var released sync.Once
	t.Cleanup(func() {
		released.Do(func() { close(open) })
		observeIdentityExit = previous
	})
	return started, func() { released.Do(func() { close(open) }) }
}

// TestReviewPendingWaitMustNotBlockKillAfterWatcherFailure is the regression for a pending Wait
// that held the signal/reap gate while it waited for a live child: cleanup then had to wait for
// the wait's context to be cancelled.
func TestReviewPendingWaitMustNotBlockKillAfterWatcherFailure(t *testing.T) {
	failObservation := injectWatchFailure(t)
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	entered, releaseObservation := holdPassiveObservation(t)
	ctx, cancel := context.WithCancel(context.Background())
	waitDone := make(chan struct{})
	killDone := make(chan error, 1)
	defer func() {
		cancel()
		releaseObservation()
		<-waitDone
		_ = process.Close()
	}()
	failObservation()
	go func() { defer close(waitDone); _, _ = process.Wait(ctx) }()
	<-entered
	if !process.gate.TryLock() {
		t.Fatal("passive observation holds the signal/reap gate")
	}
	process.gate.Unlock()
	go func() { killDone <- process.Kill(0) }()
	select {
	case <-killDone:
	case <-time.After(500 * time.Millisecond):
		cancel()
		<-killDone
		t.Fatal("Kill blocked behind live-child observation until Wait was cancelled")
	}
	select {
	case <-waitDone:
		t.Fatal("Wait returned before its context was cancelled")
	default:
	}
}
