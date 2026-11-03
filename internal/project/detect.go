package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Detection identifies a project; Root is its recognized root, or the queried directory outside Git for global.
type Detection struct {
	ProjectID   string
	Slug        string
	Name        string
	How         string
	RegistryDir string
	Root        string
}

// Detect recognizes a directory without modifying the project, Git, or registry.
func (r *Registry) Detect(ctx context.Context, dir string) (Detection, error) {
	if err := ctx.Err(); err != nil {
		return Detection{}, err
	}
	if dir == "" {
		return Detection{}, errors.New("detection directory must not be empty")
	}
	if err := r.checkDirectory(); err != nil {
		return Detection{}, err
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return Detection{}, fmt.Errorf("resolve detection directory: %w", err)
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return Detection{}, fmt.Errorf("resolve detection directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Detection{}, fmt.Errorf("inspect detection directory: %w", err)
	}
	if !info.IsDir() {
		return Detection{}, errors.New("detection path must be a directory")
	}
	repoRoot, err := gitRoot(ctx, absolute)
	if err != nil {
		return Detection{}, err
	}
	project, projectRoot, found, err := findProjectFile(ctx, absolute, repoRoot)
	if err != nil {
		return Detection{}, err
	}
	if found {
		return detected(ctx, project, "project-file", filepath.Join(r.dir, project.ID), projectRoot)
	}

	entries, problems, err := r.scanContext(ctx)
	if err != nil {
		return Detection{}, err
	}
	if err := ctx.Err(); err != nil {
		return Detection{}, err
	}
	if err := errors.Join(problems...); err != nil {
		return Detection{}, err
	}
	if err := r.checkDirectory(); err != nil {
		return Detection{}, err
	}
	host, err := os.Hostname()
	if err != nil {
		return Detection{}, fmt.Errorf("identify local host: %w", err)
	}
	probes := make(repositoryProbes)
	if repoRoot != "" && len(entries) > 0 {
		roots, err := probes.commits(ctx, repoRoot)
		if err != nil {
			return Detection{}, err
		}
		if len(roots) > 0 {
			matches, err := rootMatches(ctx, entries, host, roots, probes)
			if err != nil {
				return Detection{}, err
			}
			if len(matches) > 1 {
				remotes, err := probes.remotes(ctx, repoRoot)
				if err != nil {
					return Detection{}, err
				}
				narrowed, err := remoteMatches(ctx, matches, host, remotes, probes)
				if err != nil {
					return Detection{}, err
				}
				if len(narrowed) > 0 {
					matches = narrowed
				}
			}
			if len(matches) > 1 {
				return Detection{}, ambiguousMatches(matches)
			}
			if len(matches) == 1 {
				return detected(ctx, matches[0].Project, "root-commit", matches[0].RegistryDir, repoRoot)
			}
		}
		remotes, err := probes.remotes(ctx, repoRoot)
		if err != nil {
			return Detection{}, err
		}
		matches, err := remoteMatches(ctx, entries, host, remotes, probes)
		if err != nil {
			return Detection{}, err
		}
		if len(matches) > 1 {
			return Detection{}, ambiguousMatches(matches)
		}
		if len(matches) == 1 {
			return detected(ctx, matches[0].Project, "remote", matches[0].RegistryDir, repoRoot)
		}
	}
	matches, root, err := locationMatches(ctx, entries, host, absolute)
	if err != nil {
		return Detection{}, err
	}
	if len(matches) > 1 {
		return Detection{}, ambiguousMatches(matches)
	}
	if len(matches) == 1 {
		return detected(ctx, matches[0].Project, "path", matches[0].RegistryDir, root)
	}
	if repoRoot == "" {
		repoRoot = absolute
	}
	return detected(ctx, Project{ID: "global", Slug: "global", Name: "Global"}, "global", filepath.Join(r.dir, "global"), repoRoot)
}

func detected(ctx context.Context, project Project, how, registryDir, root string) (Detection, error) {
	if err := ctx.Err(); err != nil {
		return Detection{}, err
	}
	return Detection{ProjectID: project.ID, Slug: project.Slug, Name: project.Name, How: how, RegistryDir: registryDir, Root: root}, nil
}

func findProjectFile(ctx context.Context, dir, repoRoot string) (Project, string, bool, error) {
	if repoRoot != "" {
		dir = repoRoot
	}
	for {
		if err := ctx.Err(); err != nil {
			return Project{}, "", false, err
		}
		project, found, err := Load(dir)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Project{}, "", false, ctxErr
		}
		if err != nil {
			return Project{}, "", false, err
		}
		if found {
			return project, dir, true, nil
		}
		path := filepath.Join(dir, ".project", "Project.toml")
		if _, err := os.Lstat(path); err == nil {
			return Project{}, "", false, &Error{Path: path, Reason: "project document cannot be read"}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Project{}, "", false, fmt.Errorf("inspect project document %s: %w", path, err)
		}
		parent := filepath.Dir(dir)
		if repoRoot != "" || parent == dir {
			return Project{}, "", false, nil
		}
		dir = parent
	}
}

