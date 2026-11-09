package project

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// markerKey is the repository-local, never committed git config key naming the project a repository belongs to (SPEC-0012 §3.3).
const markerKey = "pig.projectId"

// JoinMode selects how a repository records its membership of an existing project.
type JoinMode string

const (
	// JoinFile writes .project/Project.toml into the repository.
	JoinFile JoinMode = "file"
	// JoinMarker records only the local git config marker, leaving the repository's files untouched.
	JoinMarker JoinMode = "marker"
)

// NewID returns a fresh project ID: "prj-" and ten random lowercase letters or digits.
func NewID() (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	id := make([]byte, 10)
	for i := range id {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", fmt.Errorf("generate project id: %w", err)
		}
		id[i] = alphabet[n.Int64()]
	}
	return "prj-" + string(id), nil
}

// SlugFromName derives a kebab slug from free text ("My Project!" becomes "my-project"); empty when nothing usable remains.
func SlugFromName(name string) string {
	var out []rune
	pending := false
	for _, r := range strings.ToLower(name) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			if pending && len(out) > 0 {
				out = append(out, '-')
			}
			out, pending = append(out, r), false
		} else {
			pending = true
		}
	}
	return string(out)
}

// repositoryTop is the git top-level directory of dir, or the resolved dir outside git.
func repositoryTop(ctx context.Context, dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	if absolute, err = filepath.EvalSymlinks(absolute); err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	root, err := gitRoot(ctx, absolute)
	if err != nil {
		return "", err
	}
	if root == "" {
		return absolute, nil
	}
	return root, nil
}

func locationAt(root string) Location {
	kind := "main"
	if info, err := os.Lstat(filepath.Join(root, ".git")); err == nil && !info.IsDir() {
		kind = "worktree"
	}
	return Location{Host: "local", Path: root, Kind: kind}
}

// CreateProject creates a new project rooted at the repository containing dir. It generates the ID, refuses a slug
// another registered project uses, writes .project/Project.toml (never replacing one) and registers the location.
func (r *Registry) CreateProject(ctx context.Context, dir, slug, name, description string) (Entry, error) {
	if !slugPattern.MatchString(slug) {
		return Entry{}, &Error{Path: "project", Key: "slug", Reason: "expected a lowercase kebab slug"}
	}
	entries, problems, err := r.scanContext(ctx)
	if err != nil {
		return Entry{}, err
	}
	if err := errors.Join(problems...); err != nil {
		return Entry{}, fmt.Errorf("the project registry has invalid records; repair it first: %w", err)
	}
	for _, entry := range entries {
		if entry.Project.Slug == slug {
			return Entry{}, fmt.Errorf("the slug %q already names project %s; choose another or join that project", slug, entry.Project.ID)
		}
	}
	root, err := repositoryTop(ctx, dir)
	if err != nil {
		return Entry{}, err
	}
	if existing, found, err := Load(root); err != nil {
		return Entry{}, err
	} else if found {
		return Entry{}, fmt.Errorf("%s already belongs to project %s (%s) through its Project.toml; create never replaces it", root, existing.Slug, existing.ID)
	}
	id, err := NewID()
	if err != nil {
		return Entry{}, err
	}
	created, err := Write(root, Project{
		ID: id, Slug: slug, Name: name, Description: description,
		Component:   Component{Slug: slug, Primary: true},
		Worktrees:   Worktrees{ShareProjectTree: true},
		Artifacts:   Artifacts{Storage: "tree"},
		Conventions: Conventions{Artifacts: true, AgentsMD: true},
	})
	if err != nil {
		return Entry{}, err
	}
	if err := r.Register(created, locationAt(root)); err != nil {
		return Entry{}, err
	}
	return r.entry(id)
}

