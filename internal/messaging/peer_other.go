//go:build !linux

package messaging

import "net"

// Without peer credentials the 0700 directory and the token are the boundary.
func peerIsCurrentUser(net.Conn) bool { return true }
