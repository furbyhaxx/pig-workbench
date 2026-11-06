//go:build !linux

package sessionstore

import "os"

// swap replaces the directory b by the symlink a; there is a short window without a bucket.
func swap(a, b string) error {
	link, err := os.Readlink(a)
	if err != nil {
		return err
	}
	if err := os.Rename(b, a+".moved"); err != nil {
		return err
	}
	if err := os.Symlink(link, b); err != nil {
		_ = os.Rename(a+".moved", b)
		return err
	}
	if err := os.Remove(a); err != nil {
		return err
	}
	return os.Rename(a+".moved", a)
}
