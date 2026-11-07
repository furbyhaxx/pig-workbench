//go:build unix

package fileio

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDirectoryAdmissionRejectsFIFOSubstitution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	opened, _, err := openChecked(path, os.Stat, func(name string) (*os.File, error) {
		if err := os.Rename(name, name+"-old"); err != nil {
			return nil, err
		}
		if err := syscall.Mkfifo(name, 0600); err != nil {
			return nil, err
		}
		return os.OpenFile(name, os.O_RDONLY|nonblockFlag(), 0)
	}, true)
	if opened != nil {
		opened.Close()
	}
	if err == nil {
		t.Fatal("swapped FIFO was admitted as a directory")
	}
}

func TestRootedDirectoryAdmissionRefusesOutsideLinks(t *testing.T) {
	rootPath, outside := t.TempDir(), t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Symlink(outside, filepath.Join(rootPath, "escape")); err != nil {
		t.Fatal(err)
	}
	opened, _, err := OpenDirectoryInRoot(root, "escape")
	if opened != nil {
		opened.Close()
	}
	if err == nil {
		t.Fatal("directory admission escaped the root")
	}
}
