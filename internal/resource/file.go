package resource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileProvider serves local files and directories under file://.
type FileProvider struct{}

// Scheme returns the canonical file scheme.
func (FileProvider) Scheme() string { return "file" }

// Stat describes a local path.
func (FileProvider) Stat(ctx context.Context, uri string) (StatResult, error) {
	path, err := localPath(uri)
	if err != nil {
		return StatResult{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return StatResult{}, ErrNotFound
	}
	if err != nil {
		return StatResult{}, fmt.Errorf("stat resource: %w", err)
	}
	result := StatResult{URI: FileURI(path), Size: info.Size()}
	if info.IsDir() {
		result.Kind = EntryDir
	} else {
		result.Kind = EntryFile
	}
	return result, nil
}

// List yields one bounded, directories-first page.
func (FileProvider) List(ctx context.Context, uri, cursor string, limit int) (Listing, error) {
	path, err := localPath(uri)
	if err != nil {
		return Listing{}, err
	}
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Listing{}, ErrNotFound
	}
	if err != nil {
		return Listing{}, fmt.Errorf("list directory: %w", err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	page := Listing{Entries: make([]Entry, 0, min(limit, len(entries))), Complete: true}
	start := 0
	if cursor != "" {
		start = len(entries)
		for i, entry := range entries {
			if entry.Name() == cursor {
				start = i + 1
				break
			}
		}
	}
	for _, entry := range entries[start:] {
		if err := ctx.Err(); err != nil {
			return Listing{}, err
		}
		if len(page.Entries) == limit {
			page.Complete = false
			page.NextCursor = page.Entries[len(page.Entries)-1].Name
			break
		}
		item := Entry{Name: entry.Name(), URI: FileURI(filepath.Join(path, entry.Name()))}
		switch {
		case entry.IsDir():
			item.Kind = EntryDir
		case entry.Type()&os.ModeSymlink != 0:
			item.Kind = EntryLink
			if target, err := os.Readlink(filepath.Join(path, entry.Name())); err == nil {
				item.Target = target
			}
		default:
			item.Kind = EntryFile
			if info, err := entry.Info(); err == nil {
				item.Size = info.Size()
			}
		}
		page.Entries = append(page.Entries, item)
	}
	return page, nil
}

// Read pages a regular UTF-8 file.
func (FileProvider) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	path, err := localPath(request.URI)
	if err != nil {
		return ReadResult{}, err
	}
	page, err := ReadFile(ctx, Target{Requested: request.URI, Scheme: "file", Path: path, URI: request.URI, Selection: request.Selector}, request.Budget)
	if err != nil {
		return ReadResult{}, err
	}
	return ReadResult{Content: page.Content, Facts: page}, nil
}

func localPath(uri string) (string, error) {
	trimmed, ok := strings.CutPrefix(uri, "file://")
	if !ok {
		return "", errors.New("file provider requires a file:// URI")
	}
	if !filepath.IsAbs(trimmed) {
		return "", errors.New("file URI requires an absolute path")
	}
	if strings.ContainsRune(trimmed, 0) {
		return "", errors.New("file path contains NUL")
	}
	return filepath.Clean(trimmed), nil
}
