package host

import (
	"crypto/sha256"
	"encoding/json"
	"sort"
	"sync"

	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

type presentationCallKey struct{ Owner, Key, CallID string }
type presentationRecord struct {
	tool       string
	snapshot   presentation.Snapshot
	valid      bool
	final      bool
	digest     [32]byte
	diagnostic string
	bytes      int
	sequence   uint64
}
type presentationCache struct {
	mu       sync.RWMutex
	records  map[presentationCallKey]presentationRecord
	bytes    int
	sequence uint64
}

func (c *presentationCache) observe(key presentationCallKey, tool string, descriptor presentation.Descriptor, details any, final bool) {
	snapshot, err := decodeToolSnapshot(descriptor, details)
	encoded, _ := json.Marshal(snapshot)
	digest := sha256.Sum256(encoded)
	valid := err == nil && (!final || snapshot.Phase == presentation.PhaseSettled)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.records == nil {
		c.records = make(map[presentationCallKey]presentationRecord)
	}
	previous, exists := c.records[key]
	if exists && previous.final && snapshot.Phase != presentation.PhaseSettled {
		return
	}
	if valid && exists && previous.valid {
		if snapshot.Revision < previous.snapshot.Revision {
			return
		}
		if previous.snapshot.Phase == presentation.PhaseSettled {
			if snapshot.Phase != presentation.PhaseSettled {
				return
			}
			if snapshot.Revision == previous.snapshot.Revision && digest == previous.digest {
				return
			}
			previous.diagnostic = "conflicting final presentation ignored; retained the settled outcome"
			c.sequence++
			previous.sequence = c.sequence
			c.records[key] = previous
			return
		}
		if snapshot.Revision == previous.snapshot.Revision {
			if digest != previous.digest {
				previous.diagnostic = "conflicting presentation revision ignored"
				c.sequence++
				previous.sequence = c.sequence
				c.records[key] = previous
			}
			return
		}
		if previous.snapshot.Phase == presentation.PhaseRunning && snapshot.Phase == presentation.PhasePreparing {
			return
		}
	}
	if !valid && exists && previous.valid {
		previous.diagnostic = "invalid or nonterminal final presentation ignored; retained validated facts"
		previous.final = previous.final || final
		if final && previous.snapshot.Phase != presentation.PhaseSettled {
			previous.valid = false
		}
		c.sequence++
		previous.sequence = c.sequence
		c.records[key] = previous
		return
	}
	record := presentationRecord{tool: tool, snapshot: snapshot, valid: valid, final: final || snapshot.Phase == presentation.PhaseSettled, digest: digest, bytes: len(encoded) + len(key.Owner) + len(key.Key) + len(key.CallID)}
	if !valid {
		record.diagnostic = "presentation unavailable: invalid, unsupported or nonterminal final snapshot"
	}
	if exists {
		delete(c.records, key)
		c.bytes -= previous.bytes
	}
	for len(c.records) >= 256 || c.bytes+record.bytes > 8<<20 {
		var oldest presentationCallKey
		sequence := c.sequence
		found := false
		for candidate, value := range c.records {
			if !found || value.sequence < sequence {
				oldest, sequence, found = candidate, value.sequence, true
			}
		}
		if !found {
			return
		}
		prior := c.records[oldest]
		delete(c.records, oldest)
		c.bytes -= prior.bytes
	}
	c.sequence++
	record.sequence = c.sequence
	c.records[key] = record
	c.bytes += record.bytes
}

type presentationCallSummary struct {
	Owner, Key, Tool, CallID, Phase, Outcome, Diagnostic string
	Revision                                             int64
	sequence                                             uint64
}

func (c *presentationCache) summaries(owner string) []presentationCallSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var result []presentationCallSummary
	for key, record := range c.records {
		if owner != "" && owner != key.Owner {
			continue
		}
		phase := string(record.snapshot.Phase)
		if !record.valid {
			phase = "unavailable"
		}
		outcome := ""
		if record.valid && record.snapshot.Outcome != nil {
			outcome = string(*record.snapshot.Outcome)
		}
		result = append(result, presentationCallSummary{Outcome: outcome, Owner: key.Owner, Key: key.Key, Tool: record.tool, CallID: key.CallID, Phase: phase, Revision: record.snapshot.Revision, Diagnostic: record.diagnostic, sequence: record.sequence})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].sequence > result[j].sequence })
	if len(result) > 32 {
		result = result[:32]
	}
	return result
}

func (c *presentationCache) lookup(key presentationCallKey) (presentationRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	record, ok := c.records[key]
	return record, ok
}
