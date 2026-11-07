package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func openTestRegistry(t *testing.T, agentDir string) *Registry {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("private registry permissions require Unix ownership checks")
	}
	r, err := Open(agentDir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func testRegistryProject(t *testing.T, id string) Project {
	t.Helper()
	p, err := parseDocument("fixture", validDocument)
	if err != nil {
		t.Fatal(err)
	}
	p.ID = id
	return p
}

func TestPrivateNotesRootRejectsUnsafeAndOtherProjectDirectories(t *testing.T) {
	agent := t.TempDir()
	registry := openTestRegistry(t, agent)
	if root, err := registry.PrivateNotesRoot("prj-first"); err != nil || root != "" {
		t.Fatalf("missing private root: %q, %v", root, err)
	}
	private := filepath.Join(agent, "projects", "prj-first", "private")
	if err := os.MkdirAll(private, 0700); err != nil {
		t.Fatal(err)
	}
	if root, err := registry.PrivateNotesRoot("prj-first"); err != nil || root != private {
		t.Fatalf("private root: %q, %v", root, err)
	}
	if _, err := registry.PrivateNotesRoot("../escape"); err == nil {
		t.Fatal("invalid project ID accepted")
	}
	other := filepath.Join(agent, "projects", "prj-other")
	if err := os.Symlink(filepath.Dir(private), other); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := registry.PrivateNotesRoot("prj-other"); err == nil {
		t.Fatal("symlinked project registry accepted")
	}
	if err := os.Chmod(private, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.PrivateNotesRoot("prj-first"); err == nil {
		t.Fatal("non-private notes directory accepted")
	}
}

func TestRegistryConcurrentRegistrationAndLocationMerge(t *testing.T) {
	dir := t.TempDir()
	registries := []*Registry{openTestRegistry(t, dir), openTestRegistry(t, dir)}
	projects := []Project{testRegistryProject(t, "prj-first"), testRegistryProject(t, "prj-second")}
	seen := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	locations := []Location{
		{Host: "local", Path: "/work/first", Kind: "main", LastSeen: seen},
		{Host: "local", Path: "/work/second", Kind: "clone", LastSeen: seen},
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range registries {
		wg.Go(func() {
			<-start
			results <- registries[i].Register(projects[i], locations[i])
		})
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	third := openTestRegistry(t, dir)
	entries, err := third.List()
	if err != nil || len(entries) != 2 {
		t.Fatalf("list after concurrent register: entries=%+v err=%v", entries, err)
	}
	locations[0].LastSeen = seen.Add(time.Hour)
	locations[0].Kind = "worktree"
	projects[0].Name = "Renamed project"
	projects[0].Components = []Component{{Slug: "web", DefaultRemote: "example.org/web", DefaultBranch: "main"}}
	if err := third.Register(projects[0], locations[0]); err != nil {
		t.Fatal(err)
	}
	entry, found, err := registries[0].Lookup(projects[0].ID)
	if err != nil || !found || len(entry.Locations) != 1 || entry.Locations[0].Kind != "worktree" || !entry.Locations[0].LastSeen.Equal(locations[0].LastSeen) {
		t.Fatalf("location not updated in place: entry=%+v found=%v err=%v", entry, found, err)
	}
	if !reflect.DeepEqual(entry.Project, projects[0]) {
		t.Fatalf("project fields did not survive registration: got=%+v want=%+v", entry.Project, projects[0])
	}
	if entry.RegistryDir != filepath.Join(dir, "projects", projects[0].ID) {
		t.Fatalf("registry directory = %q", entry.RegistryDir)
	}
	if _, found, err := third.Lookup("prj-missing"); err != nil || found {
		t.Fatalf("missing lookup: found=%v err=%v", found, err)
	}
}

func TestRegistryConcurrentLocationsSameProject(t *testing.T) {
	dir := t.TempDir()
	a, b := openTestRegistry(t, dir), openTestRegistry(t, dir)
	p := testRegistryProject(t, "prj-same")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, r := range []*Registry{a, b} {
		wg.Go(func() {
			results <- r.Register(p, Location{Host: "local", Path: fmt.Sprintf("/checkout/%d", i), Kind: "clone"})
		})
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	entry, found, err := a.Lookup(p.ID)
	if err != nil || !found || len(entry.Locations) != 2 {
		t.Fatalf("locations lost: entry=%+v found=%v err=%v", entry, found, err)
	}
	for _, location := range entry.Locations {
		if location.LastSeen.IsZero() {
			t.Fatal("registration did not set last-seen time")
		}
	}
}

func TestRegistryRecoversIndexAndReportsInvalidDocument(t *testing.T) {
	dir := t.TempDir()
	r := openTestRegistry(t, dir)
	p := testRegistryProject(t, "prj-good")
	if err := r.Register(p, Location{Host: "local", Path: "/good", Kind: "main"}); err != nil {
		t.Fatal(err)
	}
	badDir := filepath.Join(dir, "projects", "prj-bad")
	if err := os.Mkdir(badDir, 0700); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(badDir, "project.toml")
	if err := os.WriteFile(badPath, []byte("name = 'private-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(dir, "projects", "index.toml")
	if err := os.WriteFile(indexPath, []byte("not valid TOML [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := r.List()
	var problem *Error
	if len(entries) != 1 || entries[0].Project.ID != p.ID || !errors.As(err, &problem) || problem.Path != badPath {
		t.Fatalf("list did not return valid entry and problem: entries=%+v err=%v", entries, err)
	}
	if strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("diagnostic leaked source contents: %v", err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Projects []struct {
			ID string `toml:"id"`
		} `toml:"projects"`
	}
	if _, err := toml.Decode(string(data), &index); err != nil || len(index.Projects) != 1 || index.Projects[0].ID != p.ID {
		t.Fatalf("corrupt index was not rebuilt: %s, err=%v", data, err)
	}
	if err := os.WriteFile(indexPath, []byte("corrupt again"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Rebuild(); !errors.As(err, &problem) {
		t.Fatalf("rebuild did not report invalid document: %v", err)
	}
	data, err = os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	index.Projects = nil
	if _, err := toml.Decode(string(data), &index); err != nil || len(index.Projects) != 1 || index.Projects[0].ID != p.ID {
		t.Fatalf("explicit rebuild did not recover the index: %s, err=%v", data, err)
	}
	if entry, found, err := r.Lookup(p.ID); err != nil || !found || entry.Project.ID != p.ID {
		t.Fatalf("bad neighboring record blocked lookup: entry=%+v found=%v err=%v", entry, found, err)
	}
}

func TestRegistryModesAndAtomicCleanup(t *testing.T) {
	dir := t.TempDir()
	r := openTestRegistry(t, dir)
	p := testRegistryProject(t, "prj-mode")
	if err := r.Register(p, Location{Path: "/path", Kind: "main"}); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "projects")
	projectDir := filepath.Join(root, p.ID)
	for _, tc := range []struct {
		path string
		mode os.FileMode
	}{
		{root, 0700}, {projectDir, 0700},
		{filepath.Join(root, ".registry.lock"), 0600},
		{filepath.Join(root, "index.toml"), 0600},
		{filepath.Join(projectDir, "project.toml"), 0600},
	} {
		info, err := os.Stat(tc.path)
		if err != nil || info.Mode().Perm() != tc.mode {
			t.Fatalf("mode of %s: info=%v err=%v, want %04o", tc.path, info, err, tc.mode)
		}
	}
	blocked := filepath.Join(projectDir, "blocked")
	if err := os.Mkdir(blocked, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeRegistryFile(blocked, []byte("replacement")); err == nil {
		t.Fatal("replacing a directory should fail")
	}
	for _, path := range []string{root, projectDir} {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".index.toml.") || strings.HasPrefix(entry.Name(), ".project.toml.") || strings.HasPrefix(entry.Name(), ".blocked.") {
				t.Fatalf("temporary file leaked in %s: %s", path, entry.Name())
			}
		}
	}
}

func TestRegistryRejectsInvalidInputBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	r := openTestRegistry(t, dir)
	p := testRegistryProject(t, "prj-ok")
	if err := r.Register(p, Location{Path: "/path", Kind: "unknown"}); err == nil {
		t.Fatal("invalid location kind accepted")
	}
	p.ID = "../escape"
	if err := r.Register(p, Location{Path: "/path", Kind: "main"}); err == nil {
		t.Fatal("invalid project id accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid id changed filesystem: %v", err)
	}
}

func TestRegistryWriterProcess(t *testing.T) {
	id := os.Getenv("PROJECT_REGISTRY_WRITER")
	if id == "" {
		return
	}
	r := openTestRegistry(t, os.Getenv("PROJECT_REGISTRY_ROOT"))
	if err := r.Register(testRegistryProject(t, id), Location{Host: "local", Path: "/" + id, Kind: "clone"}); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryCrossProcessRegistration(t *testing.T) {
	dir := t.TempDir()
	openTestRegistry(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"prj-processone", "prj-processtwo"} {
		wg.Go(func() {
			<-start
			command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRegistryWriterProcess$")
			command.Env = append(os.Environ(), "PROJECT_REGISTRY_WRITER="+id, "PROJECT_REGISTRY_ROOT="+dir)
			output, err := command.CombinedOutput()
			if err != nil {
				results <- fmt.Errorf("writer %s: %w: %s", id, err, output)
				return
			}
			results <- nil
		})
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := openTestRegistry(t, dir).List()
	if err != nil || len(entries) != 2 {
		t.Fatalf("cross-process registration lost entries: entries=%+v err=%v", entries, err)
	}
}
