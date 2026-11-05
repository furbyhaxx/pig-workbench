package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

// Owner identifies the runtime incarnation that started a job. A job outlives
// its runtime when a reload or a crash skips shutdown cleanup, so the next
// incarnation of the same session is the one that reclaims it.
type Owner struct {
	SessionID  string
	Generation string
}

// Record is one persisted job process identity.
type Record struct {
	JobID      string `json:"jobId"`
	SessionID  string `json:"sessionId"`
	Generation string `json:"generation"`
	PID        int    `json:"pid"`
	Identity   string `json:"identity"`
}

// Registry persists job process identities so a later start can reclaim jobs
// whose owning runtime is gone. Liveness comes from an exclusive lock each
// runtime holds for its whole life: the kernel releases it when the runtime
// process ends, so a crash is indistinguishable from a clean exit here, and a
// live runtime can never have its jobs reclaimed.
type Registry struct {
	dir string
}

// OwnerHandle is a runtime's liveness claim. Closing it (or the process ending)
// marks the generation as gone.
type OwnerHandle struct {
	lock *flock.Flock
}

// HoldOwner claims liveness for one runtime generation.
func (r *Registry) HoldOwner(generation string) (*OwnerHandle, error) {
	if generation == "" {
		return nil, errors.New("owner generation is required")
	}
	lock := flock.New(filepath.Join(r.dir, "owners", generation+".lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, fmt.Errorf("claim owner lock: %w", err)
	}
	if !locked {
		return nil, errors.New("owner generation is already claimed")
	}
	return &OwnerHandle{lock: lock}, nil
}

// Close releases the liveness claim.
func (h *OwnerHandle) Close() error {
	if h == nil || h.lock == nil {
		return nil
	}
	lock := h.lock
	h.lock = nil
	return lock.Unlock()
}

// ownerLive reports whether any runtime still holds the generation's lock.
func (r *Registry) ownerLive(generation string) bool {
	path := filepath.Join(r.dir, "owners", generation+".lock")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	}
	lock := flock.New(path)
	locked, err := lock.TryLock()
	if err != nil || !locked {
		// Unprovable liveness is treated as live so nothing is killed blindly.
		return true
	}
	_ = lock.Unlock()
	return false
}

// SweepResult reports what a sweep reclaimed.
type SweepResult struct {
	// Killed are records whose process group was terminated.
	Killed []Record
	// Removed are records whose process was already gone.
	Removed []Record
	// Retained are records that still belong to the keeping generation.
	Retained []Record
	// Unproven are records whose stale process could not be proven dead or
	// terminated; the records stay so a later sweep can retry.
	Unproven []Record
}

// NewRegistry creates the registry directory with private permissions.
func NewRegistry(dir string) (*Registry, error) {
	if dir == "" {
		return nil, errors.New("job registry directory is required")
	}
	if err := os.MkdirAll(filepath.Join(dir, "owners"), 0o700); err != nil {
		return nil, fmt.Errorf("create job registry: %w", err)
	}
	return &Registry{dir: dir}, nil
}

// Add records one started job. A failure is not fatal to the job itself: the
// caller reports it, and the job stays correct for this runtime.
func (r *Registry) Add(record Record) error {
	if record.JobID == "" || record.SessionID == "" || record.Generation == "" || record.PID <= 0 {
		return errors.New("job registry record needs a job, session, generation and pid")
	}
	if strings.ContainsAny(record.JobID, `/\`) || strings.Contains(record.JobID, "..") {
		return errors.New("job registry record has an invalid job id")
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode job registry record: %w", err)
	}
	path := r.path(record.SessionID, record.JobID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create job registry session directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write job registry record: %w", err)
	}
	return nil
}

// Remove drops one record after its job was observed complete.
func (r *Registry) Remove(sessionID, jobID string) error {
	if err := os.Remove(r.path(sessionID, jobID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove job registry record: %w", err)
	}
	return nil
}

// Sweep reclaims records whose owning runtime is gone: those process groups are
// terminated and the records dropped. Records of a live runtime are retained,
// whichever session they belong to.
func (r *Registry) Sweep(Owner) (SweepResult, error) {
	result := SweepResult{}
	sessions, err := os.ReadDir(r.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		return result, fmt.Errorf("read job registry: %w", err)
	}
	var failures []error
	for _, session := range sessions {
		if !session.IsDir() {
			continue
		}
		sessionID := session.Name()
		entries, err := os.ReadDir(filepath.Join(r.dir, sessionID))
		if err != nil {
			failures = append(failures, fmt.Errorf("read job registry session %s: %w", sessionID, err))
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			path := filepath.Join(r.dir, sessionID, entry.Name())
			record, err := readRecord(path)
			if err != nil {
				failures = append(failures, err)
				if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					failures = append(failures, fmt.Errorf("drop unreadable job registry record: %w", removeErr))
				}
				continue
			}
			switch {
			case !remnantAlive(record):
				result.Removed = append(result.Removed, record)
			case r.ownerLive(record.Generation):
				result.Retained = append(result.Retained, record)
				continue
			default:
				if err := killRemnantGroup(record); err != nil {
					result.Unproven = append(result.Unproven, record)
					failures = append(failures, err)
					continue
				}
				result.Killed = append(result.Killed, record)
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Errorf("remove reclaimed job registry record: %w", err))
			}
		}
	}
	return result, errors.Join(failures...)
}

func (r *Registry) path(sessionID, jobID string) string {
	return filepath.Join(r.dir, sessionID, jobID+".json")
}

func readRecord(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read job registry record %s: %w", filepath.Base(path), err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode job registry record %s: %w", filepath.Base(path), err)
	}
	if record.JobID == "" || record.PID <= 0 {
		return Record{}, fmt.Errorf("job registry record %s is incomplete", filepath.Base(path))
	}
	return record, nil
}
