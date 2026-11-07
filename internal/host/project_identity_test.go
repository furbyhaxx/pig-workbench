package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

func TestProjectIdentityUsesTheParentProject(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(filepath.Join(root, ".project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	document := `version = 1
id = "prj-parent1"
slug = "parent"
name = "Parent"

[component]
slug = "parent"
primary = true

[worktrees]
links = []
share_project_tree = false

[artifacts]
storage = "tree"

[conventions]
artifacts = true
agents_md = true
`
	if err := os.WriteFile(filepath.Join(root, ".project", "Project.toml"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := project.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	places, err := location.New(context.Background(), nested, func(_ context.Context, cwd string) (string, error) {
		if cwd == root || strings.HasPrefix(cwd, root+string(filepath.Separator)) {
			return root, nil
		}
		return cwd, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current := &session{location: places, projects: registry}
	got := current.projectIdentity(context.Background())
	if got == nil || got.ID != "prj-parent1" {
		t.Fatalf("project = %+v", got)
	}
	captured := places.Current()
	work := withCapturedLocation(context.Background(), captured)
	if _, err := places.Transition(context.Background(), t.TempDir(), false, func(location.Change) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got := current.projectIdentity(context.Background()); got != nil {
		t.Fatalf("new location retained old project: %+v", got)
	}
	if got := current.projectIdentity(work); got == nil || got.ID != "prj-parent1" {
		t.Fatalf("captured request drifted with cd: %+v", got)
	}
}
