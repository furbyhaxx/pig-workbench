package messaging

import (
	"sync"
	"time"
)

// Limiter holds the per-peer sliding windows of SPEC-0011 §6.
type Limiter struct {
	mu    sync.Mutex
	rate  map[string][]time.Time
	wakes map[string][]time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter {
	return &Limiter{rate: make(map[string][]time.Time), wakes: make(map[string][]time.Time)}
}

func admit(window map[string][]time.Time, peer string, now time.Time, span time.Duration, limit int) bool {
	kept := window[peer][:0]
	for _, at := range window[peer] {
		if now.Sub(at) < span {
			kept = append(kept, at)
		}
	}
	if len(kept) >= limit {
		window[peer] = kept
		return false
	}
	window[peer] = append(kept, now)
	return true
}

// AllowMessage counts one message from peer against the per-minute limit.
func (l *Limiter) AllowMessage(peer string, now time.Time, perMinute int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return admit(l.rate, peer, now, time.Minute, perMinute)
}

// AllowWake counts one wake for peer against the per-hour budget.
func (l *Limiter) AllowWake(peer string, now time.Time, perHour int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return admit(l.wakes, peer, now, time.Hour, perHour)
}