// Join makes the repository containing dir a component of the registered project id. JoinFile writes a Project.toml
// that lists the project's other components; JoinMarker records only the git marker and needs a git repository.
// componentSlug names this repository for JoinFile; empty derives it from the directory name.
func (r *Registry) Join(ctx context.Context, dir, id string, mode JoinMode, componentSlug string) (Entry, error) {
	existing, err := r.entry(id)
	if err != nil {
		return Entry{}, err
	}
	root, err := repositoryTop(ctx, dir)
	if err != nil {
		return Entry{}, err
	}
	project := existing.Project
	switch mode {
	case JoinMarker:
		if top, err := gitRoot(ctx, root); err != nil {
			return Entry{}, err
		} else if top == "" {
			return Entry{}, errors.New("the marker lives in git config and this directory is not in a git repository; join by file instead")
		}
		if output, err := gitCommand(ctx, root, "config", "--local", markerKey, id).CombinedOutput(); err != nil {
			return Entry{}, fmt.Errorf("record the %s marker: %w: %s", markerKey, err, strings.TrimSpace(string(output)))
		}
	case JoinFile:
		if componentSlug == "" {
			componentSlug = SlugFromName(filepath.Base(root))
		}
		if project, err = joinedDocument(existing.Project, componentSlug); err != nil {
			return Entry{}, err
		}
		if _, err := Write(root, project); err != nil {
			return Entry{}, err
		}
	default:
		return Entry{}, fmt.Errorf("unknown join mode %q", mode)
	}
	if err := r.Register(project, locationAt(root)); err != nil {
		return Entry{}, err
	}
	return r.entry(id)
}

func (r *Registry) entry(id string) (Entry, error) {
	entry, found, err := r.Lookup(id)
	if err == nil && !found {
		err = fmt.Errorf("no registered project %q", id)
	}
	return entry, err
}

// joinedDocument is the Project.toml of a new component repository of an existing project: it keeps the project
// identity, adopts a component of the same slug or adds a non-primary one, and lists every other component.
func joinedDocument(existing Project, componentSlug string) (Project, error) {
	if !slugPattern.MatchString(componentSlug) {
		return Project{}, &Error{Path: "project", Key: "component.slug", Reason: "expected a lowercase kebab slug; pass one explicitly"}
	}
	joined := existing
	joined.Components = nil
	joined.Component = Component{Slug: componentSlug}
	for _, component := range append([]Component{existing.Component}, existing.Components...) {
		if component.Slug == componentSlug {
			joined.Component = component
		} else {
			joined.Components = append(joined.Components, component)
		}
	}
	return joined, nil
}

// readMarker returns the project ID recorded in the repository's local git config, empty when absent.
func readMarker(ctx context.Context, repository string) (string, error) {
	output, err := gitOutput(ctx, repository, "config", "--local", "--get", markerKey)
	return strings.TrimSpace(output), err
}

// Observe records or refreshes the registry copy of the Project.toml found for dir and registers its checkout
// (SPEC-0012 §3.4). It writes nothing when the registry already matches, and refuses a slug another project owns.
func (r *Registry) Observe(ctx context.Context, dir string) error {
	root, err := repositoryTop(ctx, dir)
	if err != nil {
		return err
	}
	repository, err := gitRoot(ctx, root)
	if err != nil {
		return err
	}
	found, projectRoot, ok, err := findProjectFile(ctx, root, repository)
	if err != nil || !ok {
		return err
	}
	entries, _, err := r.scanContext(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Project.ID != found.ID {
			if entry.Project.Slug == found.Slug {
				return fmt.Errorf("the slug %q of %s is already used by project %s", found.Slug, projectRoot, entry.Project.ID)
			}
			continue
		}
		if render(entry.Project) == render(found) {
			for _, location := range entry.Locations {
				if location.Host == "local" && location.Path == projectRoot {
					return nil
				}
			}
		}
	}
	return r.Register(found, locationAt(projectRoot))
}

// RepositoryRoot is the git top-level directory containing dir, or "" outside a repository.
func RepositoryRoot(ctx context.Context, dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	if absolute, err = filepath.EvalSymlinks(absolute); err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	return gitRoot(ctx, absolute)
}
