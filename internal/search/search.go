// Package search implements bounded file search and glob walking.
package search

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// Match is one exact source match.
type Match struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

// Result reports what was searched and whether the bound was reached.
type Result struct {
	Matches      []Match  `json:"matches"`
	FilesScanned int      `json:"filesScanned"`
	Complete     bool     `json:"complete"`
	Skipped      []string `json:"skipped,omitempty"`
}

// Limits bound one search.
type Limits struct {
	MaxMatches   int
	MaxFiles     int
	MaxFileBytes int64
	Context      int
}

// Options is one grep request with already-validated semantics.
type Options struct {
	Root       string
	Patterns   []string
	Literal    bool
	IgnoreCase bool
	Glob       string
	Exclude    []string
	FilesOnly  bool
	Limits     Limits
}

// ErrInvalidPattern reports a regular expression the caller must fix.
var ErrInvalidPattern = errors.New("invalid search pattern")

// Grep searches regular files under root without following symlinked directories.
func Grep(ctx context.Context, options Options) (Result, error) {
	if options.Root == "" {
		return Result{}, errors.New("search root is empty")
	}
	if len(options.Patterns) == 0 {
		return Result{}, errors.New("search needs at least one pattern")
	}
	limits := options.Limits
	if limits.MaxMatches <= 0 {
		limits.MaxMatches = 200
	}
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = 5000
	}
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = 4 << 20
	}
	var expressions []*regexp.Regexp
	var literals []string
	if options.Literal {
		for _, pattern := range options.Patterns {
			if pattern == "" {
				return Result{}, errors.New("literal pattern is empty")
			}
			if options.IgnoreCase {
				literals = append(literals, strings.ToLower(pattern))
			} else {
				literals = append(literals, pattern)
			}
		}
	} else {
		for _, pattern := range options.Patterns {
			if pattern == "" {
				return Result{}, errors.New("regex pattern is empty")
			}
			expression := pattern
			if options.IgnoreCase {
				expression = "(?i)" + expression
			}
			compiled, err := regexp.Compile(expression)
			if err != nil {
				return Result{}, fmt.Errorf("%w: %v", ErrInvalidPattern, err)
			}
			expressions = append(expressions, compiled)
		}
	}
	result := Result{Complete: true}
	err := filepath.WalkDir(options.Root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				result.Skipped = append(result.Skipped, path)
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			if path != options.Root && (entry.Name() == ".git" || excluded(path, options.Exclude)) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if options.Glob != "" {
			matched, err := filepath.Match(options.Glob, entry.Name())
			if err != nil {
				return fmt.Errorf("%w: glob %q", ErrInvalidPattern, options.Glob)
			}
			if !matched {
				return nil
			}
		}
		if excluded(path, options.Exclude) {
			return nil
		}
		if result.FilesScanned >= limits.MaxFiles {
			result.Complete = false
			return fs.SkipAll
		}
		matches, scanned, err := scanFile(ctx, path, options, expressions, literals, limits)
		if err != nil {
			if errors.Is(err, errTooLarge) || errors.Is(err, errBinary) {
				result.Skipped = append(result.Skipped, path)
				return nil
			}
			return err
		}
		result.FilesScanned += scanned
		if options.FilesOnly && len(matches) > 0 {
			matches = matches[:1]
		}
		for _, match := range matches {
			if len(result.Matches) >= limits.MaxMatches {
				result.Complete = false
				return fs.SkipAll
			}
			result.Matches = append(result.Matches, match)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	sort.SliceStable(result.Matches, func(i, j int) bool {
		if result.Matches[i].Path != result.Matches[j].Path {
			return result.Matches[i].Path < result.Matches[j].Path
		}
		return result.Matches[i].Line < result.Matches[j].Line
	})
	return result, nil
}

var (
	errTooLarge = errors.New("file exceeds the search byte limit")
	errBinary   = errors.New("file looks binary")
)

func scanFile(ctx context.Context, path string, options Options, expressions []*regexp.Regexp, literals []string, limits Limits) ([]Match, int, error) {
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer file.Close()
	if info.Size() > limits.MaxFileBytes {
		return nil, 0, errTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, limits.MaxFileBytes+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(data)) > limits.MaxFileBytes {
		return nil, 0, errTooLarge
	}
	if looksBinary(data) {
		return nil, 0, errBinary
	}
	var matches []Match
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		column := hit(line, expressions, literals, options.IgnoreCase)
		if column == 0 {
			continue
		}
		matches = append(matches, Match{Path: path, Line: index + 1, Column: column, Text: strings.TrimRight(line, "\r")})
	}
	return matches, 1, nil
}

func hit(line string, expressions []*regexp.Regexp, literals []string, ignoreCase bool) int {
	haystack := line
	if ignoreCase {
		haystack = strings.ToLower(line)
	}
	for _, expression := range expressions {
		if location := expression.FindStringIndex(line); location != nil {
			return location[0] + 1
		}
	}
	for _, literal := range literals {
		if index := strings.Index(haystack, literal); index >= 0 {
			return index + 1
		}
	}
	return 0
}

func looksBinary(data []byte) bool {
	probe := data
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	return strings.IndexByte(string(probe), 0) >= 0
}

func excluded(path string, patterns []string) bool {
	base := filepath.Base(path)
	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return true
		}
		if strings.Contains(pattern, "/") {
			if matched, err := filepath.Match(pattern, path); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// FindEntry is one walked path.
type FindEntry struct {
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size,omitempty"`
}

// FindResult carries a bounded glob walk.
type FindResult struct {
	Entries  []FindEntry `json:"entries"`
	Complete bool        `json:"complete"`
	Scanned  int         `json:"scanned"`
}

// Find walks root for names matching pattern.
func Find(ctx context.Context, root, pattern string, exclude []string, limit int) (FindResult, error) {
	if root == "" {
		return FindResult{}, errors.New("find root is empty")
	}
	if pattern == "" {
		return FindResult{}, errors.New("find pattern is empty")
	}
	if _, err := filepath.Match(pattern, "probe"); err != nil {
		return FindResult{}, fmt.Errorf("%w: glob %q", ErrInvalidPattern, pattern)
	}
	if limit <= 0 {
		limit = 200
	}
	result := FindResult{Complete: true}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				if entry != nil && entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || excluded(path, exclude) {
				return fs.SkipDir
			}
			return nil
		}
		result.Scanned++
		matched, matchErr := filepath.Match(pattern, entry.Name())
		if matchErr != nil {
			return fmt.Errorf("%w: glob %q", ErrInvalidPattern, pattern)
		}
		if !matched {
			return nil
		}
		if len(result.Entries) >= limit {
			result.Complete = false
			return fs.SkipAll
		}
		item := FindEntry{Path: path}
		if info, infoErr := entry.Info(); infoErr == nil {
			item.Size = info.Size()
		}
		result.Entries = append(result.Entries, item)
		return nil
	})
	if err != nil {
		return FindResult{}, err
	}
	return result, nil
}
