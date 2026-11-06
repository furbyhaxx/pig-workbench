package messaging

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// peerIsCurrentUser checks SO_PEERCRED; the directory mode and the token are the boundary elsewhere.
func peerIsCurrentUser(conn net.Conn) bool {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return false
	}
	var uid uint32
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e != nil {
			credErr = e
			return
		}
		uid = cred.Uid
	}); err != nil || credErr != nil {
		return false
	}
	return int(uid) == os.Getuid()
}
