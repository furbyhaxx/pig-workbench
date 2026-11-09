// Package sessionstore converts PiG session buckets into project-shared symlinks and back.
package sessionstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrCrossFilesystem means the bucket and the target are on different filesystems.
	ErrCrossFilesystem = errors.New("bucket and project storage are on different filesystems")
	// ErrCollision means a file of the same name but different content already exists in the target.
	ErrCollision = errors.New("a session file with the same name but different content already exists in the project storage")
	// ErrUnexpectedEntry means the bucket holds something other than regular files.
	ErrUnexpectedEntry = errors.New("the session bucket contains an entry that is not a regular file")
	// ErrForeignLink means the bucket is already a symlink to somewhere else.
	ErrForeignLink = errors.New("the session bucket is a symlink to another location")
)

// Hooks lets tests stop a conversion at a named step, as a crash would.
type Hooks struct{ At func(step string) error }

func (h Hooks) at(step string) error {
	if h.At == nil {
		return nil
	}
	return h.At(step)
}

const (
	stateLinking = "linking"
	stateLinked  = "linked"
	stateSwapped = "swapped"

	oldSuffix = ".workbench-old"
)

type journal struct {
	Version int    `json:"version"`
	Bucket  string `json:"bucket"`
	Target  string `json:"target"`
	State   string `json:"state"`
}

// Result reports what a conversion did.
type Result struct {
	// Converted is true when the bucket is now a symlink to the target.
	Converted bool
	// Already is true when it already was.
	Already bool
	// Linked counts the session files now shared.
	Linked int
	// Leftover names files that could not be placed in the target and stay in <bucket>.workbench-old.
	Leftover []string
}

// JournalPath names the journal of one bucket inside the target's project directory.
func JournalPath(bucket, target string) string {
	return filepath.Join(filepath.Dir(target), "storage-"+filepath.Base(bucket)+".journal.json")
}

func writeJournal(path string, j journal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	file, err := os.Open(tmp)
	if err == nil {
		_ = file.Sync()
		file.Close()
	}
	return os.Rename(tmp, path)
}

// Convert turns the real directory bucket into a symlink to target, sharing every session file.
func Convert(bucket, target string, hooks Hooks) (Result, error) {
	var result Result
	info, err := os.Lstat(bucket)
	if err != nil {
		return result, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(bucket)
		if err != nil {
			return result, err
		}
		if !filepath.IsAbs(link) {
			link = filepath.Join(filepath.Dir(bucket), link)
		}
		if filepath.Clean(link) == filepath.Clean(target) {
			result.Already = true
			return result, nil
		}
		return result, ErrForeignLink
	}
	if !info.IsDir() {
		return result, errors.New("the session bucket is not a directory")
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return result, err
	}
	bucketDev, err := device(bucket)
	if err != nil {
		return result, err
	}
	targetDev, err := device(target)
	if err != nil {
		return result, err
	}
	if bucketDev != targetDev {
		return result, ErrCrossFilesystem
	}
	if _, err := regularEntries(bucket); err != nil {
		return result, err
	}
	j := journal{Version: 1, Bucket: bucket, Target: target, State: stateLinking}
	if err := writeJournal(JournalPath(bucket, target), j); err != nil {
		return result, err
	}
	return run(j, hooks)
}

// Resume finishes the conversion recorded in the journal at path.
func Resume(path string, hooks Hooks) (Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	var j journal
	if err := json.Unmarshal(data, &j); err != nil || j.Version != 1 || j.Bucket == "" || j.Target == "" {
		return Result{}, errors.New("the storage journal is unreadable; inspect it by hand before retrying")
	}
	return run(j, hooks)
}

// Pending returns the journals of unfinished conversions under projects/*/.
func Pending(projectsDir string) []string {
	matches, _ := filepath.Glob(filepath.Join(projectsDir, "*", "storage-*.journal.json"))
	sort.Strings(matches)
	return matches
}

