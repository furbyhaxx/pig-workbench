package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanReadsFrontmatterAndHidesNestedUntilLoad(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "alpha", "SKILL.md"), "alpha", "the alpha skill", "")
	writeSkill(t, filepath.Join(root, "alpha", "skills", "beta", "SKILL.md"), "beta", "nested", "")
	catalog, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	visible := Visible(catalog, root, nil)
	if len(visible) != 1 || visible[0].Name != "alpha" {
		t.Fatalf("visible = %+v", visible)
	}
	loaded := map[string]bool{"alpha": true}
	visible = Visible(catalog, root, loaded)
	if len(visible) != 2 {
		t.Fatalf("nested skill stayed hidden: %+v", visible)
	}
}

func TestScanKeepsRootSkillsVisibleWhenRootIsNamedSkills(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agents", "skills")
	writeSkill(t, filepath.Join(root, "alpha", "SKILL.md"), "alpha", "top level", "")
	writeSkill(t, filepath.Join(root, "alpha", "skills", "beta", "SKILL.md"), "beta", "nested", "")
	catalog, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	visible := Visible(catalog, root, nil)
	if len(visible) != 1 || visible[0].Name != "alpha" {
		t.Fatalf("top-level skills must be visible and nested skills hidden: %+v", visible)
	}
	if visible = Visible(catalog, root, map[string]bool{"alpha": true}); len(visible) != 2 {
		t.Fatalf("loading the parent must reveal the nested skill: %+v", visible)
	}
}

func TestWhenFilesHidesUnmatchedSkill(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "rust", "SKILL.md"), "rust", "rust only", "Cargo.toml")
	catalog, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(Visible(catalog, root, nil)) != 0 {
		t.Fatal("skill listed without its when file")
	}
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(Visible(catalog, root, nil)) != 1 {
		t.Fatal("skill hidden after its when file appeared")
	}
}

func writeSkill(t *testing.T, path, name, description, whenFile string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	front := "---\nname: " + name + "\ndescription: " + description + "\n"
	if whenFile != "" {
		front += "when:\n  files: [\"" + whenFile + "\"]\n"
	}
	front += "---\nbody\n"
	if err := os.WriteFile(path, []byte(front), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNestedSkillCollidingWithTopLevelNameIsQualifiedByParent(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".agents", "skills")
	writeSkill(t, filepath.Join(root, "alpha", "SKILL.md"), "alpha", "top", "")
	writeSkill(t, filepath.Join(root, "beta", "SKILL.md"), "beta", "top beta", "")
	writeSkill(t, filepath.Join(root, "alpha", "skills", "beta", "SKILL.md"), "beta", "nested beta", "")
	catalog, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := Find(catalog, "alpha/beta")
	if err != nil || nested.Description != "nested beta" {
		t.Fatalf("nested skill must load as alpha/beta: %+v %v", nested, err)
	}
	if top, err := Find(catalog, "beta"); err != nil || top.Description != "top beta" {
		t.Fatalf("top-level beta lost: %+v %v", top, err)
	}
}
