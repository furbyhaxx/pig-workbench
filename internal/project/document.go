// Package project manages shared project documents and their local registry.
package project

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/BurntSushi/toml"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

const maxDocumentBytes = 1 << 20

// ExampleDocument is a complete, commented Project.toml that every supported key appears in; docs/projects.md embeds it verbatim.
//
//go:embed example.toml
var ExampleDocument string

var (
	idPattern      = regexp.MustCompile(`^prj-[a-z0-9]+$`)
	slugPattern    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	decoderLastKey = regexp.MustCompile(`\(last key ("(?:\\.|[^"\\])*")\)`)
)

// Project is the shared identity and configuration of a project.
type Project struct {
	ID          string      `toml:"id"`
	Slug        string      `toml:"slug"`
	Name        string      `toml:"name"`
	Description string      `toml:"description"`
	Component   Component   `toml:"component"`
	Components  []Component `toml:"components"`
	Worktrees   Worktrees   `toml:"worktrees"`
	Artifacts   Artifacts   `toml:"artifacts"`
	Conventions Conventions `toml:"conventions"`
}

// Component describes one repository's role in a project.
type Component struct {
	Slug          string `toml:"slug"`
	DefaultRemote string `toml:"default_remote"`
	DefaultBranch string `toml:"default_branch"`
	Primary       bool   `toml:"primary"`
}

// Worktrees configures links and artifact-tree sharing for worktrees.
type Worktrees struct {
	Links            []string `toml:"links"`
	ShareProjectTree bool     `toml:"share_project_tree"`
}

// Artifacts configures project artifact storage.
type Artifacts struct {
	Storage string `toml:"storage"`
}

// Conventions enables project artifact and instruction conventions.
type Conventions struct {
	Artifacts bool `toml:"artifacts"`
	AgentsMD  bool `toml:"agents_md"`
}

// Error identifies a document problem without reporting source values.
type Error struct {
	Path   string
	Key    string
	Line   int
	Column int
	Reason string
}

// Error formats a source-safe project document diagnostic.
func (e *Error) Error() string {
	where := e.Path
	if e.Line > 0 {
		where += fmt.Sprintf(":%d:%d", e.Line, e.Column)
	}
	if e.Key != "" {
		where += " (" + e.Key + ")"
	}
	return where + ": " + e.Reason
}

type document struct {
	Project
	Version int `toml:"version"`
}

var documentKeys = map[string]bool{
	"version": true, "id": true, "slug": true, "name": true, "description": true,
	"component": true, "component.slug": true, "component.primary": true,
	"component.default_remote": true, "component.default_branch": true,
	"components": true, "components.slug": true, "components.primary": true,
	"components.default_remote": true, "components.default_branch": true,
	"worktrees": true, "worktrees.links": true, "worktrees.share_project_tree": true,
	"artifacts": true, "artifacts.storage": true,
	"conventions": true, "conventions.artifacts": true, "conventions.agents_md": true,
}

// Parse strictly decodes and validates a project document of at most 1 MiB.
func Parse(path string) (Project, error) {
	file, _, err := fileio.OpenRegular(path)
	if err != nil {
		return Project{}, fmt.Errorf("read project document %s: %w", path, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxDocumentBytes+1))
	if err != nil {
		return Project{}, fmt.Errorf("read project document %s: %w", path, err)
	}
	return parseDocument(path, string(data))
}

func defaultDocument() document {
	return document{
		Version: 1,
		Project: Project{
			Worktrees: Worktrees{ShareProjectTree: true},
			Artifacts: Artifacts{Storage: "tree"},
		},
	}
}

func parseDocument(path, data string) (Project, error) {
	doc := defaultDocument()
	meta, err := decodeDocument(path, data, &doc, nil)
	if err != nil {
		return Project{}, err
	}
	if err := validateDocument(path, doc, meta); err != nil {
		return Project{}, err
	}
	return doc.Project, nil
}

func decodeDocument(path, data string, target any, extraKeys map[string]bool) (toml.MetaData, error) {
	if len(data) > maxDocumentBytes {
		return toml.MetaData{}, &Error{Path: path, Reason: "project document exceeds 1 MiB"}
	}
	meta, err := toml.Decode(data, target)
	if err != nil {
		problem := &Error{Path: path, Reason: "invalid TOML syntax or value type"}
		var parse toml.ParseError
		if errors.As(err, &parse) {
			problem.Key = parse.LastKey
			problem.Line = parse.Position.Line
			problem.Column = parse.Position.Col
		} else if match := decoderLastKey.FindStringSubmatch(err.Error()); len(match) == 2 {
			// Type mismatches expose their key only in the decoder's error text.
			if key, unquoteErr := strconv.Unquote(match[1]); unquoteErr == nil {
				for _, candidate := range meta.Keys() {
					if candidate.String() == key {
						problem.Key = key
						break
					}
				}
			}
		}
		return meta, problem
	}
	for _, key := range meta.Keys() {
		if !documentKeys[key.String()] && !extraKeys[key.String()] {
			return meta, &Error{Path: path, Key: key.String(), Reason: "unknown key"}
		}
	}
	if keys := meta.Undecoded(); len(keys) > 0 {
		return meta, &Error{Path: path, Key: keys[0].String(), Reason: "unknown key"}
	}
	return meta, nil
}

func validateDocument(path string, doc document, meta toml.MetaData) error {
	if doc.Version != 1 {
		return &Error{Path: path, Key: "version", Reason: "expected supported integer version 1"}
	}
	if !meta.IsDefined("component") {
		return &Error{Path: path, Key: "component", Reason: "component is required"}
	}
	return validate(path, doc.Project)
}

