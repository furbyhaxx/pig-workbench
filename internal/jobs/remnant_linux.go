//go:build linux

package jobs

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const remnantGrace = 2 * time.Second

// processIdentity returns a stable token for a live process: its kernel start
// time, which differs for any later process that reuses the same pid.
func processIdentity(pid int) (string, error) {
	token, _, err := processStat(pid)
	return token, err
}

// processStat reads the start-time token and the process state. A zombie has
// already run to completion and must count as dead even though the kernel keeps
// its /proc entry until the parent reaps it.
func processStat(pid int) (token string, zombie bool, err error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, ErrProcessIdentityUnsupported
		}
		return "", false, fmt.Errorf("read process identity: %w", err)
	}
	stat := string(data)
	// The command name is parenthesised and may contain spaces, so fields are
	// counted from the last ')'. Starttime is the 22nd field overall, state the 3rd.
	close := strings.LastIndex(stat, ")")
	if close < 0 {
		return "", false, errors.New("unexpected /proc stat format")
	}
	fields := strings.Fields(stat[close+1:])
	if len(fields) < 20 {
		return "", false, errors.New("unexpected /proc stat field count")
	}
	return fields[19], fields[0] == "Z", nil
}

// remnantAlive reports whether the recorded process still is the recorded one.
func remnantAlive(record Record) bool {
	if record.Identity == "" {
		return false
	}
	token, zombie, err := processStat(record.PID)
	if err != nil || zombie {
		return false
	}
	return token == record.Identity
}

// killRemnantGroup terminates a verified remnant's process group. The identity
// is rechecked immediately before signalling so a recycled pid is never hit.
func killRemnantGroup(record Record) error {
	fd, err := unix.PidfdOpen(record.PID, 0)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		return fmt.Errorf("open remnant pidfd for job %s: %w", record.JobID, err)
	}
	defer unix.Close(fd)
	if !remnantAlive(record) {
		return nil
	}
	if err := unix.PidfdSendSignal(fd, syscall.SIGTERM, nil, unix.PIDFD_SIGNAL_PROCESS_GROUP); err != nil && !errors.Is(err, unix.ESRCH) {
		return fmt.Errorf("terminate remnant group for job %s: %w", record.JobID, err)
	}
	deadline := time.NewTimer(remnantGrace)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			if err := unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, unix.PIDFD_SIGNAL_PROCESS_GROUP); err != nil && !errors.Is(err, unix.ESRCH) {
				return fmt.Errorf("kill remnant group for job %s: %w", record.JobID, err)
			}
			return nil
		case <-ticker.C:
			if err := unix.PidfdSendSignal(fd, 0, nil, unix.PIDFD_SIGNAL_PROCESS_GROUP); errors.Is(err, unix.ESRCH) {
				return nil
			}
		}
	}
}
