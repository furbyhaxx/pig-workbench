package jobs

import (
	"sync"
)

// Stream names one output stream of a job.
type Stream uint8

const (
	// StreamCombined is stdout and stderr in observed ingestion order.
	StreamCombined Stream = iota
	// StreamStdout is the stdout bytes of the combined log.
	StreamStdout
	// StreamStderr is the stderr bytes of the combined log.
	StreamStderr
)

const maxStreamMarks = 4096

type streamMark struct {
	at     int64
	stderr bool
}

// streamTrack records which combined-log offsets came from stderr, so per-stream views share
// the one bounded log instead of tripling retained bytes.
type streamTrack struct {
	mu      sync.Mutex
	logs    *LogStore
	id      string
	written int64
	marks   []streamMark
}

type streamWriter struct {
	track  *streamTrack
	stderr bool
}

func (w streamWriter) Write(data []byte) (int, error) {
	t := w.track
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.logs.Append(t.id, data); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return 0, nil
	}
	if len(t.marks) == 0 || t.marks[len(t.marks)-1].stderr != w.stderr {
		t.marks = append(t.marks, streamMark{at: t.written, stderr: w.stderr})
		if len(t.marks) > maxStreamMarks {
			t.pruneLocked()
		}
	}
	t.written += int64(len(data))
	return len(data), nil
}

// pruneLocked drops marks that only describe evicted bytes.
func (t *streamTrack) pruneLocked() {
	written, retained, _, err := t.logs.Facts(t.id)
	if err != nil {
		return
	}
	start := written - retained
	drop := 0
	for drop+1 < len(t.marks) && t.marks[drop+1].at <= start {
		drop++
	}
	t.marks = append(t.marks[:0], t.marks[drop:]...)
}

// filter keeps the bytes of data, which starts at offset from, that belong to stream.
func (t *streamTrack) filter(data []byte, from int64, stream Stream) []byte {
	t.mu.Lock()
	marks := append([]streamMark(nil), t.marks...)
	t.mu.Unlock()
	wantErr := stream == StreamStderr
	out := make([]byte, 0, len(data))
	for i, mark := range marks {
		end := int64(1<<62 - 1)
		if i+1 < len(marks) {
			end = marks[i+1].at
		}
		if mark.stderr != wantErr {
			continue
		}
		lo, hi := max(mark.at, from), min(end, from+int64(len(data)))
		if lo < hi {
			out = append(out, data[lo-from:hi-from]...)
		}
	}
	return out
}
