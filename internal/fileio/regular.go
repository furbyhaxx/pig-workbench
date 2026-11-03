// Package fileio provides safe local regular-file admission.
package fileio

import (
	"errors"
	"fmt"
	"os"
)

// OpenRegular opens an existing regular file without blocking on a swapped FIFO.
func OpenRegular(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("stat file: %w", err)
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%q is not a regular file", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|nonblockFlag(), 0)
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