func regularEntries(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("%w: %s", ErrUnexpectedEntry, entry.Name())
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

func linkAll(from, to string, hooks Hooks) (int, error) {
	names, err := regularEntries(from)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, name := range names {
		src, dst := filepath.Join(from, name), filepath.Join(to, name)
		err := os.Link(src, dst)
		if errors.Is(err, os.ErrExist) {
			a, errA := os.Stat(src)
			b, errB := os.Stat(dst)
			if errA == nil && errB == nil && os.SameFile(a, b) {
				count++
				continue
			}
			return count, fmt.Errorf("%w: %s", ErrCollision, name)
		}
		if err != nil {
			return count, err
		}
		count++
		if err := hooks.at("linked:" + name); err != nil {
			return count, err
		}
	}
	return count, nil
}

func isSymlinkTo(path, target string) bool {
	link, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(link) {
		link = filepath.Join(filepath.Dir(path), link)
	}
	return filepath.Clean(link) == filepath.Clean(target)
}

// run drives the conversion from the real filesystem state; each phase is safe to repeat.
func run(j journal, hooks Hooks) (Result, error) {
	var result Result
	journalPath := JournalPath(j.Bucket, j.Target)
	old := j.Bucket + oldSuffix
	if info, err := os.Lstat(j.Bucket); err == nil && info.IsDir() {
		count, err := linkAll(j.Bucket, j.Target, hooks)
		result.Linked = count
		if err != nil {
			if errors.Is(err, ErrCollision) {
				_ = os.Remove(journalPath)
			}
			return result, err
		}
		j.State = stateLinked
		if err := writeJournal(journalPath, j); err != nil {
			return result, err
		}
		if err := hooks.at("journal-linked"); err != nil {
			return result, err
		}
		if lstat, err := os.Lstat(old); err == nil && lstat.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(old); err != nil {
				return result, err
			}
		}
		if _, err := os.Lstat(old); err == nil {
			return result, fmt.Errorf("%s exists from an earlier attempt; resolve it by hand", old)
		}
		if err := os.Symlink(j.Target, old); err != nil {
			return result, err
		}
		if err := hooks.at("link-created"); err != nil {
			return result, err
		}
		if err := swap(old, j.Bucket); err != nil {
			_ = os.Remove(old)
			return result, err
		}
		if err := hooks.at("swapped"); err != nil {
			return result, err
		}
	} else if err == nil && !isSymlinkTo(j.Bucket, j.Target) {
		return result, ErrForeignLink
	}
	if !isSymlinkTo(j.Bucket, j.Target) {
		return result, errors.New("the bucket is neither a directory nor the expected symlink")
	}
	j.State = stateSwapped
	if err := writeJournal(journalPath, j); err != nil {
		return result, err
	}
	leftover, err := cleanup(old, j.Target, hooks)
	result.Leftover = leftover
	if err != nil {
		return result, err
	}
	if names, err := regularEntries(j.Target); err == nil {
		result.Linked = len(names)
	}
	result.Converted = true
	if len(leftover) == 0 {
		_ = os.Remove(journalPath)
	}
	return result, nil
}

// cleanup removes the old names of shared files and moves files PiG created during the swap into the target.
func cleanup(old, target string, hooks Hooks) ([]string, error) {
	info, err := os.Lstat(old)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not the old bucket directory", old)
	}
	entries, err := os.ReadDir(old)
	if err != nil {
		return nil, err
	}
	var leftover []string
	for _, entry := range entries {
		src, dst := filepath.Join(old, entry.Name()), filepath.Join(target, entry.Name())
		a, errA := os.Lstat(src)
		b, errB := os.Lstat(dst)
		switch {
		case errA != nil:
			continue
		case errB == nil && os.SameFile(a, b):
			if err := os.Remove(src); err != nil {
				return leftover, err
			}
		case errB != nil && a.Mode().IsRegular():
			if err := os.Rename(src, dst); err != nil {
				return leftover, err
			}
		default:
			leftover = append(leftover, entry.Name())
		}
		if err := hooks.at("cleaned:" + entry.Name()); err != nil {
			return leftover, err
		}
	}
	if len(leftover) > 0 {
		return leftover, nil
	}
	return nil, os.Remove(old)
}

// EncodeBucket is PiG's directory name for a working directory.
func EncodeBucket(cwd string) string {
	trimmed := strings.TrimLeft(cwd, `/\`)
	return "--" + strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(trimmed) + "--"
}
