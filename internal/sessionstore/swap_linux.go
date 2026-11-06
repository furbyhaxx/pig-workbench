package sessionstore

import "golang.org/x/sys/unix"

// swap atomically exchanges the two paths, so the bucket name never stops resolving.
func swap(a, b string) error {
	return unix.Renameat2(unix.AT_FDCWD, a, unix.AT_FDCWD, b, unix.RENAME_EXCHANGE)
}
