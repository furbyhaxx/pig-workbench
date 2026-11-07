package host

import (
	"crypto/sha256"
	"encoding/json"
	"sync"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

const repetitionHint = "\nRepeated read: unchanged resource, revision and selection. Use a different range/view or the continuation in Details if you need more.\n"

const readHintReserve = len(repetitionHint)

type readCounter struct {
	signature [32]byte
	count     int
	sequence  uint64
}
type readHistory struct {
	mu       sync.Mutex
	entries  map[string]readCounter
	sequence uint64
}

func (h *readHistory) hint(target resource.Target, page resource.ReadResult) string {
	if page.Facts.Revision == "" || page.Facts.CanonicalURI == "" {
		return ""
	}
	selection := target.Selection
	if spans, err := selection.Resolve(page.Facts.TotalLines, false); err == nil {
		selection.Ranges = spans
		selection.Tails = nil
		selection.Explicit = false
	}
	encoded, _ := json.Marshal(struct {
		Revision, View, Handler string
		Selection               resource.Selection
		Content                 [32]byte
	}{page.Facts.Revision, page.Facts.CoordinateSpace, page.Facts.Handler, selection, sha256.Sum256([]byte(page.Content))})
	signature := sha256.Sum256(encoded)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.entries == nil {
		h.entries = make(map[string]readCounter)
	}
	uri := page.Facts.CanonicalURI
	prior, exists := h.entries[uri]
	if !exists && len(h.entries) >= 1024 {
		oldest := ""
		sequence := h.sequence
		for key, entry := range h.entries {
			if oldest == "" || entry.sequence < sequence {
				oldest, sequence = key, entry.sequence
			}
		}
		delete(h.entries, oldest)
	}
	h.sequence++
	count := 1
	if prior.signature == signature {
		count = min(3, prior.count+1)
	}
	h.entries[uri] = readCounter{signature: signature, count: count, sequence: h.sequence}
	if count < 3 {
		return ""
	}
	return repetitionHint
}
