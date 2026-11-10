package host

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/resource"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/location"
)

func TestDiscoveryAndLocation(t *testing.T) {
	manager, err := location.New(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &session{id: "session", generation: "generation", location: manager}
	a := &app{}
	params, _ := json.Marshal(api.DiscoveryRequest{Majors: []int{1}, Capabilities: []string{"location"}})
	result, failure := a.dispatchService(context.Background(), sdk.Context{}, s, api.Request{Method: "discover", Params: params})
	if failure != nil {
		t.Fatal(failure)
	}
	discovery := result.(api.Discovery)
	if discovery.Generation != "generation" || discovery.SessionID != "session" || discovery.Location.Cwd != manager.Current().Cwd {
		t.Fatalf("wrong binding: %+v", discovery)
	}
	if !slices.Equal(discovery.Capabilities, advertisedCapabilities) {
		t.Fatalf("unwired capability advertised: %+v", discovery)
	}
	result, failure = a.dispatchService(context.Background(), sdk.Context{}, s, api.Request{Method: "location.get", Params: json.RawMessage(`{}`)})
	if failure != nil || result.(api.Location).Cwd != manager.Current().Cwd {
		t.Fatalf("location service differs: %v %v", result, failure)
	}
	params, _ = json.Marshal(api.DiscoveryRequest{Majors: []int{1}, Capabilities: []string{"secrets"}})
	_, failure = a.dispatchService(context.Background(), sdk.Context{}, s, api.Request{Method: "discover", Params: params})
	if failure == nil || failure.Code != "unsupported" {
		t.Fatal("unavailable capability succeeded")
	}
}

func TestCancellationIsOwnerScoped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &session{generation: "generation", requests: map[string]activeRequest{"owner\x00instance\x00pending": {owner: "owner", cancel: cancel}}}
	request := api.Request{Generation: "generation", From: api.Identity{Owner: "other", Instance: "instance"}, Params: json.RawMessage(`{"requestId":"pending"}`)}
	a := &app{}
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("cross-owner cancel applied")
	}
	request.From.Owner = "owner"
	request.Generation = "stale"
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("stale generation cancel applied")
	}
	request.Generation = "generation"
	request.From.Instance = "other-instance"
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("cross-instance cancel applied")
	}
	request.From.Instance = "instance"
	if err := a.cancelService(s, request); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("owner cancellation lost")
	}
}

func TestCfgProviderExposesSettings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agent", "Workbench.toml")
	if err := os.WriteFile(path, []byte("[read]\nmax-lines = 400\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := config.New(config.Paths{User: path, Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	provider := cfgProvider{source: func() (*config.Snapshot, config.Status) { return manager.Inspect() }}
	budget := resource.Budget{Bytes: 51200, Lines: 2000}
	status, err := provider.Read(context.Background(), resource.ReadRequest{URI: "cfg://workbench/status", Requested: "cfg://workbench/status", Budget: budget})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Content, `"configRevision"`) || !strings.Contains(status.Content, `"maxLines": 400`) {
		t.Fatalf("status does not expose effective settings: %s", status.Content)
	}
	if !strings.Contains(status.Content, `"maxBytes"`) {
		t.Fatalf("typed read policy missing: %s", status.Content)
	}
	entry, err := provider.Read(context.Background(), resource.ReadRequest{URI: "cfg://read/max-lines", Requested: "cfg://read/max-lines", Budget: budget})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(entry.Content, `"scope": "user"`) || !strings.Contains(entry.Content, `"value": 400`) {
		t.Fatalf("entry lacks provenance: %s", entry.Content)
	}
	selected, err := resource.ParseSelector(":2:raw")
	if err != nil {
		t.Fatal(err)
	}
	ranged, err := provider.Read(context.Background(), resource.ReadRequest{URI: "cfg://read/max-lines", Requested: "cfg://read/max-lines", Selector: selected, Budget: budget})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(ranged.Content, "\n") != 1 || ranged.Facts.ResourceComplete || len(ranged.Facts.FullyShownRanges) != 1 {
		t.Fatalf("selector not applied to produced content: %+v", ranged.Facts)
	}
	if _, err := provider.Read(context.Background(), resource.ReadRequest{URI: "cfg://absent/key", Requested: "cfg://absent/key", Budget: budget}); !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("unknown key not reported: %v", err)
	}
}

