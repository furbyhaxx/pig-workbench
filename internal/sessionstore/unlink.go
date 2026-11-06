package sessionstore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const maxHeaderBytes = 64 << 10

// HeaderCwd returns the working directory recorded in a session file's header line.
func HeaderCwd(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	line, err := bufio.NewReaderSize(file, maxHeaderBytes).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	var header struct {
		Cwd string `json:"cwd"`
	}
	if json.Unmarshal(line, &header) != nil || header.Cwd == "" {
		return "", errors.New("session header has no cwd")
	}
	return header.Cwd, nil
}

// UnlinkResult reports one undo.
type UnlinkResult struct {
	// Restored lists the buckets that are real directories again.
	Restored []string
	// Kept lists session files that belong to no restored bucket and stay in the project storage.
	Kept []string
}

// bucketsLinkedTo lists the buckets under root that are symlinks to target.
func bucketsLinkedTo(root, target string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var buckets []string
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 && isSymlinkTo(filepath.Join(root, entry.Name()), target) {
			buckets = append(buckets, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(buckets)
	return buckets, nil
}

// Unlink replaces every bucket symlink under root that points at target with a real directory
// holding the sessions recorded for that bucket. It can be repeated after an interruption.
func Unlink(root, target string) (UnlinkResult, error) {
	var result UnlinkResult
	buckets, err := bucketsLinkedTo(root, target)
	if err != nil {
		return result, err
	}
	files, err := regularEntries(target)
	if err != nil {
		return result, err
	}
	byBucket := map[string][]string{}
	for _, name := range files {
		cwd, err := HeaderCwd(filepath.Join(target, name))
		if err != nil {
			continue
		}
		byBucket[EncodeBucket(cwd)] = append(byBucket[EncodeBucket(cwd)], name)
	}
	placed := map[string]bool{}
	for _, bucket := range buckets {
		staging := bucket + ".workbench-new"
		if err := os.RemoveAll(staging); err != nil {
			return result, err
		}
		if err := os.Mkdir(staging, 0o700); err != nil {
			return result, err
		}
		for _, name := range byBucket[filepath.Base(bucket)] {
			if err := os.Link(filepath.Join(target, name), filepath.Join(staging, name)); err != nil {
				_ = os.RemoveAll(staging)
				return result, err
			}
			placed[name] = true
		}
		// The staging directory is exchanged with the symlink in one step, then the symlink is dropped.
		if err := swap(staging, bucket); err != nil {
			_ = os.RemoveAll(staging)
			return result, fmt.Errorf("restore %s: %w", bucket, err)
		}
		if err := os.Remove(staging); err != nil {
			return result, err
		}
		result.Restored = append(result.Restored, bucket)
	}
	for name := range placed {
		if err := os.Remove(filepath.Join(target, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
	}
	remaining, err := regularEntries(target)
	if err != nil {
		return result, err
	}
	result.Kept = remaining
	return result, nil
}
