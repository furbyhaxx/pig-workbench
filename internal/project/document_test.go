package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const validDocument = `version = 1
id = "prj-abc123"
slug = "workbench"
name = "Workbench"
description = "private-document-value"

[component]
slug = "workbench"
primary = true
default_remote = "github.com/example/workbench"
default_branch = "main"

[worktrees]
links = []
share_project_tree = false

[artifacts]
storage = "tree"

[conventions]
artifacts = true
agents_md = true
`

func writeProjectDocument(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Project.toml")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseRejectsInvalidDocumentsWithSafeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, contents, key string
	}{
		{"two primaries", validDocument + "\n[[components]]\nslug = 'other'\nprimary = true\n", "components.primary"},
		{"bad id", strings.Replace(validDocument, `id = "prj-abc123"`, `id = "invalid-id"`, 1), "id"},
		{"bad slug", strings.Replace(validDocument, `slug = "workbench"`, `slug = "Not-Kebab"`, 1), "slug"},
		{"bad component slug", strings.Replace(validDocument, "[component]\nslug = \"workbench\"", "[component]\nslug = \"not_kebab\"", 1), "component.slug"},
		{"bad storage", strings.Replace(validDocument, `storage = "tree"`, `storage = "folder"`, 1), "artifacts.storage"},
		{"malformed value", strings.Replace(validDocument, `storage = "tree"`, `storage = "private-document-value`, 1), "artifacts.storage"},
		{"unknown top level", "mystery = 'private-document-value'\n" + validDocument, "mystery"},
		{"unknown component key", strings.Replace(validDocument, "[component]\n", "[component]\nextra = 'private-document-value'\n", 1), "component.extra"},
		{"unknown inline table key", strings.Replace(validDocument, "[component]\nslug = \"workbench\"\nprimary = true\ndefault_remote = \"github.com/example/workbench\"\ndefault_branch = \"main\"", "component = { slug = 'workbench', primary = true, extra = 'private-document-value' }", 1), "component.extra"},
		{"unknown component array key", validDocument + "\n[[components]]\nslug = 'web'\nextra = 'private-document-value'\n", "components.extra"},
		{"unknown inline component array key", strings.Replace(validDocument, "\n[component]\n", "\ncomponents = [{ slug = 'web', extra = 'private-document-value' }]\n\n[component]\n", 1), "components.extra"},
		{"unknown version", strings.Replace(validDocument, "version = 1", "version = 2", 1), "version"},
		{"wrong type", strings.Replace(validDocument, "version = 1", `version = "private-document-value"`, 1), "version"},
		{"wrong nested type", strings.Replace(validDocument, "links = []", `links = ["ok", 42]`, 1), "worktrees.links"},
		{"missing component", strings.Replace(validDocument, "[component]\nslug = \"workbench\"\nprimary = true\ndefault_remote = \"github.com/example/workbench\"\ndefault_branch = \"main\"\n", "", 1), "component"},
		{"duplicate component slug", validDocument + "\n[[components]]\nslug = 'workbench'\n", "components.slug"},
		{"missing primary", strings.Replace(validDocument, "primary = true", "primary = false", 1), "component.primary"},
		{"case sensitive key", strings.Replace(validDocument, "version = 1", "VERSION = 1", 1), "VERSION"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeProjectDocument(t, tc.contents)
			_, err := Parse(path)
			if err == nil {
				t.Fatal("invalid document accepted")
			}
			message := err.Error()
			if !strings.Contains(message, path) || !strings.Contains(message, tc.key) {
				t.Fatalf("diagnostic must identify file and key: %s", message)
			}
			if strings.Contains(message, "private-document-value") || strings.Contains(message, "invalid-id") || strings.Contains(message, "folder") {
				t.Fatalf("diagnostic leaked a source value: %s", message)
			}
		})
	}
}

func TestParsePreservesDocumentFields(t *testing.T) {
	contents := validDocument + "\n[[components]]\nslug = 'web'\ndefault_remote = 'github.com/example/web'\ndefault_branch = 'develop'\n"
	p, err := Parse(writeProjectDocument(t, contents))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "prj-abc123" || p.Slug != "workbench" || p.Name != "Workbench" || p.Description != "private-document-value" {
		t.Fatalf("project fields lost: %+v", p)
	}
	if p.Component != (Component{Slug: "workbench", Primary: true, DefaultRemote: "github.com/example/workbench", DefaultBranch: "main"}) {
		t.Fatalf("component fields lost: %+v", p.Component)
	}
	if len(p.Components) != 1 || p.Components[0] != (Component{Slug: "web", DefaultRemote: "github.com/example/web", DefaultBranch: "develop"}) {
		t.Fatalf("additional components lost: %+v", p.Components)
	}
	if p.Worktrees.ShareProjectTree || p.Worktrees.Links == nil || len(p.Worktrees.Links) != 0 {
		t.Fatalf("explicit false or empty links lost: %+v", p.Worktrees)
	}
	if p.Artifacts.Storage != "tree" || !p.Conventions.Artifacts || !p.Conventions.AgentsMD {
		t.Fatalf("artifacts or conventions lost: %+v", p)
	}
}

func TestParseRejectsOversizedDocument(t *testing.T) {
	path := writeProjectDocument(t, validDocument+"\n# "+strings.Repeat("x", 1<<20))
	_, err := Parse(path)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "1 MiB") {
		t.Fatalf("missing size diagnostic: %v", err)
	}
}

