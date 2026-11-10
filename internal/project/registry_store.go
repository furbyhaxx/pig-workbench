package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/gofrs/flock"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

type registryDocument struct {
	Project
	Version   int        `toml:"version"`
	Locations []Location `toml:"locations"`
}

var registryKeys = map[string]bool{
	"locations": true, "locations.host": true, "locations.path": true,
	"locations.kind": true, "locations.last_seen": true,
}

type registryIndex struct {
	Version  int                 `toml:"version"`
	Projects []registryIndexItem `toml:"projects"`
}

type registryIndexItem struct {
	ID        string     `toml:"id"`
	Slug      string     `toml:"slug"`
	Remotes   []string   `toml:"remotes"`
	Locations []Location `toml:"locations"`
}

func privateDirectory(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect project directory %s: %w", path, err)
	}
	if !info.IsDir() {
		return nil, &Error{Path: path, Reason: "expected a directory, not a symlink"}
	}
	if err := checkRegistryPrivate(path, info, 0700); err != nil {
		return nil, err
	}
	return info, nil
}

func (r *Registry) checkDirectory() error {
	info, err := privateDirectory(r.dir)
	if err != nil {
		return err
	}
	if !os.SameFile(r.info, info) {
		return &Error{Path: r.dir, Reason: "project registry directory identity changed"}
	}
	return nil
}

func (r *Registry) lock() (*flock.Flock, error) {
	if err := r.checkDirectory(); err != nil {
		return nil, err
	}
	path := filepath.Join(r.dir, ".registry.lock")
	lock, err := fileio.Lock(context.Background(), path)
	if errors.Is(err, fileio.ErrLockReplaced) {
		return nil, &Error{Path: path, Reason: "lock must be an unchanged regular file"}
	}
	if err != nil {
		return nil, fmt.Errorf("project registry %w", err)
	}
	fail := func(err error) (*flock.Flock, error) {
		lock.Close()
		return nil, err
	}
	if err := r.checkDirectory(); err != nil {
		return fail(err)
	}
	info, err := lock.Stat()
	if err != nil {
		return fail(fmt.Errorf("inspect project registry lock: %w", err))
	}
	if err := checkRegistryPrivate(path, info, 0600); err != nil {
		return fail(err)
	}
	return lock, nil
}

func readRegistryFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect project registry file %s: %w", path, err)
	}
	if !before.Mode().IsRegular() {
		return nil, &Error{Path: path, Reason: "expected a regular file, not a symlink"}
	}
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		return nil, fmt.Errorf("open project registry file %s: %w", path, err)
	}
	defer file.Close()
	if !os.SameFile(before, info) {
		return nil, &Error{Path: path, Reason: "file identity changed while opening"}
	}
	if err := checkRegistryPrivate(path, info, 0600); err != nil {
		return nil, err
	}
	if info.Size() > maxDocumentBytes {
		return nil, &Error{Path: path, Reason: "project document exceeds 1 MiB"}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read project registry file %s: %w", path, err)
	}
	if len(data) > maxDocumentBytes {
		return nil, &Error{Path: path, Reason: "project document exceeds 1 MiB"}
	}
	return data, nil
}

func decodeRegistryDocument(path string, data []byte) (registryDocument, error) {
	defaults := defaultDocument()
	stored := registryDocument{Project: defaults.Project, Version: defaults.Version}
	meta, err := decodeDocument(path, string(data), &stored, registryKeys)
	if err != nil {
		return registryDocument{}, err
	}
	if err := validateDocument(path, document{Project: stored.Project, Version: stored.Version}, meta); err != nil {
		return registryDocument{}, err
	}
	seen := make(map[[2]string]bool, len(stored.Locations))
	for _, location := range stored.Locations {
		if err := validateLocation(path, location); err != nil {
			return registryDocument{}, err
		}
		key := [2]string{location.Host, location.Path}
		if seen[key] {
			return registryDocument{}, &Error{Path: path, Key: "locations", Reason: "duplicate host and path"}
		}
		seen[key] = true
	}
	return stored, nil
}

func encodeRegistryDocument(path string, project Project, locations []Location) ([]byte, error) {
	stored := registryDocument{Project: project, Version: 1, Locations: locations}
	var output bytes.Buffer
	if err := toml.NewEncoder(&output).Encode(stored); err != nil {
		return nil, &Error{Path: path, Reason: "cannot encode project document"}
	}
	data := output.Bytes()
	if _, err := decodeRegistryDocument(path, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *Registry) readEntry(id string) (Entry, error) {
	dir := filepath.Join(r.dir, id)
	if _, err := privateDirectory(dir); err != nil {
		return Entry{}, err
	}
	path := filepath.Join(dir, "project.toml")
	data, err := readRegistryFile(path)
	if err != nil {
		return Entry{}, err
	}
	stored, err := decodeRegistryDocument(path, data)
	if err != nil {
		return Entry{}, err
	}
	if stored.ID != id {
		return Entry{}, &Error{Path: path, Key: "id", Reason: "id does not match its registry directory"}
	}
	return Entry{Project: stored.Project, Locations: stored.Locations, RegistryDir: dir}, nil
}

func (r *Registry) writeIndex(entries []Entry, force bool) error {
	index := registryIndex{Version: 1, Projects: make([]registryIndexItem, 0, len(entries))}
	for _, entry := range entries {
		item := registryIndexItem{ID: entry.Project.ID, Slug: entry.Project.Slug, Locations: entry.Locations}
		if entry.Project.Component.DefaultRemote != "" {
			item.Remotes = append(item.Remotes, entry.Project.Component.DefaultRemote)
		}
		for _, component := range entry.Project.Components {
			if component.DefaultRemote != "" {
				item.Remotes = append(item.Remotes, component.DefaultRemote)
			}
		}
		index.Projects = append(index.Projects, item)
	}
	var output bytes.Buffer
	if err := toml.NewEncoder(&output).Encode(index); err != nil {
		return errors.New("cannot encode project registry index")
	}
	path := filepath.Join(r.dir, "index.toml")
	if !force {
		matches, err := registryCacheMatches(path, output.Bytes())
		if err != nil {
			return err
		}
		if matches {
			return nil
		}
	}
	return writeRegistryFile(path, output.Bytes())
}

func registryCacheMatches(path string, expected []byte) (bool, error) {
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect project registry index: %w", err)
	}
	if !before.Mode().IsRegular() {
		return false, &Error{Path: path, Reason: "index must be a regular file"}
	}
	if err := checkRegistryPrivate(path, before, 0600); err != nil {
		return false, err
	}
	if before.Size() != int64(len(expected)) {
		return false, nil
	}
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		return false, fmt.Errorf("open project registry index: %w", err)
	}
	defer file.Close()
	if !os.SameFile(before, info) {
		return false, &Error{Path: path, Reason: "index identity changed while opening"}
	}
	if err := checkRegistryPrivate(path, info, 0600); err != nil {
		return false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(len(expected))+1))
	if err != nil {
		return false, fmt.Errorf("read project registry index: %w", err)
	}
	return bytes.Equal(data, expected), nil
}

func writeRegistryFile(path string, data []byte) error {
	if _, err := fileio.WriteAtomic(context.Background(), path, data, 0600); err != nil {
		return fmt.Errorf("project registry file: %w", err)
	}
	return nil
}
