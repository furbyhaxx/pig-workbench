package endpoint

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

func sameToken(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Register publishes a record unless another live or unverified owner holds its ID.
func (r *Registry) Register(ctx context.Context, record Record) error {
	data, err := encodeRecord(record)
	if err != nil {
		return err
	}
	path, err := r.recordPath(record.SessionID)
	if err != nil {
		return err
	}
	lock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	previous, err := readRecord(path, record.SessionID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil && (previous.PID != record.PID || !sameToken(previous.Token, record.Token)) {
		alive, err := pidAlive(previous.PID)
		if err != nil {
			return fmt.Errorf("check existing endpoint liveness: %w", err)
		}
		if alive {
			return ErrSessionExists
		}
	}
	return writeRecord(ctx, path, data)
}

// Heartbeat updates only HeartbeatAt after authenticating the session token.
func (r *Registry) Heartbeat(ctx context.Context, sessionID, token string) error {
	path, err := r.recordPath(sessionID)
	if err != nil {
		return err
	}
	lock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	record, err := readRecord(path, sessionID)
	if err != nil {
		return err
	}
	if !sameToken(record.Token, token) {
		return ErrTokenMismatch
	}
	record.HeartbeatAt = time.Now().UTC()
	data, err := encodeRecord(record)
	if err != nil {
		return err
	}
	return writeRecord(ctx, path, data)
}

// Unregister removes the owner's record, or succeeds if it is absent.
func (r *Registry) Unregister(ctx context.Context, sessionID, token string) error {
	path, err := r.recordPath(sessionID)
	if err != nil {
		return err
	}
	lock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	record, err := readRecord(path, sessionID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !sameToken(record.Token, token) {
		return ErrTokenMismatch
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("remove endpoint record: %w", err)
	}
	if err := syncDirectory(r.dir); err != nil {
		return fmt.Errorf("endpoint record removed but directory durability is uncertain: %w", err)
	}
	return nil
}

// Lookup returns a valid record, regardless of whether its PID is live.
func (r *Registry) Lookup(ctx context.Context, sessionID string) (Record, error) {
	path, err := r.recordPath(sessionID)
	if err != nil {
		return Record{}, err
	}
	lock, err := r.lock(ctx)
	if err != nil {
		return Record{}, err
	}
	defer lock.Close()
	return readRecord(path, sessionID)
}

// List returns valid records and reports malformed, inaccessible or stale entries.
func (r *Registry) List(ctx context.Context) (ListResult, error) {
	var result ListResult
	lock, err := r.lock(ctx)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	return r.listLocked(ctx)
}

func (r *Registry) listLocked(ctx context.Context) (ListResult, error) {
	var result ListResult
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return result, fmt.Errorf("list endpoint registry: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		path, err := r.recordPath(id)
		if err != nil {
			result.Problems = append(result.Problems, recordProblem(name, err))
			continue
		}
		record, err := readRecord(path, id)
		if err != nil {
			result.Problems = append(result.Problems, recordProblem(name, err))
			continue
		}
		result.Records = append(result.Records, record)
		alive, err := pidAlive(record.PID)
		if err != nil {
			result.Problems = append(result.Problems, recordProblem(name, err))
		} else if !alive {
			result.Stale = append(result.Stale, record.SessionID)
		}
	}
	return result, nil
}

// Prune removes PID-stale records whose heartbeat is older than olderThan.
// Before automatic cleanup, the transport must also confirm the socket is unreachable.
// Partial failures are returned as ProblemsError together with any removed IDs.
func (r *Registry) Prune(ctx context.Context, olderThan time.Duration) (removed []string, err error) {
	return r.PruneUnreachable(ctx, olderThan, nil)
}

// PruneUnreachable is Prune with a liveness probe: a PID-stale record is kept while answers reports true,
// so a record is removed only when its process is gone and its endpoint no longer responds.
func (r *Registry) PruneUnreachable(ctx context.Context, olderThan time.Duration, answers func(Record) bool) (removed []string, err error) {
	if olderThan < 0 {
		return nil, errors.New("endpoint prune age must not be negative")
	}
	lock, err := r.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	result, err := r.listLocked(ctx)
	if err != nil {
		return nil, err
	}
	problems := result.Problems
	stale := make(map[string]bool, len(result.Stale))
	for _, id := range result.Stale {
		stale[id] = true
	}
	now := time.Now()
	for _, record := range result.Records {
		if err = ctx.Err(); err != nil {
			break
		}
		if !stale[record.SessionID] {
			continue
		}
		name := record.SessionID + ".json"
		if record.HeartbeatAt.IsZero() {
			problems = append(problems, Problem{File: name, Reason: "heartbeat time is missing", Err: errInvalidRecord})
			continue
		}
		if now.Sub(record.HeartbeatAt) <= olderThan {
			continue
		}
		if answers != nil && answers(record) {
			continue
		}
		path, _ := r.recordPath(record.SessionID)
		if removeErr := os.Remove(path); removeErr != nil {
			problems = append(problems, recordProblem(name, removeErr))
			continue
		}
		removed = append(removed, record.SessionID)
	}
	if len(removed) > 0 {
		if syncErr := syncDirectory(r.dir); syncErr != nil {
			err = errors.Join(err, fmt.Errorf("endpoint records removed but directory durability is uncertain: %w", syncErr))
		}
	}
	if len(problems) > 0 {
		err = errors.Join(err, &ProblemsError{Problems: problems})
	}
	return removed, err
}
