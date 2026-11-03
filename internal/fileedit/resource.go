package fileedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// target is one effect resolved to canonical identities.
type target struct {
	effect Effect
	source string      // canonical absolute path of the source's resolved target
	dest   string      // canonical absolute move destination, empty without a move
	info   os.FileInfo // source identity observed during preflight
	before string      // source revision observed during preflight
}

func revisionOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validRevision(revision string) bool {
	if len(revision) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

// resolveSource canonicalizes an existing source and requires a regular file.
func resolveSource(path string) (string, os.FileInfo, error) {
	if path == "" {
		return "", nil, errors.New("source path is empty")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", nil, fmt.Errorf("resolve source symlinks: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", nil, fmt.Errorf("inspect source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("source %s is not a regular file", canonical)
	}
	return canonical, info, nil
}

// resolveDestination canonicalizes a move destination that must not exist.
func resolveDestination(path string) (string, error) {
	if path == "" {
		return "", errors.New("move destination is empty")
	}
	absolute := filepath.Clean(path)
	parent, base := filepath.Split(absolute)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("destination %s does not name a file", path)
	}
	if _, err := os.Lstat(absolute); err == nil {
		return "", fmt.Errorf("%w: %s; choose a destination that does not exist", ErrDestinationExists, absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect destination %s: %w", absolute, err)
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Clean(parent))
	if err != nil {
		return "", fmt.Errorf("resolve destination directory: %w", err)
	}
	info, err := os.Stat(resolvedParent)
	if err != nil {
		return "", fmt.Errorf("inspect destination directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("destination directory %s is not a directory", resolvedParent)
	}
	return filepath.Join(resolvedParent, base), nil
}

// observation is the source state read without retaining its bytes.
type observation struct {
	revision string
	size     int64
	info     os.FileInfo
}

// observeSource re-resolves the effect path, verifies the source identity and
// expected revision, and digests the current bytes without buffering them.
func (t *target) observeSource(ctx context.Context) (observation, error) {
	if err := ctx.Err(); err != nil {
		return observation{}, fmt.Errorf("observe source %s: %w", t.effect.Path, err)
	}
	canonical, _, err := resolveSource(t.effect.Path)
	if err != nil {
		return observation{}, fmt.Errorf("source %s is no longer usable; re-read it and retry: %w", t.effect.Path, err)
	}
	if canonical != t.source {
		return observation{}, fmt.Errorf("%w: %s now resolves to %s instead of %s; re-read the source and retry", ErrIdentityChanged, t.effect.Path, canonical, t.source)
	}
	file, opened, err := fileio.OpenRegular(t.effect.Path)
	if err != nil {
		return observation{}, fmt.Errorf("open source %s: %w", t.effect.Path, err)
	}
	revision, size, digestErr := digestStream(ctx, file)
	closeErr := file.Close()
	if digestErr != nil {
		return observation{}, fmt.Errorf("digest source %s: %w", t.effect.Path, digestErr)
	}
	if closeErr != nil {
		return observation{}, fmt.Errorf("close source %s: %w", t.effect.Path, closeErr)
	}
	if !os.SameFile(t.info, opened) {
		return observation{}, fmt.Errorf("%w: %s was replaced after preflight; re-read the source and retry", ErrIdentityChanged, t.source)
	}
	if revision != strings.ToLower(t.effect.Revision) {
		return observation{}, fmt.Errorf("%w: %s is at revision %s, expected %s; re-read the source and restage the effect", ErrRevisionChanged, t.source, revision, t.effect.Revision)
	}
	return observation{revision: revision, size: size, info: opened}, nil
}

// digestStream hashes reader in bounded chunks and stops when ctx is done.
func digestStream(ctx context.Context, reader io.Reader) (string, int64, error) {
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		n, err := reader.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			size += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", 0, err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

// checkDestination re-resolves a move destination and requires it still missing.
func (t *target) checkDestination() error {
	if t.dest == "" {
		return nil
	}
	current, err := resolveDestination(t.effect.MoveTo)
	if err != nil {
		return err
	}
	if current != t.dest {
		return fmt.Errorf("%w: destination %s now resolves to %s; re-read the destination and retry", ErrIdentityChanged, t.effect.MoveTo, current)
	}
	return nil
}

func dirOf(path string) string {
	return filepath.Dir(path)
}
