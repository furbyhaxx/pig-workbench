package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func front(t *testing.T, root, name, frontmatter string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\ndescription: about "+name+"\n"+frontmatter+"---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(items []Skill) string {
	var out []string
	for _, item := range items {
		out = append(out, item.Name)
	}
	return strings.Join(out, ",")
}

// SPEC-0014 §8.3: every listed kind must hold and within a kind any entry; a recursive glob finds a
// file at any depth but not one git ignores.
func TestWhenNeedsEveryKindAndAnyEntryOfAKind(t *testing.T) {
	skillsRoot, repo := t.TempDir(), t.TempDir()
	front(t, skillsRoot, "deep", "when:\n  files: [\"go.mod\", \"**/*.rs\"]\n")
	front(t, skillsRoot, "both", "when:\n  files:\n    - \"**/*.rs\"\n  env:\n    - WORKBENCH_TEST_DB\n")
	catalog, err := Scan([]string{skillsRoot})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, rel), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore")
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("target/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	write("target/debug/gen.rs")
	if got := names(Visible(catalog, repo, nil)); got != "" {
		t.Fatalf("an ignored file must not satisfy a glob, visible = %q", got)
	}
	write("crates/a/src/lib.rs")
	if got := names(Visible(catalog, repo, nil)); got != "deep" {
		t.Fatalf("files holds for deep only while env is unset, visible = %q", got)
	}
	t.Setenv("WORKBENCH_TEST_DB", "")
	if got := names(Visible(catalog, repo, nil)); got != "both,deep" && got != "deep,both" {
		t.Fatalf("a set variable, even empty, satisfies env; visible = %q", got)
	}
}

func TestInvalidWhenListsUnconditionallyAndInvalidAutoLoadIsIgnoredBothReported(t *testing.T) {
	root := t.TempDir()
	front(t, root, "odd", "when:\n  branch: [\"main\"]\nauto_load:\n  touched: [\"[\"]\n")
	catalog, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(Visible(catalog, root, nil)); got != "odd" {
		t.Fatalf("a skill with an invalid when must be listed, visible = %q", got)
	}
	skill := catalog.Skills[0]
	if len(skill.AutoTouched) != 0 || len(skill.Problems) != 2 || !strings.Contains(skill.Problems[0], `unknown key "branch"`) || !strings.Contains(skill.Problems[1], "auto_load ignored") {
		t.Fatalf("problems = %q, auto = %q", skill.Problems, skill.AutoTouched)
	}
}

func TestAutoLoadMatchesTouchedPathsBelowTheRoot(t *testing.T) {
	root := t.TempDir()
	skills := []Skill{
		{Name: "byname", AutoTouched: []string{"*.sql"}},
		{Name: "bypath", AutoTouched: []string{"db/**/*.sql"}},
		{Name: "needsfile", AutoTouched: []string{"*.sql"}, AutoFiles: []string{"schema.lock"}},
		{Name: "declined", AutoTouched: []string{"*.sql"}},
		{Name: "child", Parent: "byname", AutoTouched: []string{"*.sql"}},
	}
	var conditions Conditions
	got := names(conditions.AutoLoad(context.Background(), skills, root, []string{filepath.Join(root, "db", "migrations", "1.sql")}, map[string]bool{"declined": true}))
	if got != "byname,bypath" {
		t.Fatalf("auto-load = %q", got)
	}
	if got = names(conditions.AutoLoad(context.Background(), skills, root, []string{filepath.Join(root, "q.sql")}, nil)); got != "byname,declined" {
		t.Fatalf("a path pattern must not match a file elsewhere, auto-load = %q", got)
	}
}

// SPEC-0014 §8.4: a sub-skill whose name collides with a top-level skill is addressed as parent/child.
func TestNestedSkillCollidingWithATopLevelNameIsQualifiedByItsParent(t *testing.T) {
	root := t.TempDir()
	front(t, root, "alpha", "")
	front(t, root, "beta", "")
	front(t, filepath.Join(root, "alpha", "skills"), "beta", "when:\n  files: [\"never\"]\n")
	catalog, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if got := names(Visible(catalog, root, nil)); got != "alpha,beta" {
		t.Fatalf("visible before load = %q", got)
	}
	if got := names(Visible(catalog, root, map[string]bool{"alpha": true})); got != "alpha,beta,alpha/beta" {
		t.Fatalf("conditions do not apply to a sub-skill; visible after load = %q", got)
	}
}