func TestArtifactProviderResolvesProjectTree(t *testing.T) {
	repo := t.TempDir()
	tree := filepath.Join(repo, ".project", "specs")
	if err := os.MkdirAll(tree, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "0001-a.md"), []byte("---\ndocument:\n  type: \"spec\"\n  id: \"SPEC-0001\"\n  title: \"A: contract\"\n  status: \"published\"\n  revision: 3\n  changelog:\n    - revision: 3\n      title: \"nested\"\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "cmd", "tool")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	locations, err := location.New(context.Background(), nested, nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := artifactProvider{location: locations}
	page, err := provider.Read(context.Background(), resource.ReadRequest{URI: "artifact://specs/0001-a.md", Requested: "artifact://specs/0001-a.md", Budget: resource.Budget{Bytes: 4096, Lines: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Content, "body") {
		t.Fatalf("artifact content missing: %q", page.Content)
	}
	if page.Facts.CanonicalURI != "artifact://specs/0001-a.md" {
		t.Fatalf("canonical artifact URI lost: %q", page.Facts.CanonicalURI)
	}
	listing, err := provider.List(context.Background(), "artifact://specs", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].URI != "artifact://specs/0001-a.md" {
		t.Fatalf("listing URIs not rebased: %+v", listing.Entries)
	}
	if got := listing.Entries[0].Detail; got != "SPEC-0001 · A: contract · published · rev 3" {
		t.Fatalf("artifact columns = %q", got)
	}
	if _, err := provider.Read(context.Background(), resource.ReadRequest{URI: "artifact://../escape", Requested: "x", Budget: resource.Budget{Bytes: 4096, Lines: 100}}); err == nil {
		t.Fatal("path traversal accepted")
	}
	outside := t.TempDir()
	detached, err := location.New(context.Background(), outside, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (artifactProvider{location: detached}).Stat(context.Background(), "artifact://specs"); !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("missing artifact tree reported as %v", err)
	}
}

func TestCfgProviderServesIntegrationsPerOwner(t *testing.T) {
	provider := cfgProvider{
		source: func() (*config.Snapshot, config.Status) { return &config.Snapshot{}, config.Status{} },
		integrations: func(owner string) ([]string, string, bool) {
			if owner == "" {
				return []string{"alpha", "beta"}, "", true
			}
			return nil, "alpha  docs  key\n", owner == "alpha"
		},
	}
	listing, err := provider.List(context.Background(), "cfg://workbench/integrations/", "", 10)
	if err != nil || len(listing.Entries) != 2 || listing.Entries[1].URI != "cfg://workbench/integrations/beta" {
		t.Fatalf("owners = %+v, %v", listing, err)
	}
	read, err := provider.Read(context.Background(), resource.ReadRequest{URI: "cfg://workbench/integrations/alpha", Requested: "cfg://workbench/integrations/alpha", Budget: resource.Budget{Bytes: 4096, Lines: 100}})
	if err != nil || !strings.Contains(read.Content, "alpha  docs  key") {
		t.Fatalf("owner view = %+v, %v", read, err)
	}
	if _, err := provider.Read(context.Background(), resource.ReadRequest{URI: "cfg://workbench/integrations/missing", Requested: "cfg://workbench/integrations/missing", Budget: resource.Budget{Bytes: 4096, Lines: 100}}); !errors.Is(err, resource.ErrNotFound) {
		t.Fatalf("an unknown owner must be not found, got %v", err)
	}
}
