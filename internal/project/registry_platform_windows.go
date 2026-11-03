package project

import (
	"errors"
	"os"
)

func registryPlatformSupport() error {
	return errors.New("project registry unsupported: ACL-based private-access validation is unavailable")
}

func checkRegistryPrivate(string, os.FileInfo, os.FileMode) error {
	return registryPlatformSupport()
}

func syncRegistryDirectory(string) error { return registryPlatformSupport() }
