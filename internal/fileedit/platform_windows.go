package fileedit

import "os"

// Windows cannot flush a directory handle with os.File.Sync.
func syncDirectory(string) error { return nil }

func isCrossDevice(error) bool { return false }

func checkLockDir(string, os.FileInfo) error { return nil }
