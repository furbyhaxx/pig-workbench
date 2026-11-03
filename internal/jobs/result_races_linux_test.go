package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitUsesResultWhenCloseFinishesBeforeObservation(t *testing.T) {
	failWatcher := injectWatchFailure(t)
	process, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	entered, release := holdPassiveObservation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		exit Exit
		err  error
	}
	done := make(chan outcome, 1)
	defer func() { release(); _ = process.Close() }()
	failWatcher()
	go func() { exit, err := process.Wait(ctx); done <- outcome{exit, err} }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	closeErr := process.Close()
	release()
	result := <-done
	if !errors.Is(closeErr, errFixtureWatcher) {
		t.Fatalf("Close lost watcher error: %v", closeErr)
	}
	expected, known := process.terminalExit()
	if !known || result.exit != *expected {
		t.Fatalf("Wait lost recorded exit: %+v, %v; expected %+v", result.exit, result.err, expected)
	}
	if !errors.Is(result.err, errFixtureWatcher) || errors.Is(result.err, ErrProcessIdentityUnsupported) {
		t.Fatalf("Wait returned obsolete observation error: %v", result.err)
	}
}

func TestClosePreservesWatcherErrorAfterWaitRecovery(t *testing.T) {
	failWatcher := injectWatchFailure(t)
	process, err := StartProcess(Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	failWatcher()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := process.Wait(ctx)
	if exit.Code != 0 || !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("Wait: %+v %v", exit, err)
	}
	if err := process.Close(); !errors.Is(err, errFixtureWatcher) {
		t.Fatalf("Close lost recorded watcher failure: %v", err)
	}
}
