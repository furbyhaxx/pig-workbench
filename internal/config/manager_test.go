package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func setup(t *testing.T, user, project string) (*Manager, Paths) {
	t.Helper()
	root := t.TempDir()
	paths := Paths{User: filepath.Join(root, "agent", "Workbench.toml"), Project: filepath.Join(root, "project", ".pig", "Workbench.toml")}
	for path, text := range map[string]string{paths.User: user, paths.Project: project} {
		if text != "" {
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	manager, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	return manager, paths
}
func trust() (bool, error) { return true, nil }
func current(t *testing.T, m *Manager) *Snapshot {
	t.Helper()
	s, err := m.Current()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLayersScopeAndReset(t *testing.T) {
	m, paths := setup(t, "version = 1\n[read]\nmax-lines = 800\n[guides]\ndisabled = ['file','http']\n", "[read]\nmax-lines = 400\n")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if current(t, m).Read().MaxLines != 800 {
		t.Fatal("untrusted project was loaded")
	}
	if err := m.Load(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	base := current(t, m)
	if base.Read().MaxLines != 400 {
		t.Fatal("project override lost")
	}
	if _, err := m.Save(context.Background(), Runtime, base, []Edit{{Key: "read.max-lines", Value: int64(600)}, {Key: "guides.disabled", Value: []string{}}}, trust); err != nil {
		t.Fatal(err)
	}
	s := current(t, m)
	entry, _ := s.Get("guides.disabled")
	if s.Read().MaxLines != 600 || entry.Scope != Runtime || len(entry.Value.([]string)) != 0 {
		t.Fatal("runtime precedence or empty array replacement lost")
	}
	if _, err := m.Save(context.Background(), Runtime, s, []Edit{{Key: "read.max-lines", Reset: true}}, trust); err != nil {
		t.Fatal(err)
	}
	if current(t, m).Read().MaxLines != 400 {
		t.Fatal("reset wrote default rather than inheriting")
	}
	bytes, err := os.ReadFile(paths.Project)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != "[read]\nmax-lines = 400\n" {
		t.Fatal("runtime override touched project file")
	}
}

func TestProjectCannotWidenPolicyOrSetTrust(t *testing.T) {
	for _, project := range []string{"[read]\nmax-lines = 900\n", "[secrets]\nenabled = false\n", "[edit]\nenforce-seen-lines = false\n", "[trust]\nrules = []\n"} {
		m, _ := setup(t, "[read]\nmax-lines = 800\n", project)
		if err := m.Load(context.Background(), true); err == nil {
			t.Fatalf("project policy accepted: %s", project)
		}
	}
	m, _ := setup(t, "", "[edit]\nenforce-seen-lines = true\n")
	if err := m.Load(context.Background(), true); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedReloadIsStaleAndSourceSafe(t *testing.T) {
	m, paths := setup(t, "[read]\nmax-lines = 100\n", "")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	base := current(t, m)
	cases := []string{"[read]\nmax-lines = 10\nmax-lines = 20\n", "version = 2\n", "[read]\nmax-lines = 'do-not-echo-this-value'\n", "[read\n"}
	for _, text := range cases {
		if err := os.WriteFile(paths.User, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		err := m.Load(context.Background(), false)
		if err == nil {
			t.Fatal("invalid file loaded")
		}
		if strings.Contains(err.Error(), "do-not-echo-this-value") {
			t.Fatal("value leaked in diagnostic")
		}
		previous, status := m.Inspect()
		if previous != base || !status.Stale || !status.Ready {
			t.Fatal("last valid snapshot was lost or error hidden")
		}
		if _, err := m.Current(); err == nil {
			t.Fatal("stale configuration admitted work")
		}
		if _, err := m.Save(context.Background(), User, base, []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil); err == nil {
			t.Fatal("malformed file overwritten")
		}
		actual, err := os.ReadFile(paths.User)
		if err != nil {
			t.Fatal(err)
		}
		if string(actual) != text {
			t.Fatal("invalid original replaced")
		}
	}
}

func TestRoundTripUnknownFieldsAndExplicitValues(t *testing.T) {
	original := "# top\nversion = 1\n\n[read] # table\nmax-lines = 100 # keep me\nfuture = { marker = 'unchanged', values = [1, 2] } # future comment\n\n[secrets]\nenabled = true\n"
	m, paths := setup(t, original, "")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(current(t, m).Warnings()) != 2 {
		t.Fatalf("unknown fields not visible: %v", current(t, m).Warnings())
	}
	changes := []Edit{{Key: "read.max-lines", Value: int64(200)}, {Key: "secrets.enabled", Value: false}, {Key: "jobs.yield-after-ms", Value: int64(0)}, {Key: "guides.disabled", Value: []string{}}}
	result, err := m.Save(context.Background(), User, current(t, m), changes, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Saved || !result.Published || len(result.Pending) != 3 {
		t.Fatalf("save/apply status wrong: %+v", result)
	}
	bytes, err := os.ReadFile(paths.User)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{"# top", "# table", "# keep me", "# future comment", "'unchanged'"} {
		if !strings.Contains(string(bytes), comment) {
			t.Fatalf("round-trip lost %q", comment)
		}
	}
	parsed, err := parseDocument(paths.User, bytes, definitions())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.tree["read"].(map[string]any)["future"], map[string]any{"marker": "unchanged", "values": []any{int64(1), int64(2)}}) {
		t.Fatal("unknown data changed")
	}
	if parsed.values["secrets.enabled"] != false || parsed.values["jobs.yield-after-ms"] != int64(0) || len(parsed.values["guides.disabled"].([]string)) != 0 {
		t.Fatal("false/zero/empty array not saved")
	}
	info, err := os.Stat(paths.User)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("settings file is not private")
	}
	if _, err := m.Save(context.Background(), User, current(t, m), []Edit{{Key: "read.max-lines", Reset: true}}, nil); err != nil {
		t.Fatal(err)
	}
	resetBytes, err := os.ReadFile(paths.User)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resetBytes), "# keep me") {
		t.Fatal("reset discarded the user's attached comment")
	}
}

func TestInlineTableInsertionAndDisjointSaves(t *testing.T) {
	first, paths := setup(t, "read = { max-lines = 100, future = 'keep' }\n", "")
	second, err := New(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []*Manager{first, second} {
		if err := m.Load(context.Background(), false); err != nil {
			t.Fatal(err)
		}
	}
	base := current(t, second)
	if _, err := first.Save(context.Background(), User, current(t, first), []Edit{{Key: "read.max-bytes", Value: int64(2048)}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Save(context.Background(), User, base, []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil); err != nil {
		t.Fatal(err)
	}
	policy := current(t, second).Read()
	if policy.MaxBytes != 2048 || policy.MaxLines != 200 {
		t.Fatalf("disjoint save clobbered other session: %+v", policy)
	}
	if _, err := second.Save(context.Background(), User, base, []Edit{{Key: "read.max-lines", Value: int64(300)}}, nil); err == nil {
		t.Fatal("same-key conflict was silently overwritten")
	}
}

func TestSymlinkSaveAndTrustRevocation(t *testing.T) {
	m, paths := setup(t, "[read]\nmax-lines = 100\n", "")
	target := filepath.Join(filepath.Dir(paths.User), "settings-target.toml")
	if err := os.Rename(paths.User, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("settings-target.toml", paths.User); err != nil {
		t.Fatal(err)
	}
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(context.Background(), User, current(t, m), []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(paths.User)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "max-lines = 200") {
		t.Fatal("symlink target was not updated")
	}
	calls := 0
	revoked := func() (bool, error) { calls++; return calls == 1, nil }
	if _, err := m.Save(context.Background(), Project, current(t, m), []Edit{{Key: "read.max-lines", Value: int64(100)}}, revoked); err == nil {
		t.Fatal("revoked trust ignored")
	}
	if _, err := os.Stat(paths.Project); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("project settings written after revocation")
	}
}

func TestSavedButNotAppliedAndLockCancellation(t *testing.T) {
	m, paths := setup(t, "[read]\nmax-lines = 100\n", "")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	base := current(t, m)
	m.reconcile = func(context.Context, *Snapshot, *Snapshot) error { return errors.New("consumer restart failed") }
	result, err := m.Save(context.Background(), User, base, []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil)
	if err == nil || !result.Saved || result.Published {
		t.Fatalf("consumer failure hidden: %+v %v", result, err)
	}
	retained, status := m.Inspect()
	if retained != base || !status.Stale {
		t.Fatal("consumer failure published unusable configuration")
	}
	m.reconcile = nil
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if current(t, m).Read().MaxLines != 200 {
		t.Fatal("recovery did not apply saved value")
	}
	lock := flock.New(paths.User + ".lock")
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.Save(ctx, User, current(t, m), []Edit{{Key: "read.max-lines", Value: int64(300)}}, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock did not honor deadline: %v", err)
	}
}
