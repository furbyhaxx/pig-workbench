package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestSkillReadLoadsAndUnloadDeclines(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: alpha\ndescription: alpha\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	current := &session{skills: &skillState{loaded: map[string]bool{}, declined: map[string]bool{}, replayed: map[string]bool{}, roots: func() []string { return []string{root} }}}
	provider := skillProvider{source: func() (*session, bool) { return current, true }}
	request := resource.ReadRequest{URI: "skill://alpha", Requested: "skill://alpha", Budget: resource.Budget{Bytes: 4096, Lines: 40}}
	first, err := provider.Read(context.Background(), request)
	if err != nil || !strings.Contains(first.Content, "body") {
		t.Fatalf("first = %q %v", first.Content, err)
	}
	second, err := provider.Read(context.Background(), request)
	if err != nil || !strings.Contains(second.Content, "already loaded") {
		t.Fatalf("second = %q %v", second.Content, err)
	}
	if _, err := provider.unload(current, "skill://alpha/ctl", "unload"); err != nil {
		t.Fatal(err)
	}
	current.skills.mu.Lock()
	declined := current.skills.declined["alpha"]
	loaded := current.skills.loaded["alpha"]
	current.skills.mu.Unlock()
	if !declined || loaded {
		t.Fatalf("declined=%v loaded=%v", declined, loaded)
	}
}

func TestUnloadWithdrawsRevealedSubSkills(t *testing.T) {
	root := t.TempDir()
	for path, name := range map[string]string{"alpha/SKILL.md": "alpha", "alpha/skills/beta/SKILL.md": "beta"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("---\nname: "+name+"\ndescription: d\n---\nbody-"+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	current := &session{skills: &skillState{loaded: map[string]bool{}, declined: map[string]bool{}, replayed: map[string]bool{}, roots: func() []string { return []string{root} }}}
	provider := skillProvider{source: func() (*session, bool) { return current, true }}
	for _, name := range []string{"alpha", "beta"} {
		uri := "skill://" + name
		if _, err := provider.Read(context.Background(), resource.ReadRequest{URI: uri, Requested: uri, Budget: resource.Budget{Bytes: 4096, Lines: 40}}); err != nil {
			t.Fatal(err)
		}
	}
	withdrawn, err := provider.unload(current, "skill://alpha/ctl", "unload")
	if err != nil {
		t.Fatal(err)
	}
	if len(withdrawn) != 2 || current.skills.loaded["beta"] || current.skills.loaded["alpha"] {
		t.Fatalf("withdrawn=%v loaded=%v", withdrawn, current.skills.loaded)
	}
	if !current.skills.declined["alpha"] || current.skills.declined["beta"] {
		t.Fatalf("only the explicitly unloaded skill is declined: %v", current.skills.declined)
	}
}
