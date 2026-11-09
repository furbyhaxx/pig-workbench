package instructions

import "os"

// untrustedWriter refuses host files on Windows: ownership and ACLs are not validated there.
func untrustedWriter(os.FileInfo) string { return "cannot be validated on this platform" }
