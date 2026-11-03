package resource

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// OutStore owns spilled tool outputs for one session.
type OutStore struct {
	mu       sync.Mutex
	dir      string
	maxBytes int64
	used     int64
	order    []outEntry
}

type outEntry struct {
	id   string
	size int64
}

// NewOutStore prepares a private spill directory with a total budget.
func NewOutStore(dir string, maxBytes int64) (*OutStore, error) {
	if !filepath.IsAbs(dir) || maxBytes < 1024 {
		return nil, errors.New("out store needs an absolute directory and a positive budget")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create spill directory: %w", err)
	}
	return &OutStore{dir: dir, maxBytes: maxBytes}, nil
}

// Spill writes one bounded output and returns its immutable out:// URI.
func (s *OutStore) Spill(ctx context.Context, data []byte) (string, error) {
	if int64(len(data)) > s.maxBytes {
		return "", fmt.Errorf("output of %d bytes exceeds the %d-byte spill budget", len(data), s.maxBytes)
	}
	var token [6]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(token[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.used+int64(len(data)) > s.maxBytes && len(s.order) > 0 {
		oldest := s.order[0]
		s.order = s.order[1:]
		if err := os.Remove(filepath.Join(s.dir, oldest.id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("evict spilled output: %w", err)
		}
		s.used -= oldest.size
	}
	if err := os.WriteFile(filepath.Join(s.dir, id), data, 0600); err != nil {
		return "", fmt.Errorf("write spilled output: %w", err)
	}
	s.used += int64(len(data))
	s.order = append(s.order, outEntry{id: id, size: int64(len(data))})
	return "out://" + id, nil
}

// Path maps an out:// URI to the private file, refusing traversal.
func (s *OutStore) Path(uri string) (string, error) {
	id, ok := strings.CutPrefix(uri, "out://")
	if !ok || id == "" || strings.ContainsAny(id, "/\\.") {
		return "", errors.New("out:// takes a single opaque identifier")
	}
	return filepath.Join(s.dir, id), nil
}

// OutProvider serves spilled outputs as immutable files.
type OutProvider struct{ Store *OutStore }

// Scheme returns the canonical out scheme.
func (OutProvider) Scheme() string { return "out" }

// Stat describes one spilled output.
func (p OutProvider) Stat(ctx context.Context, uri string) (StatResult, error) {
	path, err := p.Store.Path(uri)
	if err != nil {
		return StatResult{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return StatResult{}, ErrNotFound
	}
	if err != nil {
		return StatResult{}, err
	}
	return StatResult{URI: uri, Kind: EntryFile, Size: info.Size()}, nil
}

// List reports that spilled outputs are addressed directly, not enumerated.
func (OutProvider) List(context.Context, string, string, int) (Listing, error) {
	return Listing{}, ErrUnsupported
}

// Read pages one spilled output.
func (p OutProvider) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	path, err := p.Store.Path(request.URI)
	if err != nil {
		return ReadResult{}, err
	}
	page, err := ReadFile(ctx, Target{Requested: request.URI, Scheme: "file", Path: path, URI: request.URI, Selection: request.Selector}, request.Budget)
	if err != nil {
		return ReadResult{}, err
	}
	page.CanonicalURI = request.URI
	return ReadResult{Content: page.Content, Facts: page}, nil
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
	rest := strings.TrimPrefix(uri, "local:")
	rest = strings.TrimPrefix(rest, "//")
	if rest == "" || rest == "/" {
		return p.root, nil
	}
	rest = strings.TrimPrefix(rest, "/")
	clean := filepath.Clean(rest)
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
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
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return StatResult{}, ErrNotFound
	}
	if err != nil {
		return StatResult{}, err
	}
	kind := EntryFile
	if info.IsDir() {
		kind = EntryDir
	}
	return StatResult{URI: uri, Kind: kind, Size: info.Size()}, nil
}

// List yields one bounded scratch page.
func (p *LocalProvider) List(ctx context.Context, uri, cursor string, limit int) (Listing, error) {
	path, err := p.path(uri)
	if err != nil {
		return Listing{}, err
	}
	return FileProvider{}.List(ctx, FileURI(path), cursor, limit)
}

// Read pages one scratch file.
func (p *LocalProvider) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	path, err := p.path(request.URI)
	if err != nil {
		return ReadResult{}, err
	}
	page, err := ReadFile(ctx, Target{Requested: request.URI, Scheme: "file", Path: path, URI: request.URI, Selection: request.Selector}, request.Budget)
	if err != nil {
		return ReadResult{}, err
	}
	page.CanonicalURI = "local://" + strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(p.root))
	return ReadResult{Content: page.Content, Facts: page}, nil
}
