package jobs

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	logPageSize    = 64 * 1024
	logSegmentSize = 32 * 1024
)

// ErrInvalidLogID means a log job ID is empty.
var ErrInvalidLogID = errors.New("log job ID is empty")

// ErrInvalidLogCursor means a cursor is negative or beyond the written offset.
var ErrInvalidLogCursor = errors.New("log cursor is outside the written range")

// ErrLogNotFound means no log has been created for the job ID.
var ErrLogNotFound = errors.New("job log not found")

// LogEvictedError means the entire retained log was evicted by the session quota.
type LogEvictedError struct{}

// Error describes the eviction.
func (*LogEvictedError) Error() string { return "job log was evicted" }

// ErrLogEvicted identifies a whole-job quota eviction.
var ErrLogEvicted = &LogEvictedError{}

// LogDroppedError means a log was explicitly dropped.
type LogDroppedError struct{}

// Error describes the dropped log.
func (*LogDroppedError) Error() string { return "job log was dropped" }

// ErrLogDropped identifies an explicitly dropped log.
var ErrLogDropped = &LogDroppedError{}

// LogClosedError means the log store has been closed.
type LogClosedError struct{}

// Error describes the closed store.
func (*LogClosedError) Error() string { return "log store is closed" }

// ErrLogClosed identifies a closed log store.
var ErrLogClosed = &LogClosedError{}

type logStatus uint8

const (
	logLive logStatus = iota
	logEvicted
	logDropped
)

type logView struct {
	segments  [][]byte
	written   int64
	start     int64
	retained  int64
	truncated bool
	status    logStatus
}

type logEntry struct {
	id     string
	window uint64
}

type logIndex struct {
	entries map[string]*logView
	used    int64
	closed  bool
}

// LogStore owns bounded raw job output and monotonic per-job byte offsets.
// Readers use immutable snapshots and never hold a lock needed by Append.
type LogStore struct {
	mu         sync.Mutex
	perJob     int64
	perSession int64
	window     uint64
	active     map[string]*logEntry
	index      atomic.Pointer[logIndex]
}

// NewLogStore creates a store with strictly positive per-job and session caps.
func NewLogStore(perJob, perSession int64) (*LogStore, error) {
	if perJob <= 0 || perSession <= 0 {
		return nil, errors.New("log limits must be positive")
	}
	store := &LogStore{perJob: perJob, perSession: perSession, active: make(map[string]*logEntry)}
	store.index.Store(&logIndex{entries: make(map[string]*logView)})
	return store, nil
}

