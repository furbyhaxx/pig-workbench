package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/skills"
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

func TestSkillsChangeNamesOnlyWhatBecameAvailableOrWentAway(t *testing.T) {
	list := func(names ...string) string {
		var listed []skills.Listed
		for _, name := range names {
			listed = append(listed, skills.Listed{Name: name, Description: "about " + name, Location: "skill://" + name})
		}
		return skillsListing(listed)
	}
	if got := skillsListing(nil); got != "" {
		t.Fatalf("an empty catalog has no listing, got %q", got)
	}
	held := list("alpha", "beta")
	if !strings.Contains(held, "- skill://alpha: about alpha") || !strings.HasPrefix(held, skillsListingIntro) {
		t.Fatalf("listing = %q", held)
	}
	if change := skillsChange(held, strings.Replace(held, "about beta", "", 1)); change != "" {
		t.Fatalf("a shortened description is no availability change, got %q", change)
	}
	change := skillsChange(held, list("beta", "gamma"))
	want := skillsChangeIntro + "\nAvailable now:\n- skill://gamma: about gamma\nNo longer available:\n- skill://alpha"
	if change != want {
		t.Fatalf("change = %q, want %q", change, want)
	}
	if strings.Contains(change, "beta") {
		t.Fatal("an unchanged skill must not be named again")
	}
	if gone := skillsChange(held, ""); gone != skillsChangeIntro+"\nNo longer available:\n- skill://alpha\n- skill://beta" {
		t.Fatalf("withdrawal of every skill = %q", gone)
	}
	lines := bodyLines(change)
	if got := skillsRowSummary(lines[0], lines[1:]); got != "+gamma, -alpha" {
		t.Fatalf("change row = %q", got)
	}
	lines = bodyLines(held)
	if got := skillsRowSummary(lines[0], lines[1:]); got != "2 skills" {
		t.Fatalf("listing row = %q", got)
	}
}

func TestSkillsListingBodyIsTheBudgetedLinesAfterOneIntroLine(t *testing.T) {
	var items []skills.Skill
	for i := range 40 {
		items = append(items, skills.Skill{Name: fmt.Sprintf("skill-%02d", i), Description: strings.Repeat("routing words ", 30)})
	}
	body := skillsListing(skills.List(items, 4096))
	lines := strings.TrimPrefix(body, skillsListingIntro+"\n") + "\n"
	if len(lines) > 4096 {
		t.Fatalf("the listed lines are %d bytes over the 4096 budget", len(lines))
	}
	if order, _ := skillLines(body); len(order) != 40 {
		t.Fatalf("listed %d of 40 skills", len(order))
	}
}