func validate(path string, project Project) error {
	problem := func(key, reason string) error {
		return &Error{Path: path, Key: key, Reason: reason}
	}
	if !idPattern.MatchString(project.ID) {
		return problem("id", "expected prj- followed by lowercase letters or digits")
	}
	if !slugPattern.MatchString(project.Slug) {
		return problem("slug", "expected a lowercase kebab slug")
	}
	if !slugPattern.MatchString(project.Component.Slug) {
		return problem("component.slug", "expected a lowercase kebab slug")
	}
	if project.Artifacts.Storage != "tree" && project.Artifacts.Storage != "branch" {
		return problem("artifacts.storage", "expected tree or branch")
	}
	seen := map[string]bool{project.Component.Slug: true}
	primary := 0
	if project.Component.Primary {
		primary++
	}
	for _, component := range project.Components {
		if !slugPattern.MatchString(component.Slug) {
			return problem("components.slug", "expected a lowercase kebab slug")
		}
		if seen[component.Slug] {
			return problem("components.slug", "component slugs must be unique")
		}
		seen[component.Slug] = true
		if component.Primary {
			primary++
			if primary > 1 {
				return problem("components.primary", "exactly one component must be primary")
			}
		}
	}
	if primary != 1 {
		return problem("component.primary", "exactly one component must be primary")
	}
	return nil
}

// Load reads the project document directly under dir; found reports its presence.
func Load(dir string) (project Project, found bool, err error) {
	path := filepath.Join(dir, ".project", "Project.toml")
	project, err = Parse(path)
	if errors.Is(err, os.ErrNotExist) {
		return Project{}, false, nil
	}
	return project, true, err
}

// render writes a validated project as a commented Project.toml.
func render(project Project) string {
	var b strings.Builder
	b.WriteString("# Project identity shared by all component repositories.\nversion = 1\n")
	fmt.Fprintf(&b, "id = %q\nslug = %q\n", project.ID, project.Slug)
	if project.Name != "" {
		fmt.Fprintf(&b, "name = %q\n", project.Name)
	}
	if project.Description != "" {
		fmt.Fprintf(&b, "description = %q\n", project.Description)
	}
	b.WriteString("\n# This repository's role in the project.\n[component]\n")
	writeComponent(&b, project.Component)
	for _, component := range project.Components {
		b.WriteString("\n# Another repository of the project.\n[[components]]\n")
		writeComponent(&b, component)
	}
	links := make([]string, len(project.Worktrees.Links))
	for i, link := range project.Worktrees.Links {
		links[i] = fmt.Sprintf("%q", link)
	}
	fmt.Fprintf(&b, "\n[worktrees]\nlinks = [%s]\nshare_project_tree = %t\n", strings.Join(links, ", "), project.Worktrees.ShareProjectTree)
	fmt.Fprintf(&b, "\n[artifacts]\nstorage = %q # tree or branch\n", project.Artifacts.Storage)
	fmt.Fprintf(&b, "\n[conventions]\nartifacts = %t\nagents_md = %t\n", project.Conventions.Artifacts, project.Conventions.AgentsMD)
	return b.String()
}

func writeComponent(b *strings.Builder, component Component) {
	fmt.Fprintf(b, "slug = %q\n", component.Slug)
	if component.DefaultRemote != "" {
		fmt.Fprintf(b, "default_remote = %q\n", component.DefaultRemote)
	}
	if component.DefaultBranch != "" {
		fmt.Fprintf(b, "default_branch = %q\n", component.DefaultBranch)
	}
	if component.Primary {
		b.WriteString("primary = true\n")
	}
}

// Create writes a new single-repository project document without replacing an existing path.
func Create(dir, id, slug string) (Project, error) {
	return Write(dir, Project{
		ID: id, Slug: slug, Component: Component{Slug: slug, Primary: true},
		Worktrees: Worktrees{ShareProjectTree: true}, Artifacts: Artifacts{Storage: "tree"},
		Conventions: Conventions{Artifacts: true, AgentsMD: true},
	})
}

// Write validates project and publishes it as dir/.project/Project.toml without replacing an existing path.
func Write(dir string, project Project) (Project, error) {
	path := filepath.Join(dir, ".project", "Project.toml")
	if err := validate(path, project); err != nil {
		return Project{}, err
	}
	for _, text := range []string{project.Name, project.Description} {
		if !utf8.ValidString(text) || strings.ContainsFunc(text, unicode.IsControl) {
			return Project{}, &Error{Path: path, Key: "name", Reason: "text must be valid UTF-8 without control characters"}
		}
	}
	contents := render(project)
	parsed, err := parseDocument(path, contents)
	if err != nil {
		return Project{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return Project{}, fmt.Errorf("create project directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".Project.toml.*")
	if err != nil {
		return Project{}, fmt.Errorf("create project temporary file: %w", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.WriteString(contents); err != nil {
		return Project{}, fmt.Errorf("write project document: %w", err)
	}
	if err := file.Sync(); err != nil {
		return Project{}, fmt.Errorf("flush project document: %w", err)
	}
	if err := file.Close(); err != nil {
		return Project{}, fmt.Errorf("close project document: %w", err)
	}
	// Linking publishes atomically without replacing a racing creator's file.
	if err := os.Link(file.Name(), path); err != nil {
		return Project{}, fmt.Errorf("create project document %s: %w", path, err)
	}
	return parsed, nil
}
