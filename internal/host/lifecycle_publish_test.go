package host

import (
	"sync"
	"testing"
)

// TestPublishedSessionAlreadyHasItsEndpointAndActivity runs under the race gate: handlers read
// these fields without a lock, so they must be assigned before the session becomes visible.
func TestPublishedSessionAlreadyHasItsEndpointAndActivity(t *testing.T) {
	for range 200 {
		a := &app{}
		s := &session{}
		stop := make(chan struct{})
		var readers sync.WaitGroup
		for range 4 {
			readers.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					if seen, err := a.current(); err == nil && (seen.endpoint == nil || seen.activity == nil) {
						t.Error("a handler saw a published session without its endpoint or activity line")
						return
					}
				}
			})
		}
		a.publish(s, func() {
			s.endpoint = &sessionEndpoint{}
			s.activity = &activityLine{}
		})
		close(stop)
		readers.Wait()
	}
}

func TestCleanupStackRunsNewestFirstAndOnlyOnce(t *testing.T) {
	var order []int
	var stack cleanupStack
	for i := range 3 {
		stack.push(func() { order = append(order, i) })
	}
	stack.run()
	stack.run()
	if len(order) != 3 || order[0] != 2 || order[2] != 0 {
		t.Fatalf("cleanup order %v, want [2 1 0] once", order)
	}
}

func TestDetachedWorkEndsQuietlyOnlyWhenItsContextIsStale(t *testing.T) {
	if !detachedWhile(func() bool { return false }, func() { panic("stale context") }) {
		t.Fatal("work that panicked on a stale context was not reported as ended")
	}
	if detachedWhile(func() bool { return true }, func() {}) {
		t.Fatal("finished work was reported as stale")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a panic with a live context was swallowed")
		}
	}()
	detachedWhile(func() bool { return true }, func() { panic("defect") })
}
