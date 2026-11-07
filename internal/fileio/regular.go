// Package fileio provides nonblocking descriptor-based file admission.
package fileio

import (
	"errors"
	"fmt"
	"os"
)

// OpenRegular opens an existing regular file without blocking on a swapped FIFO.
func OpenRegular(path string) (*os.File, os.FileInfo, error) {
	return openChecked(path, os.Stat, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0)
	}, false)
}

// OpenRegularInRoot confines regular-file admission to root.
func OpenRegularInRoot(root *os.Root, path string) (*os.File, os.FileInfo, error) {
	return openChecked(path, root.Stat, func(name string) (*os.File, error) {
		return root.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0)
	}, false)
}

// OpenDirectory opens an existing directory without blocking on a swapped FIFO.
func OpenDirectory(path string) (*os.File, os.FileInfo, error) {
	return openChecked(path, os.Stat, func(name string) (*os.File, error) { return os.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0) }, true)
}

// OpenDirectoryInRoot confines directory admission to root.
func OpenDirectoryInRoot(root *os.Root, path string) (*os.File, os.FileInfo, error) {
	return openChecked(path, root.Stat, func(name string) (*os.File, error) { return root.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0) }, true)
}

func openChecked(path string, stat func(string) (os.FileInfo, error), open func(string) (*os.File, error), directory bool) (*os.File, os.FileInfo, error) {
	kind := "regular file"
	valid := func(info os.FileInfo) bool { return info.Mode().IsRegular() }
	if directory {
		kind = "directory"
		valid = func(info os.FileInfo) bool { return info.IsDir() }
	}
	before, err := stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat file: %w", err)
	}
	if !valid(before) {
		return nil, nil, fmt.Errorf("%q is not a %s", path, kind)
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
	if !valid(opened) || !os.SameFile(before, opened) {
		f.Close()
		return nil, nil, errors.New("file identity changed while opening; retry")
	}
	return f, opened, nil
}
