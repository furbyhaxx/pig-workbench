// Package endpoint maintains the private registry of live session endpoints.
package endpoint

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxRecordSize = 64 * 1024

var (
	// ErrTokenMismatch indicates that a token did not match the registered owner.
	ErrTokenMismatch = errors.New("endpoint token mismatch")
	// ErrSessionExists indicates that another live owner holds the session ID.
	ErrSessionExists = errors.New("live endpoint already registered")
	// ErrNotFound indicates that a session record does not exist.
	ErrNotFound = errors.New("endpoint not found")
	// ErrRecordTooLarge indicates that the encoded record exceeds 64 KiB.
	ErrRecordTooLarge = errors.New("endpoint record exceeds 64 KiB")
	errInvalidRecord  = errors.New("invalid endpoint record")
)

// Record is a session's private endpoint metadata.
type Record struct {
	SchemaVersion int       `json:"schemaVersion"`
	SessionID     string    `json:"sessionId"`
	Token         string    `json:"token"`
	Cwd           string    `json:"cwd"`
	ProjectID     string    `json:"projectId"`
	State         string    `json:"state"`
	PID           int       `json:"pid"`
	StartedAt     time.Time `json:"startedAt"`
	HeartbeatAt   time.Time `json:"heartbeatAt"`
	Discoverable  bool      `json:"discoverable"`
	// Endpoint names the session's socket file; it is stable across reloads.
	StateSince  time.Time `json:"stateSince,omitempty"`
	Endpoint    string    `json:"endpoint,omitempty"`
	SessionName string    `json:"sessionName,omitempty"`
	ProjectSlug string    `json:"projectSlug,omitempty"`
	Model       string    `json:"model,omitempty"`
}

// Problem identifies a record that could not be safely inspected.
type Problem struct {
	File   string
	Reason string
	Err    error
}

// Error formats the problem without including record contents.
func (p Problem) Error() string { return fmt.Sprintf("%q: %s", p.File, p.Reason) }

// Unwrap returns the underlying inspection error.
func (p Problem) Unwrap() error { return p.Err }

// ListResult contains all valid records, diagnostics, and IDs whose PIDs are dead.
type ListResult struct {
	Records  []Record
	Problems []Problem
	Stale    []string
}

// ProblemsError reports records that could not be inspected during pruning.
type ProblemsError struct {
	Problems []Problem
}

// Error summarizes the number of records that could not be inspected.
func (e *ProblemsError) Error() string {
	return fmt.Sprintf("could not inspect %d endpoint record(s)", len(e.Problems))
}

// Unwrap returns the individual inspection problems.
func (e *ProblemsError) Unwrap() []error {
	errs := make([]error, len(e.Problems))
	for i := range e.Problems {
		errs[i] = e.Problems[i]
	}
	return errs
}

// PermissionError reports a path whose privacy or ownership is unacceptable.
type PermissionError struct {
	Path   string
	Mode   os.FileMode
	Reason string
}

// Error describes the path and its unacceptable permissions or ownership.
func (e *PermissionError) Error() string {
	return fmt.Sprintf("endpoint path %q: %s (mode %04o)", e.Path, e.Reason, e.Mode.Perm())
}

// PlatformUnsupportedError reports a security guarantee unavailable on this platform.
type PlatformUnsupportedError struct {
	Feature string
	Missing string
}

// Error describes the unsupported feature and missing guarantee.
func (e *PlatformUnsupportedError) Error() string {
	return fmt.Sprintf("%s unsupported: %s", e.Feature, e.Missing)
}

// PIDProbeUnsupportedError reports the lack of a supported PID liveness probe.
type PIDProbeUnsupportedError struct{}

// Error describes the unavailable PID liveness probe.
func (*PIDProbeUnsupportedError) Error() string { return "PID liveness probe unsupported" }

// ValidSessionID reports whether id is a session identifier Workbench accepts: usable as one file name on every platform.
func ValidSessionID(id string) bool {
	if id == "" || id == "." || id == ".." || !utf8.ValidString(id) || strings.HasSuffix(id, ".") || strings.HasSuffix(id, " ") {
		return false
	}
	for _, ch := range id {
		if unicode.IsControl(ch) || strings.ContainsRune(`/\:*?"<>|`, ch) {
			return false
		}
	}
	return true
}

func validateRecord(record Record) error {
	if record.SchemaVersion != 1 || !ValidSessionID(record.SessionID) || record.Token == "" || record.PID <= 0 || record.PID > math.MaxInt32 {
		return errInvalidRecord
	}
	return nil
}

func recordProblem(name string, err error) Problem {
	reason := "cannot inspect record"
	var permission *PermissionError
	var unsupported *PIDProbeUnsupportedError
	switch {
	case errors.As(err, &permission):
		reason = "unsafe permissions or ownership"
	case errors.As(err, &unsupported):
		reason = "PID liveness probe unsupported"
	case errors.Is(err, ErrRecordTooLarge):
		reason = "record exceeds 64 KiB"
	case errors.Is(err, errInvalidRecord):
		reason = "invalid record"
	}
	return Problem{File: name, Reason: reason, Err: err}
}
