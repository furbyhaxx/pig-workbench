package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlugFromName(t *testing.T) {
	for name, want := range map[string]string{
		"My Project!":        "my-project",
		"  PiG   extensions": "pig-extensions",
		"Zürich café":        "z-rich-caf",
		"---":                "",
		"v2.0":               "v2-0",
	} {
		if got := SlugFromName(name); got != want {
			t.Errorf("SlugFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestNewIDIsValidAndFresh(t *testing.T) {
	first, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := NewID()
	if !idPattern.MatchString(first) || len(first) != len("prj-")+10 || first == second {
		t.Fatalf("ids %q and %q are not distinct valid ids", first, second)
	}
}

func TestCreateProjectWritesDocumentRegistersAndDetects(t *testing.T) {
	requireTestGit(t)
	registry, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	newTestRepository(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry, err := registry.CreateProject(context.Background(), filepath.Join(repo, "pkg"), "demo", "Demo \"quoted\" project", "Line one")
	if err != nil {
		t.Fatal(err)
	}
	if !idPattern.MatchString(entry.Project.ID) || entry.Project.Name != "Demo \"quoted\" project" || len(entry.Locations) != 1 {
		t.Fatalf("unexpected entry %+v", entry)
	}
	root := testCanonical(t, repo)
	loaded, found, err := Load(root)
	if err != nil || !found || loaded.ID != entry.Project.ID || loaded.Description != "Line one" {
		t.Fatalf("Project.toml not written at the repository top: %+v %v %v", loaded, found, err)
	}
	detection, err := registry.Detect(context.Background(), filepath.Join(repo, "pkg"))
	if err != nil || detection.ProjectID != entry.Project.ID || detection.How != "project-file" {
		t.Fatalf("created project not recognised: %+v %v", detection, err)
	}
	if _, err := registry.CreateProject(context.Background(), repo, "demo", "", ""); err == nil || !strings.Contains(err.Error(), "already names project") {
		t.Fatalf("duplicate slug accepted: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(root, ".project", "Project.toml"))
	if _, err := registry.CreateProject(context.Background(), repo, "other", "", ""); err == nil {
		t.Fatal("an existing Project.toml was replaced")
	}
	after, _ := os.ReadFile(filepath.Join(root, ".project", "Project.toml"))
	if string(before) != string(after) {
		t.Fatal("existing Project.toml changed")
	}
	if _, err := registry.CreateProject(context.Background(), repo, "Not A Slug", "", ""); err == nil {
		t.Fatal("invalid slug accepted")
	}
}

func TestJoinByMarkerRecognisesRepositoryWithoutAnyFile(t *testing.T) {
	requireTestGit(t)
	registry, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(t.TempDir(), "owner")
	newTestRepository(t, owner)
	entry, err := registry.CreateProject(context.Background(), owner, "shared", "Shared", "")
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	newTestRepository(t, foreign)
	if detection, _ := registry.Detect(context.Background(), foreign); detection.ProjectID != "global" {
		t.Fatalf("unrelated repository not global: %+v", detection)
	}
	if _, _, err := registry.Join(context.Background(), foreign, entry.Project.ID, JoinMarker, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(foreign, ".project")); !os.IsNotExist(err) {
		t.Fatal("marker join wrote files into the repository")
	}
	detection, err := registry.Detect(context.Background(), foreign)
	if err != nil || detection.ProjectID != entry.Project.ID || detection.How != "marker" {
		t.Fatalf("marker not honoured: %+v %v", detection, err)
	}
	plain := t.TempDir()
	if _, _, err := registry.Join(context.Background(), plain, entry.Project.ID, JoinMarker, ""); err == nil {
		t.Fatal("marker join accepted outside git")
	}
	if _, _, err := registry.Join(context.Background(), foreign, "prj-unknown", JoinMarker, ""); err == nil {
		t.Fatal("joined an unregistered project")
	}
}

func TestJoinByFileListsTheOtherComponents(t *testing.T) {
	requireTestGit(t)
	registry, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "first")
	newTestRepository(t, first)
	entry, err := registry.CreateProject(context.Background(), first, "suite", "Suite", "")
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "Second Repo")
	newTestRepository(t, second)
	joined, _, err := registry.Join(context.Background(), second, entry.Project.ID, JoinFile, "")
	if err != nil {
		t.Fatal(err)
	}
	document, found, err := Load(testCanonical(t, second))
	if err != nil || !found {
		t.Fatalf("joined document missing: %v", err)
	}
	if document.ID != entry.Project.ID || document.Component.Slug != "second-repo" || document.Component.Primary ||
		len(document.Components) != 1 || document.Components[0].Slug != "suite" || !document.Components[0].Primary {
		t.Fatalf("component roles wrong: %+v", document)
	}
	if len(joined.Locations) != 2 {
		t.Fatalf("both checkouts must be registered, got %+v", joined.Locations)
	}
}

func TestObserveRegistersAProjectFileOnceAndRefreshesIt(t *testing.T) {
	requireTestGit(t)
	registry, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	newTestRepository(t, repo)
	if _, err := Create(repo, "prj-observed1", "observed"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Observe(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	entry, err := registry.entry("prj-observed1")
	if err != nil || len(entry.Locations) != 1 {
		t.Fatalf("not registered: %+v %v", entry, err)
	}
	seen := entry.Locations[0].LastSeen
	if err := registry.Observe(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if again, _ := registry.entry("prj-observed1"); !again.Locations[0].LastSeen.Equal(seen) {
		t.Fatal("an unchanged project was written again")
	}
	document := filepath.Join(repo, ".project", "Project.toml")
	text, _ := os.ReadFile(document)
	text = []byte(strings.Replace(string(text), "slug = \"observed\"\n", "slug = \"observed\"\nname = \"Late name\"\n", 1))
	if err := os.WriteFile(document, text, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := registry.Observe(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if refreshed, _ := registry.entry("prj-observed1"); refreshed.Project.Name != "Late name" {
		t.Fatalf("registry copy not refreshed: %+v", refreshed.Project)
	}
	other := filepath.Join(t.TempDir(), "other")
	newTestRepository(t, other)
	if _, err := Create(other, "prj-thief0001", "observed"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Observe(context.Background(), other); err == nil {
		t.Fatal("a slug owned by another project was registered")
	}
}

func TestJoinAProjectAlreadyDeclaredHereIsIdempotentAndAnotherOneIsRefusedPlainly(t *testing.T) {
	requireTestGit(t)
	registry, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	newTestRepository(t, repo)
	created, err := registry.CreateProject(context.Background(), repo, "home", "Home", "")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(repo, ".project", "Project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []JoinMode{JoinFile, JoinMarker} {
		entry, changed, err := registry.Join(context.Background(), repo, created.Project.ID, mode, "home")
		if err != nil || changed || entry.Project.ID != created.Project.ID {
			t.Fatalf("joining the project this checkout already declares (%s) = %+v, %v", mode, entry, err)
		}
	}
	after, _ := os.ReadFile(filepath.Join(repo, ".project", "Project.toml"))
	if string(before) != string(after) {
		t.Fatal("an idempotent join rewrote the Project.toml")
	}
	foreign := filepath.Join(t.TempDir(), "foreign")
	newTestRepository(t, foreign)
	second, err := registry.CreateProject(context.Background(), foreign, "second", "Second", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []JoinMode{JoinFile, JoinMarker} {
		_, _, err := registry.Join(context.Background(), repo, second.Project.ID, mode, "x")
		if err == nil || !strings.Contains(err.Error(), "already declares project home") || strings.Contains(err.Error(), "link") {
			t.Fatalf("joining a different project over an existing Project.toml (%s) = %v", mode, err)
		}
	}
}
