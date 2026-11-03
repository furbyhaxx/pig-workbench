// Package endpoint maintains the private registry of live session endpoints.
package endpoint

import (
	"errors"
	"fmt"
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
}

// Problem identifies a record that could not be safely inspected.
type Problem struct {
	File   string
	Reason string
	Err    error
}

func (p Problem) Error() string { return fmt.Sprintf("%q: %s", p.File, p.Reason) }
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

func (e *ProblemsError) Error() string {
	return fmt.Sprintf("could not inspect %d endpoint record(s)", len(e.Problems))
}

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

func (e *PermissionError) Error() string {
	return fmt.Sprintf("endpoint path %q: %s (mode %04o)", e.Path, e.Reason, e.Mode.Perm())
}

// PlatformUnsupportedError reports a security guarantee unavailable on this platform.
type PlatformUnsupportedError struct {
	Feature string
	Missing string
}

func (e *PlatformUnsupportedError) Error() string {
	return fmt.Sprintf("%s unsupported: %s", e.Feature, e.Missing)
}

// PIDProbeUnsupportedError reports the lack of a supported PID liveness probe.
type PIDProbeUnsupportedError struct{}

func (*PIDProbeUnsupportedError) Error() string { return "PID liveness probe unsupported" }

func validSessionID(id string) bool {
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
	if record.SchemaVersion != 1 || !validSessionID(record.SessionID) || record.Token == "" || record.PID <= 0 {
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
