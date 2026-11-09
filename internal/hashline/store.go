package hashline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// StoreLimits bounds source snapshot retention independently of source readability.
type StoreLimits struct {
	Resources, Versions int
	Bytes, FileBytes    int64
}

// Snapshot is an immutable copy of a retained exact source and its current visibility.
type Snapshot struct {
	URI, Revision, Tag, Epoch string
	Raw                       []byte
	Seen                      []LineSpan
	Tagged                    bool
}

type retainedSnapshot struct {
	Snapshot
	used uint64
}

// Store retains exact source versions and epoch-bound model visibility.
type Store struct {
	mu      sync.Mutex
	limits  StoreLimits
	entries map[string][]*retainedSnapshot
	bytes   int64
	clock   uint64
	epoch   string
}

// NewStore creates a bounded cache. Zero limits select the documented defaults.
func NewStore(limits StoreLimits, epoch string) (*Store, error) {
	if limits.Resources == 0 {
		limits.Resources = 256
	}
	if limits.Versions == 0 {
		limits.Versions = 4
	}
	if limits.Bytes == 0 {
		limits.Bytes = 64 << 20
	}
	if limits.FileBytes == 0 {
		limits.FileBytes = 4 << 20
	}
	if limits.Resources < 1 || limits.Versions < 1 || limits.Bytes < 1 || limits.FileBytes < 1 || !validSeenEpoch(epoch) {
		return nil, errors.New("invalid snapshot limits or epoch")
	}
	return &Store{limits: limits, entries: make(map[string][]*retainedSnapshot), epoch: epoch}, nil
}

// SourceRevision is the digest of exact source bytes, before normalization.
func SourceRevision(source []byte) string {
	sum := sha256.Sum256(source)
	return hex.EncodeToString(sum[:])
}

// Retain stores source bytes without granting any seen-line authority.
func (s *Store) Retain(uri string, raw []byte) (Snapshot, error) {
	normalized, err := Normalize(raw)
	if err != nil {
		return Snapshot{}, err
	}
	if uri == "" {
		return Snapshot{}, errors.New("snapshot canonical URI is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Copies and normalized views can coexist while editing; account for both.
	cost := int64(len(raw)) * 2
	if int64(len(raw)) > s.limits.FileBytes || cost > s.limits.Bytes {
		return Snapshot{}, errors.New("source exceeds retained snapshot limit; use exact-text edit or a smaller resource")
	}
	revision := SourceRevision(raw)
	for _, entry := range s.entries[uri] {
		if entry.Revision == revision {
			s.touch(entry)
			return cloneSnapshot(entry.Snapshot), nil
		}
	}
	entry := &retainedSnapshot{Snapshot: Snapshot{URI: uri, Revision: revision, Tag: normalized.Tag, Epoch: s.epoch, Raw: append([]byte(nil), raw...)}}
	s.touch(entry)
	s.entries[uri] = append(s.entries[uri], entry)
	s.bytes += cost
	for len(s.entries[uri]) > s.limits.Versions {
		s.removeOldest(uri)
	}
	for len(s.entries) > s.limits.Resources || s.bytes > s.limits.Bytes {
		s.removeOldest("")
	}
	return cloneSnapshot(entry.Snapshot), nil
}

// Delivered grants visibility only after the caller verifies the exact rendered block
// survived in the model-facing result. Raw source spans count, but do not grant a tag.
func (s *Store) Delivered(uri, revision, epoch string, spans []LineSpan, tagged bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if epoch != s.epoch {
		return &StaleSeenEpochError{}
	}
	for _, entry := range s.entries[uri] {
		if entry.Revision == revision {
			normalized, err := Normalize(entry.Raw)
			if err != nil {
				return err
			}
			for _, span := range spans {
				if span.Start < 1 || span.End < span.Start || span.End > len(normalized.Lines) {
					return errors.New("delivered source interval is outside snapshot")
				}
			}
			if entry.Epoch != epoch {
				entry.Seen = nil
				entry.Tagged = false
				entry.Epoch = epoch
			}
			entry.Seen = mergeSeenSpans(append(append([]LineSpan(nil), entry.Seen...), spans...))
			entry.Tagged = entry.Tagged || tagged
			s.touch(entry)
			return nil
		}
	}
	return errors.New("snapshot was evicted before delivery; read the source again")
}

// Resolve selects the newest retained matching tag within one canonical resource.
// Full source revision and relocation validation remain mandatory before editing.
func (s *Store) Resolve(uri, tag string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.entries[uri]
	for i := len(versions) - 1; i >= 0; i-- {
		entry := versions[i]
		if entry.Tag == tag {
			if entry.Epoch != s.epoch || !entry.Tagged {
				return Snapshot{}, errors.New("tag was not delivered in the active context; read a tagged source view")
			}
			s.touch(entry)
			return cloneSnapshot(entry.Snapshot), nil
		}
	}
	if len(versions) == 0 {
		return Snapshot{}, fmt.Errorf("no retained snapshot of %s; read it before editing with a tag", uri)
	}
	tags := make([]string, len(versions))
	for i, entry := range versions {
		tags[i] = entry.Tag
	}
	return Snapshot{}, fmt.Errorf("tag %s is not a retained version of %s (retained: %s); the file changed or the tag was mistyped, so read %s again and use its current tag", tag, uri, strings.Join(tags, ", "), uri)
}

// Latest returns the newest retained version of one canonical resource without changing its recency.
func (s *Store) Latest(uri string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	versions := s.entries[uri]
	if len(versions) == 0 {
		return Snapshot{}, false
	}
	return cloneSnapshot(versions[len(versions)-1].Snapshot), true
}

// SetEpoch invalidates visibility while retaining source versions for diagnostics.
func (s *Store) SetEpoch(epoch string) error {
	if !validSeenEpoch(epoch) {
		return &StaleSeenEpochError{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch = epoch
	for _, versions := range s.entries {
		for _, entry := range versions {
			entry.Seen = nil
			entry.Tagged = false
			entry.Epoch = epoch
		}
	}
	return nil
}

// Epoch returns the active context epoch.
func (s *Store) Epoch() string { s.mu.Lock(); defer s.mu.Unlock(); return s.epoch }

func (s *Store) touch(entry *retainedSnapshot) { s.clock++; entry.used = s.clock }
func cloneSnapshot(snapshot Snapshot) Snapshot {
	snapshot.Raw = append([]byte(nil), snapshot.Raw...)
	snapshot.Seen = append([]LineSpan(nil), snapshot.Seen...)
	return snapshot
}
func (s *Store) removeOldest(uri string) {
	keys := make([]string, 0, len(s.entries))
	for key := range s.entries {
		if uri == "" || key == uri {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var oldest *retainedSnapshot
	var key string
	index := 0
	for _, candidate := range keys {
		for i, entry := range s.entries[candidate] {
			if oldest == nil || entry.used < oldest.used {
				oldest = entry
				key = candidate
				index = i
			}
		}
	}
	if oldest == nil {
		return
	}
	s.bytes -= int64(len(oldest.Raw)) * 2
	versions := s.entries[key]
	versions = append(versions[:index], versions[index+1:]...)
	if len(versions) == 0 {
		delete(s.entries, key)
	} else {
		s.entries[key] = versions
	}
}
