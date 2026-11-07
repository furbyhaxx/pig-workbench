package host

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

type cursorBinding struct{ Registration, Descriptor, Generation, Instance string }
type listingCursor struct {
	Binding                             cursorBinding
	Requested, Canonical, Raw, Revision string
	created                             time.Time
	sequence                            uint64
	bytes                               int
}
type listingCursorStore struct {
	mu          sync.Mutex
	key         [32]byte
	initialized bool
	entries     map[string]listingCursor
	bytes       int
	sequence    uint64
}

func (s *listingCursorStore) put(entry listingCursor, now time.Time) (string, error) {
	size := len(entry.Requested) + len(entry.Canonical) + len(entry.Raw) + len(entry.Revision) + len(entry.Binding.Registration) + len(entry.Binding.Descriptor) + len(entry.Binding.Generation) + len(entry.Binding.Instance)
	if size > 1<<20 {
		return "", errors.New("limitExceeded: listing cursor exceeds the 1 MiB retention budget")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		if _, err := rand.Read(s.key[:]); err != nil {
			return "", err
		}
		s.initialized = true
		s.entries = make(map[string]listingCursor)
	}
	// Stable opaque handles expose no backend payload and preserve repeated-cursor detection.
	encoded, _ := json.Marshal(entry)
	digest := hmac.New(sha256.New, s.key[:])
	_, _ = digest.Write(encoded)
	token := "wb-list-" + hex.EncodeToString(digest.Sum(nil))
	for key, prior := range s.entries {
		if key == token || now.Sub(prior.created) >= 5*time.Minute {
			delete(s.entries, key)
			s.bytes -= prior.bytes
		}
	}
	for len(s.entries) >= 256 || s.bytes+size > 1<<20 {
		oldest := ""
		sequence := s.sequence
		for key, prior := range s.entries {
			if oldest == "" || prior.sequence < sequence {
				oldest, sequence = key, prior.sequence
			}
		}
		prior := s.entries[oldest]
		delete(s.entries, oldest)
		s.bytes -= prior.bytes
	}
	s.sequence++
	entry.created, entry.sequence, entry.bytes = now, s.sequence, size
	s.entries[token] = entry
	s.bytes += size
	return token, nil
}

func (s *listingCursorStore) get(token string, binding cursorBinding, uri string, now time.Time) (listingCursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, exists := s.entries[token]
	if exists && now.Sub(entry.created) >= 5*time.Minute {
		delete(s.entries, token)
		s.bytes -= entry.bytes
		exists = false
	}
	if !exists || entry.Binding != binding || uri != entry.Requested && uri != entry.Canonical {
		return listingCursor{}, errors.New("staleRevision: listing cursor expired or belongs to another resource or registration; restart listing")
	}
	s.sequence++
	entry.sequence = s.sequence
	s.entries[token] = entry
	return entry, nil
}
