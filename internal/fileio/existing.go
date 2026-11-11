package fileio

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// NearestExisting returns path or its closest ancestor that still exists, so a removed
// working directory resolves to the tree it was in.
func NearestExisting(path string) string {
	for {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}
