package fileio

import "os"

// NonblockFlag is empty: Windows opens do not block on the file kind.
func NonblockFlag() int { return 0 }

// LockFlags opens a Windows lock file for both reading and writing.
func LockFlags() int { return os.O_CREATE | os.O_RDWR }
