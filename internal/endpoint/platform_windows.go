package endpoint

import "os"

func platformSupport() error {
	return &PlatformUnsupportedError{Feature: "endpoint registry", Missing: "ACL-based private-access validation"}
}

func checkPrivate(string, os.FileInfo, os.FileMode) error { return platformSupport() }

func pidAlive(int) (bool, error) { return false, &PIDProbeUnsupportedError{} }
