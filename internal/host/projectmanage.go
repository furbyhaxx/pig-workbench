package host

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/project"
)

// createProject creates a project at the session's current location and reports what was written.
func (s *session) createProject(ctx context.Context, slug, name, description string) (string, error) {
	cwd := s.location.Current().Cwd
	entry, err := s.projects.CreateProject(ctx, cwd, slug, name, description)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Created project %s (%s) and wrote %s/.project/Project.toml. Commit that file to share the identity; session storage adopts the project at the next session start.", entry.Project.Slug, entry.Project.ID, entry.Locations[len(entry.Locations)-1].Path), nil
}

// joinProject joins the session's current location to a registered project named by ID or slug.
func (s *session) joinProject(ctx context.Context, ref string, mode project.JoinMode, component string) (string, error) {
	entry, err := s.resolveProject(ref)
	if err != nil {
		return "", err
	}
	joined, err := s.projects.Join(ctx, s.location.Current().Cwd, entry.Project.ID, mode, component)
	if err != nil {
		return "", err
	}
	how := "wrote .project/Project.toml"
	if mode == project.JoinMarker {
		how = "recorded the local git marker pig.projectId (nothing committed, no file written)"
	}
	return fmt.Sprintf("Joined project %s (%s): %s.", joined.Project.Slug, joined.Project.ID, how), nil
}

func (s *session) resolveProject(ref string) (project.Entry, error) {
	entries, err := s.projects.List()
	if err != nil {
		return project.Entry{}, err
	}
	for _, entry := range entries {
		if entry.Project.ID == ref || entry.Project.Slug == ref {
			return entry, nil
		}
	}
	return project.Entry{}, fmt.Errorf("no registered project %q; see /workbench projects list", ref)
}

func (s *session) listProjects() (string, error) {
	entries, err := s.projects.List()
	if err != nil && len(entries) == 0 {
		return "", err
	}
	var out strings.Builder
	if len(entries) == 0 {
		out.WriteString("no registered projects; create one with /workbench projects create <slug> [name]\n")
	}
	for _, entry := range entries {
		fmt.Fprintf(&out, "%s  %s", entry.Project.Slug, entry.Project.ID)
		if entry.Project.Name != "" {
			fmt.Fprintf(&out, "  %s", entry.Project.Name)
		}
		fmt.Fprintf(&out, "  (%d location(s))\n", len(entry.Locations))
	}
	if err != nil {
		fmt.Fprintf(&out, "warning: %v\n", err)
	}
	return out.String(), nil
}

// projectsManage handles list, create, join and example; handled is false for other verbs.
func (s *session) projectsManage(ctx context.Context, verb, rest string) (text string, handled bool, err error) {
	fields := strings.Fields(rest)
	switch verb {
	case "list":
		text, err = s.listProjects()
	case "example":
		text = project.ExampleDocument
	case "create":
		if len(fields) == 0 {
			return "", true, errors.New("usage: /workbench projects create <slug> [name]")
		}
		name := strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), fields[0])), `"`)
		text, err = s.createProject(ctx, fields[0], name, "")
	case "join":
		if len(fields) == 0 || len(fields) > 3 {
			return "", true, errors.New("usage: /workbench projects join <id|slug> [file|marker] [component-slug]")
		}
		mode, component := project.JoinFile, ""
		if len(fields) > 1 {
			mode = project.JoinMode(fields[1])
			if mode != project.JoinFile && mode != project.JoinMarker {
				return "", true, fmt.Errorf("join mode must be file or marker, not %q", fields[1])
			}
		}
		if len(fields) > 2 {
			component = fields[2]
		}
		text, err = s.joinProject(ctx, fields[0], mode, component)
	default:
		return "", false, nil
	}
	return text, true, err
}
