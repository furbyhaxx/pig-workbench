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
	"time"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// ContextLine is one surrounding source line, not a match.
type ContextLine struct {
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Match is one exact source match.
type Match struct {
	Path             string        `json:"path"`
	Revision         string        `json:"revision,omitempty"`
	Line             int           `json:"line"`
	Column           int           `json:"column"`
	Text             string        `json:"text"`
	Windowed         bool          `json:"windowed,omitempty"`
	WindowStart      int           `json:"windowStart,omitempty"`
	ContextTruncated bool          `json:"contextTruncated,omitempty"`
	Before           []ContextLine `json:"before,omitempty"`
	After            []ContextLine `json:"after,omitempty"`
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
	Selection  resource.Selection
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
	if err := validateGlobs(options.Glob, options.Exclude); err != nil {
		return Result{}, err
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
	expressions, literals, err := compilePatterns(options)
	if err != nil {
		return Result{}, err
	}
	if options.Selection.Explicit {
		info, err := os.Stat(options.Root)
		if err != nil {
			return Result{}, err
		}
		if info.IsDir() {
			return Result{}, errors.New("grep line selectors require a file, not a directory")
		}
	}
	ignores, err := newIgnoreTree(ctx, options.Root)
	if err != nil {
		return Result{}, err
	}
	result := Result{Complete: true}
	remainingBytes, remainingLines := 48<<10, 1900
	err = filepath.WalkDir(options.Root, func(path string, entry fs.DirEntry, err error) error {
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
		if entry.IsDir() && path != options.Root && (entry.Name() == ".git" || excluded(options.Root, path, options.Exclude)) {
			return fs.SkipDir
		}
		ignored, err := ignores.visit(ctx, path, entry)
		if err != nil {
			return err
		}
		if ignored {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if options.Glob != "" && !matchGlob(options.Glob, options.Root, path, entry.Name()) {
			return nil
		}
		if path != options.Root && excluded(options.Root, path, options.Exclude) {
			return nil
		}
		if result.FilesScanned >= limits.MaxFiles {
			result.Complete = false
			return fs.SkipAll
		}
		matches, scanned, err := scanFile(ctx, path, options, expressions, literals, limits, limits.MaxMatches-len(result.Matches)+1)
		if err != nil {
			if errors.Is(err, errTooLarge) || errors.Is(err, errBinary) {
				result.Skipped = append(result.Skipped, path)
				return nil
			}
			return err
		}
		result.FilesScanned += scanned
		if !appendMatches(&result, matches, limits.MaxMatches, &remainingBytes, &remainingLines) {
			return fs.SkipAll
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

func appendMatches(result *Result, matches []Match, limit int, remainingBytes, remainingLines *int) bool {
	for _, match := range matches {
		bytes, lines := matchBudget(match)
		for (bytes > *remainingBytes || lines > *remainingLines) && len(match.Before)+len(match.After) > 0 {
			if len(match.Before) >= len(match.After) {
				match.Before = match.Before[1:]
			} else {
				match.After = match.After[:len(match.After)-1]
			}
			match.ContextTruncated = true
			result.Complete = false
			bytes, lines = matchBudget(match)
		}
		if len(result.Matches) >= limit || bytes > *remainingBytes || lines > *remainingLines {
			result.Complete = false
			return false
		}
		match.Text = strings.Clone(match.Text)
		for i := range match.Before {
			match.Before[i].Text = strings.Clone(match.Before[i].Text)
		}
		for i := range match.After {
			match.After[i].Text = strings.Clone(match.After[i].Text)
		}
		result.Matches = append(result.Matches, match)
		if match.Windowed {
			result.Complete = false
		}
		*remainingBytes -= bytes
		*remainingLines -= lines
	}
	return true
}

// GrepProvider searches bounded source pages in a provider tree without following links.
func GrepProvider(ctx context.Context, provider resource.Provider, options Options) (Result, error) {
	if options.Root == "" || len(options.Patterns) == 0 {
		return Result{}, errors.New("search needs a root and at least one pattern")
	}
	if err := validateGlobs(options.Glob, options.Exclude); err != nil {
		return Result{}, err
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
	expressions, literals, err := compilePatterns(options)
	if err != nil {
		return Result{}, err
	}
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := Result{Complete: true}
	remainingBytes, remainingLines, sourceBudget := 48<<10, 1900, int64(16<<20)
	visit := func(entry resource.Entry, _ int) (resource.WalkAction, error) {
		if entry.Name == ".git" || excluded(options.Root, entry.URI, options.Exclude) {
			return resource.WalkSkipDir, nil
		}
		if entry.Kind != resource.EntryFile || entry.NoWalk {
			return resource.WalkContinue, nil
		}
		if options.Glob != "" && !matchGlob(options.Glob, options.Root, entry.URI, entry.Name) {
			return resource.WalkContinue, nil
		}
		if result.FilesScanned >= limits.MaxFiles || sourceBudget <= 0 {
			result.Complete = false
			return resource.WalkStop, nil
		}
		var revision string
		data, complete, readErr := readProviderSource(work, provider, entry.URI, min(limits.MaxFileBytes, sourceBudget), &sourceBudget, &revision)
		if readErr != nil {
			if errors.Is(readErr, resource.ErrUnsupported) || errors.Is(readErr, resource.ErrNotFound) || errors.Is(readErr, errTooLarge) {
				result.Skipped = append(result.Skipped, entry.URI)
				result.Complete = false
				return resource.WalkContinue, nil
			}
			return resource.WalkStop, readErr
		}
		if !complete {
			result.Skipped = append(result.Skipped, entry.URI)
			result.Complete = false
			return resource.WalkContinue, nil
		}
		matches, scanErr := scanBytes(work, entry.URI, data, options, expressions, literals, limits, limits.MaxMatches-len(result.Matches)+1)
		if errors.Is(scanErr, errBinary) {
			result.Skipped = append(result.Skipped, entry.URI)
			result.Complete = false
			return resource.WalkContinue, nil
		}
		if scanErr != nil {
			return resource.WalkStop, scanErr
		}
		result.FilesScanned++
		for i := range matches {
			matches[i].Revision = revision
		}
		if !appendMatches(&result, matches, limits.MaxMatches, &remainingBytes, &remainingLines) {
			return resource.WalkStop, nil
		}
		return resource.WalkContinue, nil
	}
	stat, err := provider.Stat(work, options.Root)
	walkComplete := true
	if err == nil {
		switch stat.Kind {
		case resource.EntryFile:
			_, err = visit(resource.Entry{URI: stat.URI, Name: filepath.Base(stat.URI), Kind: stat.Kind}, 0)
		case resource.EntryDir:
			if options.Selection.Explicit {
				return Result{}, errors.New("grep line selectors require a file, not a directory")
			}
			walk, walkErr := resource.Walk(work, provider, options.Root, visit)
			err, walkComplete = walkErr, walk.Complete
		default:
			return Result{}, errors.New("grep target must be a file or directory, not a link or control")
		}
	}
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		result.Complete = false
		err = nil
	}
	if err != nil {
		return Result{}, err
	}
	result.Complete = result.Complete && walkComplete
	sort.SliceStable(result.Matches, func(i, j int) bool {
		if result.Matches[i].Path != result.Matches[j].Path {
			return result.Matches[i].Path < result.Matches[j].Path
		}
		return result.Matches[i].Line < result.Matches[j].Line
	})
	return result, nil
}

func readProviderSource(ctx context.Context, provider resource.Provider, uri string, limit int64, sourceBudget *int64, sourceRevision *string) ([]byte, bool, error) {
	var data []byte
	next := 1
	revision := ""
	for pages := 0; pages < 256; pages++ {
		selection := resource.Selection{Raw: true}
		if next > 1 {
			selection.Ranges = []resource.Range{{Start: next}}
		}
		page, err := provider.Read(ctx, resource.ReadRequest{URI: uri, Requested: uri, Selector: selection, Budget: resource.Budget{Bytes: 48 << 10, Lines: 1000}})
		if err != nil {
			return nil, false, err
		}
		*sourceBudget -= int64(len(page.Content))
		if *sourceBudget < 0 {
			return data, false, errTooLarge
		}
		facts := page.Facts
		if facts.CanonicalURI != uri {
			return data, false, nil
		}
		if pages > 0 && (revision == "" || facts.Revision != revision) {
			return data, false, nil
		}
		if pages == 0 {
			revision = facts.Revision
			*sourceRevision = revision
		}
		if len(facts.FullyShownRanges) == 0 {
			return data, facts.SelectionComplete && facts.ResourceComplete && len(data) == 0, nil
		}
		if len(facts.FullyShownRanges) != 1 || facts.FullyShownRanges[0].Start != next {
			return data, false, nil
		}
		end := facts.FullyShownRanges[0].End
		if end < next || len(facts.ShownRanges) != 1 || facts.ShownRanges[0] != facts.FullyShownRanges[0] {
			return data, false, nil
		}
		rest := page.Content
		for line := next; line <= end; line++ {
			i := strings.IndexByte(rest, '\n')
			if i < 0 {
				if line != end || !facts.SelectionComplete {
					return data, false, nil
				}
				i = len(rest)
			}
			if int64(len(data)+i+1) > limit {
				return data, false, errTooLarge
			}
			data = append(data, rest[:i]...)
			data = append(data, '\n')
			if i < len(rest) {
				rest = rest[i+1:]
			} else {
				rest = ""
			}
		}
		if facts.SelectionComplete {
			return data, next > 1 || facts.ResourceComplete, nil
		}
		if revision == "" {
			return data, false, nil
		}
		next = end + 1
	}
	return data, false, nil
}

func compilePatterns(options Options) ([]*regexp.Regexp, []string, error) {
	var expressions []*regexp.Regexp
	var literals []string
	for _, pattern := range options.Patterns {
		if pattern == "" {
			if options.Literal {
				return nil, nil, errors.New("literal pattern is empty")
			}
			return nil, nil, errors.New("regex pattern is empty")
		}
		if options.Literal && !options.IgnoreCase {
			literals = append(literals, pattern)
			continue
		}
		if options.Literal {
			pattern = regexp.QuoteMeta(pattern)
		}
		if options.IgnoreCase {
			pattern = "(?i)" + pattern
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %v", ErrInvalidPattern, err)
		}
		expressions = append(expressions, compiled)
	}
	return expressions, literals, nil
}

var (
	errTooLarge = errors.New("file exceeds the search byte limit")
	errBinary   = errors.New("file looks binary")
)

func matchBudget(match Match) (int, int) {
	lines := 1 + len(match.Before) + len(match.After)
	bytes := lines*(len(match.Path)+64) + len(match.Text)
	for _, line := range match.Before {
		bytes += len(line.Text)
	}
	for _, line := range match.After {
		bytes += len(line.Text)
	}
	return bytes, lines
}

func scanFile(ctx context.Context, path string, options Options, expressions []*regexp.Regexp, literals []string, limits Limits, matchLimit int) ([]Match, int, error) {
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
	matches, err := scanBytes(ctx, path, data, options, expressions, literals, limits, matchLimit)
	return matches, 1, err
}

func scanBytes(ctx context.Context, path string, data []byte, options Options, expressions []*regexp.Regexp, literals []string, limits Limits, matchLimit int) ([]Match, error) {
	if looksBinary(data) {
		return nil, errBinary
	}
	var matches []Match
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	spans, err := options.Selection.Resolve(len(lines), false)
	if err != nil {
		return nil, err
	}
	for index, line := range lines {
		if options.Selection.Explicit {
			selected := false
			for _, span := range spans {
				selected = selected || index+1 >= span.Start && index+1 <= span.End
			}
			if !selected {
				continue
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		column := hit(line, expressions, literals)
		if column == 0 {
			continue
		}
		if options.FilesOnly {
			matches = append(matches, Match{Path: path, Line: index + 1, Column: column})
			break
		}
		match := Match{Path: path, Line: index + 1, Column: column, Text: strings.TrimSuffix(line, "\r")}
		const windowBytes = 8192
		if len(match.Text) > windowBytes {
			start := min(max(0, column-1-windowBytes/2), len(match.Text)-windowBytes)
			for start > 0 && !utf8.RuneStart(match.Text[start]) {
				start--
			}
			end := min(len(match.Text), start+windowBytes)
			for end < len(match.Text) && !utf8.RuneStart(match.Text[end]) {
				end--
			}
			match.Text = match.Text[start:end]
			match.Windowed = true
			match.WindowStart = start + 1
		}
		for i := max(0, index-limits.Context); i < index; i++ {
			match.Before = append(match.Before, ContextLine{Line: i + 1, Text: strings.TrimSuffix(lines[i], "\r")})
		}
		for i := index + 1; i < min(len(lines), index+limits.Context+1); i++ {
			match.After = append(match.After, ContextLine{Line: i + 1, Text: strings.TrimSuffix(lines[i], "\r")})
		}
		matches = append(matches, match)
		if len(matches) >= matchLimit {
			break
		}
	}
	return matches, nil
}

func hit(line string, expressions []*regexp.Regexp, literals []string) int {
	for _, expression := range expressions {
		if location := expression.FindStringIndex(line); location != nil {
			return location[0] + 1
		}
	}
	for _, literal := range literals {
		if index := strings.Index(line, literal); index >= 0 {
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

// FindProvider walks a provider root for names matching pattern.
func FindProvider(ctx context.Context, provider resource.Provider, root, pattern string, exclude []string, limit int) (FindResult, error) {
	if pattern == "" {
		return FindResult{}, errors.New("find pattern is empty")
	}
	if err := validateGlobs(pattern, exclude); err != nil {
		return FindResult{}, err
	}
	if limit <= 0 {
		limit = 200
	}
	result := FindResult{}
	remaining := 48 << 10
	walk, err := resource.Walk(ctx, provider, root, func(entry resource.Entry, _ int) (resource.WalkAction, error) {
		if entry.Name == ".git" || excluded(root, entry.URI, exclude) {
			return resource.WalkSkipDir, nil
		}
		if entry.Kind == resource.EntryDir {
			return resource.WalkContinue, nil
		}
		if !matchGlob(pattern, root, entry.URI, entry.Name) {
			return resource.WalkContinue, nil
		}
		if len(result.Entries) >= limit || len(entry.URI)+1 > remaining {
			return resource.WalkStop, nil
		}
		result.Entries = append(result.Entries, FindEntry{Path: entry.URI, Size: entry.Size})
		remaining -= len(entry.URI) + 1
		return resource.WalkContinue, nil
	})
	if err != nil {
		return FindResult{}, err
	}
	result.Scanned, result.Complete = walk.Scanned, walk.Complete
	return result, nil
}

// Find walks root for names matching pattern.
func Find(ctx context.Context, root, pattern string, exclude []string, limit int) (FindResult, error) {
	if root == "" {
		return FindResult{}, errors.New("find root is empty")
	}
	if pattern == "" {
		return FindResult{}, errors.New("find pattern is empty")
	}
	if err := validateGlobs(pattern, exclude); err != nil {
		return FindResult{}, err
	}
	if limit <= 0 {
		limit = 200
	}
	ignores, err := newIgnoreTree(ctx, root)
	if err != nil {
		return FindResult{}, err
	}
	result := FindResult{Complete: true}
	remaining := 48 << 10
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
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
		if path != root && entry.IsDir() && (entry.Name() == ".git" || excluded(root, path, exclude)) {
			return fs.SkipDir
		}
		ignored, err := ignores.visit(ctx, path, entry)
		if err != nil {
			return err
		}
		if ignored {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || path != root && excluded(root, path, exclude) {
			return nil
		}
		result.Scanned++
		if !matchGlob(pattern, root, path, entry.Name()) {
			return nil
		}
		if len(result.Entries) >= limit || len(path)+1 > remaining {
			result.Complete = false
			return fs.SkipAll
		}
		remaining -= len(path) + 1
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
