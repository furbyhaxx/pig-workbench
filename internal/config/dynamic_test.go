package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDynamicDefinitionsCannotWidenScopeAndDoNotReadUntrustedProjectValues(t *testing.T) {
	root := t.TempDir()
	paths := Paths{User: filepath.Join(root, "user.toml"), Project: filepath.Join(root, "project.toml")}
	if err := os.WriteFile(paths.Project, []byte("[tools.owner.key]\nenabled=false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	field := Definition{Key: "tools.owner.key.enabled", Kind: Boolean, Default: true, Project: AllowProject, Application: "live"}
	if err := manager.UpdateDynamic(t.Context(), []string{"tools", "owner", "key"}, []Definition{field}, true, false); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := manager.Current()
	if value, _ := snapshot.Get(field.Key); value.Value != true || value.Scope != Defaults {
		t.Fatalf("untrusted project preference applied: %+v", value)
	}
	if err := manager.UpdateDynamic(t.Context(), []string{"tools", "owner", "key"}, []Definition{field}, true, true); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.Current()
	if value, _ := snapshot.Get(field.Key); value.Value != false || value.Scope != Project {
		t.Fatalf("trusted project display preference not applied: %+v", value)
	}
	if err := manager.UpdateDynamic(t.Context(), []string{"tools", "owner", "key"}, nil, false, false); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.Current()
	if value, _ := snapshot.Get(field.Key); value.Value != true || value.Scope != Defaults {
		t.Fatalf("withdrawal retained a no-longer-trusted project layer: %+v", value)
	}
	if err := manager.UpdateDynamic(t.Context(), []string{"trust", "owner", "key"}, []Definition{field}, true, false); err == nil {
		t.Fatal("dynamic schema admitted outside display namespace")
	}
	field.Key = "trust.rules"
	if err := manager.UpdateDynamic(t.Context(), []string{"tools", "owner", "key"}, []Definition{field}, true, false); err == nil {
		t.Fatal("dynamic schema claimed an execution-policy field")
	}
}

func TestDynamicWithdrawalDoesNotRepairAnUnrelatedStaleConfiguration(t *testing.T) {
	root := t.TempDir()
	paths := Paths{User: filepath.Join(root, "user.toml"), Project: filepath.Join(root, "project.toml")}
	manager, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	field := Definition{Key: "tools.owner.key.enabled", Kind: Boolean, Default: true, Project: AllowProject, Application: "live"}
	if err := manager.UpdateDynamic(t.Context(), []string{"tools", "owner", "key"}, []Definition{field}, true, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.User, []byte("malformed=[\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err == nil {
		t.Fatal("malformed file was accepted")
	}
	if err := manager.UpdateDynamic(t.Context(), []string{"tools", "owner", "key"}, nil, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Current(); err == nil {
		t.Fatal("withdrawing a schema cleared an unrelated stale-file error")
	}
}

func TestDynamicDisplayDefinitionsPreserveQuotedKeysAndDormantOverrides(t *testing.T) {
	root := t.TempDir()
	paths := Paths{User: filepath.Join(root, "user.toml"), Project: filepath.Join(root, "project.toml")}
	if err := os.WriteFile(paths.User, []byte("# preferences\n[tools.\"owner.dot\".\"key.dot\"]\ndensity='compact' # keep me\nenabled=false\nunknown='preserved'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Project, []byte("[tools.\"owner.dot\".\"key.dot\"]\ndensity='full'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	namespace := []string{"tools", "owner.dot", "key.dot"}
	density := `tools."owner.dot"."key.dot".density`
	enabled := `tools."owner.dot"."key.dot".enabled`
	defs := []Definition{{Key: density, Kind: String, Default: "normal", Choices: []string{"compact", "normal", "full"}, Project: AllowProject, Application: "live"}, {Key: enabled, Kind: Boolean, Default: true, Project: AllowProject, Application: "live"}}
	if err := manager.UpdateDynamic(t.Context(), namespace, defs, true, true); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := snapshot.Get(density); !ok || value.Value != "full" || value.Scope != Project {
		t.Fatalf("dynamic precedence: %+v %v", value, ok)
	}
	if _, err := manager.Save(t.Context(), Runtime, snapshot, []Edit{{Key: density, Value: "compact"}}, func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.Current()
	if _, err := manager.Save(t.Context(), User, snapshot, []Edit{{Key: enabled, Value: true}}, func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(paths.User)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# keep me") || !strings.Contains(string(data), "unknown = 'preserved'") || !strings.Contains(string(data), `[tools."owner.dot"."key.dot"]`) {
		t.Fatalf("dynamic save rewrote unrelated data: %s", data)
	}
	if err := manager.UpdateDynamic(t.Context(), namespace, nil, false, true); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.Current()
	if value, ok := snapshot.Get(density); !ok || value.Value != "compact" || value.Scope != Runtime || value.Application != "unavailable" {
		t.Fatalf("dormant override lost: %+v %v", value, ok)
	}
	if err := manager.UpdateDynamic(t.Context(), namespace, defs, true, true); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.Current()
	if _, err := manager.Save(t.Context(), Runtime, snapshot, []Edit{{Key: density, Reset: true}}, func() (bool, error) { return true, nil }); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = manager.Current()
	if value, _ := snapshot.Get(density); value.Value != "full" || value.Scope != Project {
		t.Fatalf("reset did not reveal inherited project preference: %+v", value)
	}
	changed := []Definition{{Key: enabled, Kind: Integer, Default: int64(1), Min: 0, Max: 2, Project: AllowProject, Application: "live"}}
	if err := manager.UpdateDynamic(t.Context(), namespace, changed, true, true); err == nil {
		t.Fatal("boolean preference reinterpreted under an integer descriptor")
	}
	current, err := manager.Current()
	if err != nil {
		t.Fatalf("rejected schema invalidated working config: %v", err)
	}
	if value, _ := current.Get(enabled); value.Value != true {
		t.Fatalf("rejected schema changed active values: %+v", value)
	}
}
