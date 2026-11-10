package fileio

// SyncDirectory does nothing: Windows cannot flush a directory handle.
func SyncDirectory(string) error { return nil }
