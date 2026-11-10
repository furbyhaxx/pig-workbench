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
