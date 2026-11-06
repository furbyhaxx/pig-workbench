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
	if err := provider.unload(current, "skill://alpha/ctl", "unload"); err != nil {
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
