package config

// Windows does not support flushing a directory handle with os.File.Sync.
func syncDirectory(string) error { return nil }
