package resource

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// PageEntries returns one listing page of at most limit entries starting at the entry named cursor.
// A cursor whose entry is gone is stale: answering from the start would repeat entries.
func PageEntries(entries []Entry, cursor string, limit int) (Listing, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	start := 0
	if cursor != "" {
		start = slices.IndexFunc(entries, func(entry Entry) bool { return entry.Name == cursor })
		if start < 0 {
			return Listing{}, errors.New("staleRevision: listing changed or cursor entry disappeared; restart listing")
		}
	}
	end := min(start+limit, len(entries))
	listing := Listing{Entries: entries[start:end], Complete: end == len(entries)}
	if !listing.Complete {
		listing.NextCursor = entries[end].Name
	}
	return listing, nil
}

// LocalProvider serves the per-session scratch directory under local://.
type LocalProvider struct{ root string }

// NewLocalProvider prepares a private scratch root with a maximum size check.
func NewLocalProvider(root string) (*LocalProvider, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("local scratch root must be absolute")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, fmt.Errorf("create local scratch: %w", err)
	}
	return &LocalProvider{root: root}, nil
}

// Scheme returns the canonical local scheme.
func (*LocalProvider) Scheme() string { return "local" }

func (p *LocalProvider) path(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "local" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("invalid local:// URI")
	}
	rest := u.Host + u.Path
	if u.Opaque != "" {
		rest, err = url.PathUnescape(u.Opaque)
		if err != nil {
			return "", errors.New("invalid local:// path encoding")
		}
	}
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" {
		return p.root, nil
	}
	clean := filepath.Clean(filepath.FromSlash(rest))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) || strings.ContainsRune(clean, 0) {
		return "", errors.New("local:// path must stay inside the session scratch")
	}
	return filepath.Join(p.root, clean), nil
}

// Stat describes one scratch entry.
func (p *LocalProvider) Stat(ctx context.Context, uri string) (StatResult, error) {
	path, err := p.path(uri)
	if err != nil {
		return StatResult{}, err
	}
	rel, err := filepath.Rel(p.root, path)
	if err != nil {
		return StatResult{}, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return StatResult{}, err
	}
	defer root.Close()
	info, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return StatResult{}, ErrNotFound
	}
	if err != nil {
		return StatResult{}, err
	}
	kind := EntryFile
	if info.IsDir() {
		kind = EntryDir
	} else if info.Mode()&os.ModeSymlink != 0 {
		kind = EntryLink
	}
	return StatResult{URI: uri, Kind: kind, Size: info.Size()}, nil
}

// List yields one bounded scratch page.
func (p *LocalProvider) List(ctx context.Context, uri, cursor string, limit int) (Listing, error) {
	path, err := p.path(uri)
	if err != nil {
		return Listing{}, err
	}
	rel, err := filepath.Rel(p.root, path)
	if err != nil {
		return Listing{}, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return Listing{}, err
	}
	defer root.Close()
	before, err := root.Lstat(rel)
	if errors.Is(err, os.ErrNotExist) {
		return Listing{}, ErrNotFound
	}
	if err != nil {
		return Listing{}, err
	}
	if !before.IsDir() {
		return Listing{}, errors.New("scratch listing target is not a directory")
	}
	dir, opened, err := fileio.OpenDirectoryInRoot(root, rel)
	if err != nil {
		return Listing{}, err
	}
	defer dir.Close()
	if !opened.IsDir() || !os.SameFile(before, opened) {
		return Listing{}, errors.New("scratch directory changed while opening; retry")
	}
	entries, err := dir.ReadDir(5001)
	if err != nil && !errors.Is(err, io.EOF) {
		return Listing{}, err
	}
	if len(entries) > 5000 {
		return Listing{}, errors.New("scratch directory exceeds the 5000-entry listing limit")
	}
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	page, err := pageDirectory(ctx, entries, FileURI(path), cursor, limit, func(entry fs.DirEntry) Entry {
		name := filepath.ToSlash(filepath.Join(rel, entry.Name()))
		child := (&url.URL{Scheme: "local", Path: "/" + name}).String()
		return directoryEntry(entry, child)
	})
	if err != nil {
		return Listing{}, err
	}
	after, err := dir.Stat()
	if err != nil {
		return Listing{}, err
	}
	current, err := root.Lstat(rel)
	if err != nil || !os.SameFile(opened, current) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return Listing{}, errors.New("staleRevision: scratch directory changed while listing; restart")
	}
	return page, nil
}

// Read pages one scratch file.
func (p *LocalProvider) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	path, err := p.path(request.URI)
	if err != nil {
		return ReadResult{}, err
	}
	rel, err := filepath.Rel(p.root, path)
	if err != nil {
		return ReadResult{}, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return ReadResult{}, err
	}
	defer root.Close()
	file, opened, err := fileio.OpenRegularInRoot(root, rel)
	if err != nil {
		return ReadResult{}, err
	}
	defer file.Close()
	page, err := readOpened(ctx, Target{Requested: request.URI, Selection: request.Selector}, request.Budget, file, opened, request.URI)
	if err != nil {
		return ReadResult{}, err
	}
	return ReadResult{Content: page.Content, Facts: page}, nil
}

// LocalWriteLimit bounds one scratch write.
const LocalWriteLimit = 4 << 20

// Write creates or replaces one scratch file below an existing directory and reports
// its canonical URI. Symlinks, directories and paths outside the scratch root are refused.
func (p *LocalProvider) Write(ctx context.Context, uri string, data []byte) (string, bool, error) {
	if len(data) > LocalWriteLimit {
		return "", false, fmt.Errorf("scratch write of %d bytes exceeds the %d-byte limit", len(data), LocalWriteLimit)
	}
	path, err := p.path(uri)
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(p.root, path)
	if err != nil || rel == "." {
		return "", false, errors.New("local:// write needs a file name below the scratch root")
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return "", false, err
	}
	defer root.Close()
	created := true
	switch info, err := root.Lstat(rel); {
	case err == nil && info.Mode().IsRegular():
		created = false
	case err == nil:
		return "", false, errors.New("local:// target is a directory or link; only regular scratch files can be written")
	case !errors.Is(err, os.ErrNotExist):
		return "", false, err
	}
	if parent := filepath.Dir(rel); parent != "." {
		info, err := root.Stat(parent)
		if err != nil || !info.IsDir() {
			return "", false, errors.New("parent directory does not exist in the scratch; directories are not created")
		}
	}
	var token [6]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", false, err
	}
	temporary := filepath.Join(filepath.Dir(rel), ".write-"+hex.EncodeToString(token[:]))
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", false, fmt.Errorf("create scratch file: %w", err)
	}
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = root.Rename(temporary, rel)
	}
	if writeErr != nil {
		_ = root.Remove(temporary)
		return "", false, fmt.Errorf("write scratch file: %w", writeErr)
	}
	return "local://" + filepath.ToSlash(rel), created, nil
}
