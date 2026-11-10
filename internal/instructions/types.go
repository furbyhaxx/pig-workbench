// Package instructions discovers local ambient files and constructs bounded views.
package instructions

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Class defines filenames, placement, content role and per-file view limits.
// Placement is ancestors, root+nearest, nearest or root; Select is first or all.
// Zero values inherit the matching built-in or general defaults.
type Class struct {
	ID        string
	Filenames []string
	Placement string
	Select    string
	Role      string
	MaxLines  int
	MaxBytes  int
	MaxItems  int
	// AllowPin permits explicit pinning of files of this class (SPEC-0007 §6).
	AllowPin bool
}

// Options defines the project boundary, authorized user directories and scan budget.
// A zero MaxTotalBytes permits up to 4 MiB of source bytes per discovery.
type Options struct {
	ProjectRoot string
	// OuterRoot, when it lies above ProjectRoot, extends the ancestor chain up to it; files found
	// there carry user trust. Nested-repository checks still apply only below ProjectRoot.
	OuterRoot string
	UserRoots []string
	// PrivateRoot is a separately authorized project-private notes directory.
	PrivateRoot   string
	MaxTotalBytes int
	AllowNested   bool
	// ValidatePointers reports that target validation is deferred; it does not read targets.
	ValidatePointers bool
}

// NestedRepositoryError reports a nested repository encountered before the supplied boundary.
type NestedRepositoryError struct {
	Dir         string
	ProjectRoot string
}

// Error describes the boundary that needs an explicit caller decision.
func (e *NestedRepositoryError) Error() string {
	return fmt.Sprintf("nested repository %q lies below project boundary %q", e.Dir, e.ProjectRoot)
}

// Selected is a canonical file and its delivered view.
// Digest is SHA-256 over exactly the source bytes read, which may be a bounded prefix.
// OmittedItems is -1 for an item-capped class if the source scan was incomplete.
type Selected struct {
	Class        string
	Path         string
	DisplayPath  string
	Role         string
	Trust        string
	Content      string
	Digest       string
	Truncated    bool
	OmittedItems int
	Continuation string
}

// Reason describes selection, exclusion, cap, malformed-pointer or deferred-validation decisions.
type Reason struct {
	Class       string
	Path        string
	DisplayPath string
	Status      string
	Detail      string
}

// Set contains selected views and their discovery reasons.
type Set struct {
	Files   []Selected
	Reasons []Reason
}

// DefaultClasses returns independent default steering and backlog policies.
func DefaultClasses() []Class {
	return []Class{
		{ID: "steering", Filenames: []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD"}, Placement: "ancestors", Select: "first", Role: "instruction", MaxLines: 200, MaxBytes: 16384},
		{ID: "backlog", Filenames: []string{"BACKLOG.md"}, Placement: "ancestors", Select: "first", Role: "context", MaxLines: 200, MaxBytes: 16384, MaxItems: 25, AllowPin: true},
	}
}

// ProjectPrivateClass returns the built-in user-owned private-notes policy.
func ProjectPrivateClass() Class {
	return Class{ID: "project-private", Filenames: []string{"*.md"}, Placement: "root", Select: "all", Role: "instruction", MaxLines: 200, MaxBytes: 16384, AllowPin: true}
}

func validatedClasses(classes []Class) ([]Class, error) {
	if classes == nil {
		return DefaultClasses(), nil
	}
	defaults := DefaultClasses()
	result := make([]Class, 0, len(classes))
	ids := make(map[string]bool)
	for _, class := range classes {
		if class.ID == "" || ids[class.ID] {
			return nil, fmt.Errorf("class ID is empty or duplicated: %q", class.ID)
		}
		ids[class.ID] = true
		if class.MaxLines < 0 || class.MaxBytes < 0 || class.MaxItems < 0 {
			return nil, fmt.Errorf("class %q has a negative limit", class.ID)
		}
		base := Class{Placement: "ancestors", Select: "first", Role: "context", MaxLines: 200, MaxBytes: 16384}
		for _, builtin := range defaults {
			if builtin.ID == class.ID {
				base = builtin
				break
			}
		}
		if len(class.Filenames) == 0 {
			class.Filenames = append([]string(nil), base.Filenames...)
		} else {
			class.Filenames = append([]string(nil), class.Filenames...)
		}
		if class.Placement == "" {
			class.Placement = base.Placement
		}
		if class.Select == "" {
			class.Select = base.Select
		}
		if class.Role == "" {
			class.Role = base.Role
		}
		if class.MaxLines == 0 {
			class.MaxLines = base.MaxLines
		}
		if class.MaxBytes == 0 {
			class.MaxBytes = base.MaxBytes
		}
		if class.MaxItems == 0 {
			class.MaxItems = base.MaxItems
		}
		if len(class.Filenames) == 0 {
			return nil, fmt.Errorf("class %q has no filenames", class.ID)
		}
		names := make(map[string]bool)
		for _, name := range class.Filenames {
			if name == "" || name == "." || name == ".." || filepath.Base(name) != name || !filepath.IsLocal(name) || strings.ContainsAny(name, "/\\\x00") || names[name] {
				return nil, fmt.Errorf("class %q has an invalid or repeated filename %q", class.ID, name)
			}
			names[name] = true
		}
		switch class.Placement {
		case "ancestors", "root+nearest", "nearest", "root":
		default:
			return nil, fmt.Errorf("class %q has invalid placement %q", class.ID, class.Placement)
		}
		if class.Select != "first" && class.Select != "all" {
			return nil, fmt.Errorf("class %q has invalid selection %q", class.ID, class.Select)
		}
		if class.Role != "instruction" && class.Role != "context" {
			return nil, fmt.Errorf("class %q has invalid role %q", class.ID, class.Role)
		}
		result = append(result, class)
	}
	return result, nil
}