// SetLimits changes the caps for later appends; retained bytes shrink as their logs grow.
func (l *LogStore) SetLimits(perJob, perSession int64) error {
	if perJob <= 0 || perSession <= 0 {
		return errors.New("log limits must be positive")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.perJob, l.perSession = perJob, perSession
	return nil
}

// Append copies data into a job's log; only empty IDs and closed stores fail.
// A write after eviction or Drop revives the log without resetting its offset.
func (l *LogStore) Append(jobID string, data []byte) error {
	if l.index.Load().closed {
		return ErrLogClosed
	}
	if jobID == "" {
		return ErrInvalidLogID
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.index.Load()
	if index.closed {
		return ErrLogClosed
	}
	previous := index.entries[jobID]
	entry := l.active[jobID]
	if previous == nil {
		previous = &logView{}
		entry = &logEntry{id: jobID}
		l.startWindow(entry)
	} else if previous.status != logLive {
		previous = &logView{written: previous.written, start: previous.written, truncated: previous.truncated}
		entry = &logEntry{id: jobID}
		l.startWindow(entry)
	}
	entries := cloneLogIndex(index.entries)
	limit := min(l.perJob, l.perSession)
	view := appendLogView(previous, data, limit)
	used := index.used + view.retained - previous.retained
	for used > l.perSession {
		victim := l.oldestOther(entry)
		if victim == nil {
			view = trimLogView(view, used-l.perSession)
			used = l.perSession
			break
		}
		old := entries[victim.id]
		used -= old.retained
		entries[victim.id] = &logView{written: old.written, start: old.written, truncated: old.truncated || old.retained > 0, status: logEvicted}
		delete(l.active, victim.id)
	}
	entries[jobID] = view
	l.index.Store(&logIndex{entries: entries, used: used})
	return nil
}

func cloneLogIndex(previous map[string]*logView) map[string]*logView {
	entries := make(map[string]*logView, len(previous)+1)
	for id, view := range previous {
		entries[id] = view
	}
	return entries
}

func (l *LogStore) startWindow(entry *logEntry) {
	l.window++
	entry.window = l.window
	l.active[entry.id] = entry
}

func (l *LogStore) oldestOther(current *logEntry) *logEntry {
	var oldest *logEntry
	for _, entry := range l.active {
		if entry == current {
			continue
		}
		if oldest == nil || entry.window < oldest.window || (entry.window == oldest.window && entry.id < oldest.id) {
			oldest = entry
		}
	}
	return oldest
}

func appendLogView(previous *logView, data []byte, limit int64) *logView {
	written := previous.written + int64(len(data))
	retained := previous.retained + int64(len(data))
	segments := previous.segments
	if int64(len(data)) >= limit {
		data = data[len(data)-int(limit):]
		segments = nil
		retained = int64(len(data))
	}
	segments = appendLogSegments(segments, data)
	view := &logView{segments: segments, written: written, start: written - retained, retained: retained, truncated: previous.truncated}
	if retained > limit {
		view = trimLogView(view, retained-limit)
	}
	if view.start > previous.start {
		view.truncated = true
	}
	return view
}

func appendLogSegments(previous [][]byte, data []byte) [][]byte {
	segments := make([][]byte, len(previous), len(previous)+(len(data)/logSegmentSize)+1)
	copy(segments, previous)
	if len(segments) > 0 && len(segments[len(segments)-1]) < logSegmentSize && len(data) > 0 {
		last := segments[len(segments)-1]
		n := min(logSegmentSize-len(last), len(data))
		merged := make([]byte, len(last)+n)
		copy(merged, last)
		copy(merged[len(last):], data[:n])
		segments[len(segments)-1] = merged
		data = data[n:]
	}
	for len(data) > 0 {
		n := min(logSegmentSize, len(data))
		segments = append(segments, append([]byte(nil), data[:n]...))
		data = data[n:]
	}
	return segments
}

func trimLogView(view *logView, discard int64) *logView {
	segments := append([][]byte(nil), view.segments...)
	left := discard
	first := 0
	for left > 0 {
		segment := segments[first]
		if left >= int64(len(segment)) {
			left -= int64(len(segment))
			segments[first] = nil
			first++
			continue
		}
		segments[first] = append([]byte(nil), segment[int(left):]...)
		left = 0
	}
	return &logView{segments: segments[first:], written: view.written, start: view.start + discard, retained: view.retained - discard, truncated: true}
}

// Read returns at most 64 KiB from cursor and the next monotonic byte offset.
// A nonpositive max uses the page size; larger positive values are capped there.
// truncated is true if cursor was evicted, and gapFrom is the first retained offset.
func (l *LogStore) Read(jobID string, cursor int64, maxBytes int) (data []byte, next int64, truncated bool, gapFrom int64, err error) {
	view, err := l.snapshot(jobID)
	if err != nil {
		return nil, 0, false, 0, err
	}
	if cursor < 0 || cursor > view.written {
		return nil, 0, false, 0, ErrInvalidLogCursor
	}
	if cursor < view.start {
		truncated, gapFrom, cursor = true, view.start, view.start
	}
	if maxBytes <= 0 || maxBytes > logPageSize {
		maxBytes = logPageSize
	}
	count := int(min(int64(maxBytes), view.written-cursor))
	data = make([]byte, 0, count)
	position := view.start
	for _, segment := range view.segments {
		end := position + int64(len(segment))
		if cursor < end && len(data) < count {
			start := int(max(cursor-position, 0))
			n := min(count-len(data), len(segment)-start)
			data = append(data, segment[start:start+n]...)
			cursor += int64(n)
		}
		position = end
		if len(data) == count {
			break
		}
	}
	return data, cursor, truncated, gapFrom, nil
}

// Facts reports lifetime bytes, retained bytes, and whether any output was lost.
// Evicted and dropped logs return their last written offset and a typed error.
func (l *LogStore) Facts(jobID string) (written int64, retained int64, truncated bool, err error) {
	view, err := l.snapshot(jobID)
	if view == nil {
		return 0, 0, false, err
	}
	return view.written, view.retained, view.truncated, err
}

func (l *LogStore) snapshot(jobID string) (*logView, error) {
	index := l.index.Load()
	if index.closed {
		return nil, ErrLogClosed
	}
	if jobID == "" {
		return nil, ErrInvalidLogID
	}
	view := index.entries[jobID]
	if view == nil {
		return nil, ErrLogNotFound
	}
	switch view.status {
	case logEvicted:
		return view, fmt.Errorf("%w: %s", ErrLogEvicted, jobID)
	case logDropped:
		return view, fmt.Errorf("%w: %s", ErrLogDropped, jobID)
	default:
		return view, nil
	}
}

// Drop releases a log and marks it dropped until the next Append for that ID.
// Appending after Drop resumes from the previous lifetime byte offset.
func (l *LogStore) Drop(jobID string) error {
	if jobID == "" {
		return ErrInvalidLogID
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.index.Load()
	if index.closed {
		return ErrLogClosed
	}
	view := index.entries[jobID]
	if view == nil {
		return ErrLogNotFound
	}
	used := index.used
	if view.status == logLive {
		used -= view.retained
		delete(l.active, jobID)
	}
	entries := cloneLogIndex(index.entries)
	entries[jobID] = &logView{written: view.written, start: view.written, truncated: view.truncated || view.retained > 0, status: logDropped}
	l.index.Store(&logIndex{entries: entries, used: used})
	return nil
}

// Forget removes a log's bytes and metadata permanently, so the ID answers as unknown
// afterwards. Unlike Drop nothing survives for a later Append or Facts.
func (l *LogStore) Forget(jobID string) error {
	if jobID == "" {
		return ErrInvalidLogID
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	index := l.index.Load()
	if index.closed {
		return ErrLogClosed
	}
	view := index.entries[jobID]
	if view == nil {
		return ErrLogNotFound
	}
	used := index.used
	if view.status == logLive {
		used -= view.retained
		delete(l.active, jobID)
	}
	entries := cloneLogIndex(index.entries)
	delete(entries, jobID)
	l.index.Store(&logIndex{entries: entries, used: used})
	return nil
}

// SessionBytes returns the currently retained output byte count.
func (l *LogStore) SessionBytes() int64 { return l.index.Load().used }

// Close discards retained output and prevents further access; it is idempotent.
func (l *LogStore) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.index.Load().closed {
		return nil
	}
	l.index.Store(&logIndex{closed: true})
	l.active = nil
	return nil
}
