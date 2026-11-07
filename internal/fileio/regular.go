// Package fileio provides safe local regular-file admission.
package fileio

import (
	"errors"
	"fmt"
	"os"
)

// OpenRegular opens an existing regular file without blocking on a swapped FIFO.
func OpenRegular(path string) (*os.File, os.FileInfo, error) {
	return openRegular(path, os.Stat, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0)
	})
}

// OpenRegularInRoot confines regular-file admission to root.
func OpenRegularInRoot(root *os.Root, path string) (*os.File, os.FileInfo, error) {
	return openRegular(path, root.Stat, func(name string) (*os.File, error) {
		return root.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0)
	})
}

func openRegular(path string, stat func(string) (os.FileInfo, error), open func(string) (*os.File, error)) (*os.File, os.FileInfo, error) {
	before, err := stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat file: %w", err)
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%q is not a regular file", path)
	}
	f, err := open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open file: %w", err)
	}
	opened, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		f.Close()
		return nil, nil, errors.New("file identity changed while opening; retry")
	}
	return f, opened, nil
}
