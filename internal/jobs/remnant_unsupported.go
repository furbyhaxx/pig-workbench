//go:build !linux

package jobs

// processIdentity is unavailable without a kernel-provided process identity, so
// remnants are never signalled on this platform (fail closed).
func processIdentity(int) (string, error) {
	return "", ErrProcessIdentityUnsupported
}

func remnantAlive(Record) bool { return false }

func killRemnantGroup(Record) error { return ErrProcessIdentityUnsupported }
