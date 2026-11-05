package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func setupSecrets(t *testing.T, user, userSecrets, project, projectSecrets string) (*Manager, Paths) {
	t.Helper()
	root := t.TempDir()
	paths := Paths{
		User:           filepath.Join(root, "agent", "Workbench.toml"),
		Secrets:        filepath.Join(root, "agent", "Secrets.toml"),
		Project:        filepath.Join(root, "project", ".pig", "Workbench.toml"),
		ProjectSecrets: filepath.Join(root, "project", ".pig", "Secrets.toml"),
	}
	for path, text := range map[string]string{paths.User: user, paths.Secrets: userSecrets, paths.Project: project, paths.ProjectSecrets: projectSecrets} {
		if text == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager, paths
}

func list(t *testing.T, m *Manager, key string) []string {
	t.Helper()
	snapshot, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	entry, found := snapshot.Get(key)
	if !found {
		t.Fatalf("%s is not in the snapshot", key)
	}
	values, ok := entry.Value.([]string)
	if !ok {
		t.Fatalf("%s is not a string list", key)
	}
	return values
}

// TestSecretsFileAppendsAcrossLayers proves detection policy is a separate file
// whose arrays append user-then-project instead of replacing (SPEC-0010 §3).
func TestSecretsFileAppendsAcrossLayers(t *testing.T) {
	manager, _ := setupSecrets(t, "", "[secrets]\npatterns = ['user-one']\n", "", "[secrets]\npatterns = ['project-one']\nexclusions = ['fixture']\n")
	if err := manager.Load(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := list(t, manager, "secrets.patterns"); !reflect.DeepEqual(got, []string{"user-one", "project-one"}) {
		t.Fatalf("patterns did not append across layers: %v", got)
	}
	if got := list(t, manager, "secrets.exclusions"); !reflect.DeepEqual(got, []string{"fixture"}) {
		t.Fatalf("project exclusions were not applied: %v", got)
	}
	snapshot, _ := manager.Inspect()
	entry, _ := snapshot.Get("secrets.patterns")
	if entry.File == "" || !strings.HasSuffix(entry.File, filepath.Join(".pig", "Secrets.toml")) {
		t.Fatalf("a appended entry must name the file that supplied it: %+v", entry)
	}
}

// TestSecretsFileRejectsForeignSettings keeps the two files honest.
func TestSecretsFileRejectsForeignSettings(t *testing.T) {
	manager, _ := setupSecrets(t, "", "[edit]\nmode = 'replace'\n", "", "")
	if err := manager.Load(context.Background(), true); err == nil || !strings.Contains(err.Error(), "Secrets.toml") {
		t.Fatalf("a Workbench setting in Secrets.toml was accepted: %v", err)
	}
	manager, _ = setupSecrets(t, "[secrets]\npatterns = ['x']\n", "", "", "")
	if err := manager.Load(context.Background(), true); err == nil || !strings.Contains(err.Error(), "Secrets.toml") {
		t.Fatalf("a detection setting in Workbench.toml was accepted: %v", err)
	}
}

// TestProjectSecretsRespectTrust proves an untrusted project contributes nothing.
func TestProjectSecretsRespectTrust(t *testing.T) {
	manager, _ := setupSecrets(t, "", "[secrets]\npatterns = ['user-one']\n", "", "[secrets]\npatterns = ['project-one']\n")
	if err := manager.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := list(t, manager, "secrets.patterns"); !reflect.DeepEqual(got, []string{"user-one"}) {
		t.Fatalf("an untrusted project contributed detection policy: %v", got)
	}
}

// TestSaveRefusesDetectionPolicyEdits keeps the settings command honest.
func TestSaveRefusesDetectionPolicyEdits(t *testing.T) {
	manager, _ := setupSecrets(t, "", "", "", "")
	if err := manager.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := manager.Inspect()
	_, err := manager.Save(context.Background(), User, snapshot, []Edit{{Key: "secrets.patterns", Value: []string{"x"}}}, trust)
	if err == nil || !strings.Contains(err.Error(), "Secrets.toml") {
		t.Fatalf("editing detection policy through Workbench.toml was accepted: %v", err)
	}
}

// TestSecretsMalformedDiagnostics names the file and line, and keeps the last
// valid snapshot active instead of silently dropping policy.
func TestSecretsMalformedDiagnostics(t *testing.T) {
	manager, paths := setupSecrets(t, "", "[secrets]\npatterns = ['ok']\n", "", "")
	if err := manager.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Secrets, []byte("[secrets\npatterns = [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := manager.Load(context.Background(), false)
	if err == nil {
		t.Fatal("malformed Secrets.toml was accepted")
	}
	var problem *Error
	if !errors.As(err, &problem) || problem.Path != paths.Secrets || problem.Line == 0 {
		t.Fatalf("diagnostic does not name the file and line: %v", err)
	}
	retained, _ := manager.Inspect()
	if retained == nil {
		t.Fatal("the last valid snapshot was dropped")
	}
	entry, _ := retained.Get("secrets.patterns")
	if got := entry.Value.([]string); !reflect.DeepEqual(got, []string{"ok"}) {
		t.Fatalf("the last valid policy was not retained: %v", got)
	}
}