func localLocation(location Location, host string) bool {
	return location.Host == "" || strings.EqualFold(location.Host, "local") || strings.EqualFold(location.Host, "localhost") || strings.EqualFold(location.Host, host)
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func rootMatches(ctx context.Context, entries []Entry, host string, roots map[string]bool, probes repositoryProbes) ([]Entry, error) {
	var matches []Entry
	for _, entry := range entries {
		for _, location := range entry.Locations {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !localLocation(location, host) || !filepath.IsAbs(location.Path) {
				continue
			}
			known, err := probes.commits(ctx, location.Path)
			if err != nil {
				return nil, err
			}
			matched := false
			for commit := range known {
				if roots[commit] {
					matched = true
					break
				}
			}
			if matched {
				matches = append(matches, entry)
				break
			}
		}
	}
	return matches, nil
}

func remoteMatches(ctx context.Context, entries []Entry, host string, remotes map[string]bool, probes repositoryProbes) ([]Entry, error) {
	if len(remotes) == 0 {
		return nil, ctx.Err()
	}
	var matches []Entry
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matched := remotes[resolveRemote(entry.Project.Component.DefaultRemote)]
		for _, component := range entry.Project.Components {
			matched = matched || remotes[resolveRemote(component.DefaultRemote)]
		}
		for _, location := range entry.Locations {
			if matched {
				break
			}
			if !localLocation(location, host) || !filepath.IsAbs(location.Path) {
				continue
			}
			known, err := probes.remotes(ctx, location.Path)
			if err != nil {
				return nil, err
			}
			for remote := range known {
				if remotes[remote] {
					matched = true
					break
				}
			}
		}
		if matched {
			matches = append(matches, entry)
		}
	}
	return matches, nil
}

func locationMatches(ctx context.Context, entries []Entry, host, dir string) ([]Entry, string, error) {
	var matches []Entry
	var root string
	seen := make(map[string]bool)
	for _, entry := range entries {
		for _, location := range entry.Locations {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
			if !localLocation(location, host) || !filepath.IsAbs(location.Path) {
				continue
			}
			path, err := filepath.EvalSymlinks(location.Path)
			if err != nil || !pathWithin(path, dir) || len(path) < len(root) {
				continue
			}
			if len(path) > len(root) {
				matches, root = nil, path
				clear(seen)
			}
			if !seen[entry.Project.ID] {
				matches = append(matches, entry)
				seen[entry.Project.ID] = true
			}
		}
	}
	return matches, root, ctx.Err()
}

func ambiguousMatches(entries []Entry) error {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.Project.ID)
	}
	return fmt.Errorf("ambiguous project match: %s", strings.Join(ids, ", "))
}
