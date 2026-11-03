package fileio

import "os"

func nonblockFlag() int { return 0 }

// LockFlags opens a Windows lock file for both reading and writing.
func LockFlags() int { return os.O_CREATE | os.O_RDWR }
