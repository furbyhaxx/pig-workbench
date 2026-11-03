// Package secrets provides deterministic secret placeholders and key handling.
package secrets

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

const keySize = 32

// KeyPermissionError reports an existing key file with excessively wide permissions.
type KeyPermissionError struct {
	Path string
	Mode os.FileMode
}

func (e *KeyPermissionError) Error() string {
	return fmt.Sprintf("secret key %q has mode %04o (%s); restrict it with chmod 600", e.Path, e.Mode.Perm(), e.Mode)
}

// LoadKey reads an existing 32-byte master key, rejecting unsafe permissions.
func LoadKey(path string) ([]byte, error) {
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		return nil, fmt.Errorf("open secret key: %w", err)
	}
	defer file.Close()

	mode := info.Mode()
	if mode.Perm() & ^os.FileMode(0600) != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, &KeyPermissionError{Path: path, Mode: mode}
	}
	key, err := io.ReadAll(io.LimitReader(file, keySize+1))
	if err != nil {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	if len(key) != keySize {
		return nil, errors.New("secret key must contain exactly 32 bytes")
	}
	return key, nil
}

// CreateKey creates a private master key or loads the existing key without replacing it.
func CreateKey(path string) ([]byte, error) {
	key, err := LoadKey(path)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create secret key directory: %w", err)
	}
	key = make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".secret-key-*")
	if err != nil {
		return nil, fmt.Errorf("create secret key temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return nil, fmt.Errorf("set new secret key permissions: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		return nil, fmt.Errorf("write secret key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("flush secret key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close secret key: %w", err)
	}
	if err := os.Link(file.Name(), path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return LoadKey(path)
		}
		return nil, fmt.Errorf("install secret key: %w", err)
	}
	return key, nil
}

// DeriveHMACKey derives the placeholder HMAC key from a 32-byte master key.
func DeriveHMACKey(master []byte) ([]byte, error) {
	if len(master) != keySize {
		return nil, errors.New("master key must contain exactly 32 bytes")
	}
	key, err := hkdf.Key(sha256.New, master, nil, "nukii red-sec hmac v1", keySize)
	if err != nil {
		return nil, fmt.Errorf("derive secret HMAC key: %w", err)
	}
	return key, nil
}