func TestParseBranchStorage(t *testing.T) {
	contents := strings.Replace(validDocument, `storage = "tree"`, `storage = "branch"`, 1)
	p, err := Parse(writeProjectDocument(t, contents))
	if err != nil {
		t.Fatal(err)
	}
	if p.Artifacts.Storage != "branch" {
		t.Fatalf("storage = %q, want branch", p.Artifacts.Storage)
	}
}

func TestComponentInAnotherRepositoryMayBePrimary(t *testing.T) {
	contents := strings.Replace(validDocument, "primary = true", "primary = false", 1) + "\n[[components]]\nslug = 'web'\nprimary = true\n"
	p, err := Parse(writeProjectDocument(t, contents))
	if err != nil {
		t.Fatal(err)
	}
	if p.Component.Primary || len(p.Components) != 1 || !p.Components[0].Primary {
		t.Fatalf("primary component changed: %+v", p)
	}
}

func TestLoadReportsMissingAndInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	if _, found, err := Load(dir); err != nil || found {
		t.Fatalf("missing file: found=%v err=%v", found, err)
	}
	path := filepath.Join(dir, ".project", "Project.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("id = 'bad'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := Load(dir); err == nil || !found {
		t.Fatalf("invalid file: found=%v err=%v", found, err)
	}
}

func TestCreateAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	created, err := Create(dir, "prj-123abc", "example-project")
	if err != nil {
		t.Fatal(err)
	}
	loaded, found, err := Load(dir)
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(created, loaded) || !loaded.Component.Primary || loaded.Component.Slug != "example-project" {
		t.Fatalf("round trip differs: created=%+v loaded=%+v", created, loaded)
	}
	path := filepath.Join(dir, ".project", "Project.toml")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "#") {
		t.Fatal("created project must use a commented template")
	}
	if _, err := Create(dir, "prj-456", "other"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing file was not refused: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(contents) {
		t.Fatal("existing file changed")
	}
}

func TestExplicitWorktreeValuesSurviveLoad(t *testing.T) {
	dir := t.TempDir()
	if _, err := Create(dir, "prj-123", "example"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".project", "Project.toml")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(original), "share_project_tree = true", "share_project_tree = false", 1)
	if edited == string(original) {
		t.Fatal("template is missing the worktree setting")
	}
	if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := Load(dir)
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if loaded.Worktrees.ShareProjectTree || loaded.Worktrees.Links == nil || len(loaded.Worktrees.Links) != 0 {
		t.Fatalf("explicit worktree values lost: %+v", loaded.Worktrees)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != edited {
		t.Fatal("loading modified the document")
	}
}

func TestCreateRejectsInvalidIdentityBeforeWriting(t *testing.T) {
	for _, tc := range []struct{ id, slug, key string }{
		{"prj-", "example", "id"},
		{"prj-abc", "../escape", "slug"},
	} {
		dir := t.TempDir()
		_, err := Create(dir, tc.id, tc.slug)
		if err == nil || !strings.Contains(err.Error(), tc.key) || !strings.Contains(err.Error(), filepath.Join(dir, ".project", "Project.toml")) {
			t.Fatalf("missing validation diagnostic: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, ".project")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("invalid identity created a directory: %v", err)
		}
	}
}

func TestParseDefaultsAndExplicitEmptyValues(t *testing.T) {
	minimal := "id = 'prj-123'\nslug = 'example'\n[component]\nslug = 'example'\nprimary = true\n"
	p, err := Parse(writeProjectDocument(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if p.Artifacts.Storage != "tree" || !p.Worktrees.ShareProjectTree {
		t.Fatalf("unexpected defaults: artifacts=%+v worktrees=%+v", p.Artifacts, p.Worktrees)
	}
	p, err = Parse(writeProjectDocument(t, minimal+"\n[worktrees]\nlinks = []\nshare_project_tree = false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Worktrees.ShareProjectTree || p.Worktrees.Links == nil || len(p.Worktrees.Links) != 0 {
		t.Fatalf("explicit worktree values lost: %+v", p.Worktrees)
	}
}

func TestConcurrentCreateRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		wg.Go(func() {
			_, err := Create(dir, fmt.Sprintf("prj-%d", i), "example")
			results <- err
		})
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d creators, want one", winners)
	}
	if _, found, err := Load(dir); err != nil || !found {
		t.Fatalf("resulting project invalid: found=%v err=%v", found, err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".project"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Project.toml" {
		t.Fatalf("unexpected temporary files: %v", entries)
	}
}

func TestExampleDocumentParsesAndNamesEverySupportedKey(t *testing.T) {
	project, err := parseDocument("example.toml", ExampleDocument)
	if err != nil {
		t.Fatalf("the published example must be a valid document: %v", err)
	}
	if project.Slug != "pig-extensions" || len(project.Components) != 1 || !project.Component.Primary {
		t.Fatalf("the example parsed to something else than it documents: %+v", project)
	}
	for key := range documentKeys {
		leaf := key
		if i := strings.LastIndex(key, "."); i >= 0 {
			leaf = key[i+1:]
		}
		if !strings.Contains(ExampleDocument, leaf) {
			t.Errorf("the example never mentions the supported key %q", key)
		}
	}
}

func TestProjectDocumentationEmbedsTheExampleVerbatim(t *testing.T) {
	docs, err := os.ReadFile(filepath.Join("..", "..", "docs", "projects.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(docs), ExampleDocument) {
		t.Fatal("docs/projects.md does not contain internal/project/example.toml verbatim; copy the file into its toml block")
	}
}
