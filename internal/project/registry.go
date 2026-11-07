package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Location describes one observed checkout of a project.
type Location struct {
	Host     string    `toml:"host"`
	Path     string    `toml:"path"`
	Kind     string    `toml:"kind"`
	LastSeen time.Time `toml:"last_seen"`
}

// Entry combines a project with its observed locations and private registry directory.
type Entry struct {
	Project     Project
	Locations   []Location
	RegistryDir string
}

// Registry stores project records under an agent directory.
type Registry struct {
	dir  string
	info os.FileInfo
}

// Open opens or creates the private projects directory under agentDir.
func Open(agentDir string) (*Registry, error) {
	if agentDir == "" {
		return nil, errors.New("agent directory must not be empty")
	}
	if err := registryPlatformSupport(); err != nil {
		return nil, err
	}
	dir, err := filepath.Abs(filepath.Join(agentDir, "projects"))
	if err != nil {
		return nil, fmt.Errorf("resolve project registry: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create project registry: %w", err)
	}
	info, err := privateDirectory(dir)
	if err != nil {
		return nil, err
	}
	return &Registry{dir: dir, info: info}, nil
}

// Register saves the project and adds or updates a location identified by host and path.
func (r *Registry) Register(project Project, location Location) error {
	if err := validate("project", project); err != nil {
		return err
	}
	if err := validateLocation("location", location); err != nil {
		return err
	}
	if location.LastSeen.IsZero() {
		location.LastSeen = time.Now()
	}
	location.LastSeen = location.LastSeen.UTC()
	lock, err := r.lock()
	if err != nil {
		return err
	}
	defer lock.Close()

	dir := filepath.Join(r.dir, project.ID)
	if err := os.Mkdir(dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create project registry entry: %w", err)
	}
	if _, err := privateDirectory(dir); err != nil {
		return err
	}
	if err := syncRegistryDirectory(r.dir); err != nil {
		return fmt.Errorf("flush project registry directory: %w", err)
	}
	previous, err := r.readEntry(project.ID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	locations := previous.Locations
	updated := false
	for i := range locations {
		if locations[i].Host == location.Host && locations[i].Path == location.Path {
			locations[i] = location
			updated = true
			break
		}
	}
	if !updated {
		locations = append(locations, location)
	}
	path := filepath.Join(dir, "project.toml")
	data, err := encodeRegistryDocument(path, project, locations)
	if err != nil {
		return err
	}
	if err := writeRegistryFile(path, data); err != nil {
		return err
	}
	entries, _, err := r.scan()
	if err != nil {
		return fmt.Errorf("project saved but registry index could not be updated: %w", err)
	}
	if err := r.writeIndex(entries, false); err != nil {
		return fmt.Errorf("project saved but registry index could not be updated: %w", err)
	}
	return nil
}

// List returns valid entries and any problems encountered in other project records.
func (r *Registry) List() ([]Entry, error) {
	lock, err := r.lock()
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	entries, problems, err := r.scan()
	if err != nil {
		return nil, err
	}
	if err := r.writeIndex(entries, false); err != nil {
		problems = append(problems, err)
	}
	return entries, errors.Join(problems...)
}

// Lookup finds one project without being affected by other invalid records.
func (r *Registry) Lookup(id string) (Entry, bool, error) {
	if !idPattern.MatchString(id) {
		return Entry{}, false, &Error{Key: "id", Reason: "expected prj- followed by lowercase letters or digits"}
	}
	lock, err := r.lock()
	if err != nil {
		return Entry{}, false, err
	}
	defer lock.Close()
	entry, err := r.readEntry(id)
	if errors.Is(err, os.ErrNotExist) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}

// PrivateNotesRoot returns an existing private notes directory for the project, or an empty path if absent.
func (r *Registry) PrivateNotesRoot(id string) (string, error) {
	if id != "global" && !idPattern.MatchString(id) {
		return "", &Error{Key: "id", Reason: "expected global or prj- followed by lowercase letters or digits"}
	}
	if err := r.checkDirectory(); err != nil {
		return "", err
	}
	entry := filepath.Join(r.dir, id)
	if _, err := privateDirectory(entry); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	private := filepath.Join(entry, "private")
	if _, err := privateDirectory(private); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return private, nil
}

// Rebuild replaces the cache from valid project records and reports invalid records.
func (r *Registry) Rebuild() error {
	lock, err := r.lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	entries, problems, err := r.scan()
	if err != nil {
		return err
	}
	if err := r.writeIndex(entries, true); err != nil {
		problems = append(problems, err)
	}
	return errors.Join(problems...)
}

func validateLocation(path string, location Location) error {
	if location.Path == "" {
		return &Error{Path: path, Key: "locations.path", Reason: "path is required"}
	}
	switch location.Kind {
	case "main", "worktree", "clone", "ssh":
		return nil
	default:
		return &Error{Path: path, Key: "locations.kind", Reason: "expected main, worktree, clone, or ssh"}
	}
}

func (r *Registry) scan() ([]Entry, []error, error) {
	return r.scanContext(context.Background())
}

func (r *Registry) scanContext(ctx context.Context) ([]Entry, []error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	dirs, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, nil, fmt.Errorf("list project registry: %w", err)
	}
	entries := make([]Entry, 0)
	var problems []error
	for _, dir := range dirs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		id := dir.Name()
		if !strings.HasPrefix(id, "prj-") {
			continue
		}
		if !idPattern.MatchString(id) {
			problems = append(problems, &Error{Path: filepath.Join(r.dir, id), Key: "id", Reason: "invalid project directory name"})
			continue
		}
		entry, err := r.readEntry(id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				err = &Error{Path: filepath.Join(r.dir, id, "project.toml"), Reason: "project document is missing"}
			}
			problems = append(problems, err)
			continue
		}
		entries = append(entries, entry)
	}
	return entries, problems, nil
}
