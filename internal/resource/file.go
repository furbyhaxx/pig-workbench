package resource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	return pageDirectory(ctx, entries, cursor, limit, func(entry fs.DirEntry) Entry {
		item := directoryEntry(entry, FileURI(filepath.Join(path, entry.Name())))
		if item.Kind == EntryLink {
			item.Target, _ = os.Readlink(filepath.Join(path, entry.Name()))
		}
		return item
	})
}

func pageDirectory(ctx context.Context, entries []fs.DirEntry, cursor string, limit int, makeEntry func(fs.DirEntry) Entry) (Listing, error) {
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
		page.Entries = append(page.Entries, makeEntry(entry))
	}
	return page, nil
}

func directoryEntry(entry fs.DirEntry, uri string) Entry {
	item := Entry{Name: entry.Name(), URI: uri}
	switch {
	case entry.IsDir():
		item.Kind = EntryDir
	case entry.Type()&os.ModeSymlink != 0:
		item.Kind = EntryLink
	default:
		item.Kind = EntryFile
		if info, err := entry.Info(); err == nil {
			item.Size = info.Size()
		}
	}
	return item
}

// Read pages a regular UTF-8 file, or lists a directory as a stable view.
func (FileProvider) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	path, err := localPath(request.URI)
	if err != nil {
		return ReadResult{}, err
	}
	if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
		return readDirectory(ctx, path, request)
	}
	page, err := ReadFile(ctx, Target{Requested: request.Requested, Scheme: "file", Path: path, URI: request.URI, Selection: request.Selector}, request.Budget)
	if err != nil {
		return ReadResult{}, err
	}
	return ReadResult{Content: page.Content, Facts: page}, nil
}

func localPath(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", errors.New("invalid file URI encoding")
	}
	if parsed.Scheme != "file" || parsed.Opaque != "" {
		return "", errors.New("file provider requires a file:// URI")
	}
	if parsed.User != nil || parsed.Host != "" && parsed.Host != "localhost" {
		return "", errors.New("file URI must be local")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", errors.New("file URI does not accept a query or fragment")
	}
	if !filepath.IsAbs(parsed.Path) {
		return "", errors.New("file URI requires an absolute path")
	}
	if strings.ContainsRune(parsed.Path, 0) {
		return "", errors.New("file path contains NUL")
	}
	return filepath.Clean(parsed.Path), nil
}

// readDirectory renders a directory as a stable, pageable listing view.
func readDirectory(ctx context.Context, path string, request ReadRequest) (ReadResult, error) {
	if request.Selector.Raw {
		return ReadResult{}, errors.New("raw is not available for a directory; use :N-M over the listing")
	}
	if len(request.Selector.Ranges)+len(request.Selector.Tails) > 1 {
		return ReadResult{}, errors.New("a directory listing accepts one range or tail, not multiple ranges")
	}
	listing, err := (FileProvider{}).List(ctx, FileURI(path), "", 2000)
	if err != nil {
		return ReadResult{}, err
	}
	var body strings.Builder
	for _, entry := range listing.Entries {
		switch entry.Kind {
		case EntryDir:
			body.WriteString(entry.Name + "/\n")
		case EntryLink:
			body.WriteString(entry.Name + " -> " + entry.Target + "\n")
		default:
			if entry.Size > 0 {
				body.WriteString(entry.Name + " (" + strconv.FormatInt(entry.Size, 10) + " bytes)\n")
			} else {
				body.WriteString(entry.Name + "\n")
			}
		}
	}
	if !listing.Complete {
		body.WriteString("… more entries not listed\n")
	}
	page, err := RenderText(ctx, request.URI, request.Requested, []byte(body.String()), request.Selector, request.Budget)
	if err != nil {
		return ReadResult{}, err
	}
	page.CoordinateSpace = "directory-lines"
	page.ResourceComplete = page.ResourceComplete && listing.Complete
	return ReadResult{Content: page.Content, Facts: page}, nil
}
