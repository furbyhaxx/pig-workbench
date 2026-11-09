//go:build !unix

package secrets

import "errors"

func narrowKeyMode(string) error {
	return errors.New("key permissions cannot be narrowed on this platform")
}
