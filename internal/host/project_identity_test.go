package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

const parentProjectDocument = `version = 1
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

func TestProjectIdentityUsesTheParentProject(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", "deeper")
	if err := os.MkdirAll(filepath.Join(root, ".project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".project", "Project.toml"), []byte(parentProjectDocument), 0o644); err != nil {
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

func TestLocationSurfacesCarryTheProject(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".project", "Project.toml"), []byte(parentProjectDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := project.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	places, err := location.New(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: "session", generation: "generation", location: places, projects: registry}
	a := &app{}
	ctx := context.Background()

	params, _ := json.Marshal(api.DiscoveryRequest{Majors: []int{1}, Capabilities: []string{"location"}})
	result, failure := a.dispatchService(ctx, sdk.Context{}, s, api.Request{Method: "discover", Params: params})
	if failure != nil {
		t.Fatal(failure)
	}
	if got := result.(api.Discovery).Location.Project; got == nil || got.ID != "prj-parent1" || got.Slug != "parent" {
		t.Fatalf("discover location project = %+v", got)
	}
	result, failure = a.dispatchService(ctx, sdk.Context{}, s, api.Request{Method: "location.get", Params: json.RawMessage(`{}`)})
	if failure != nil {
		t.Fatal(failure)
	}
	if got := result.(api.Location).Project; got == nil || got.ID != "prj-parent1" {
		t.Fatalf("location.get project = %+v", got)
	}

	change, err := places.Transition(ctx, t.TempDir(), false, func(location.Change) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	event := s.apiLocationChange(ctx, change)
	if event.Previous.Project == nil || event.Previous.Project.ID != "prj-parent1" {
		t.Fatalf("previous side lost the project: %+v", event.Previous.Project)
	}
	if event.Current.Project != nil {
		t.Fatalf("a directory outside any project reports %+v", event.Current.Project)
	}
	encoded, err := json.Marshal(event.Current)
	if err != nil || strings.Contains(string(encoded), `"project"`) {
		t.Fatalf("project must be absent on the wire outside a project: %s %v", encoded, err)
	}
}
