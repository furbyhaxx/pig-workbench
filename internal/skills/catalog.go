// Package skills catalogs SKILL.md files and tracks which ones a session loaded.
package skills

import (
	"bufio"
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Skill is one discovered skill.
type Skill struct {
	Name        string
	Description string
	Dir         string
	File        string
	Parent      string
	WhenFiles   []string
	WhenEnv     []string
	AutoFiles   []string
	AutoTouched []string
	// Problems names invalid when or auto_load frontmatter; it is shown to the user, never to the model.
	Problems []string
	Bundled  bool
	Body     string
}

// Catalog is a point-in-time scan.
type Catalog struct {
	Skills []Skill
}

// Source is a skill another catalog already discovered: its name, routing text and SKILL.md.
type Source struct {
	Name        string
	Description string
	File        string
}

// Stamp identifies the files Build would read by path, size and modification time, without reading them.
// Two equal stamps mean Build returns the same catalog.
func Stamp(sources []Source, roots []string) string {
	var b strings.Builder
	note := func(path string) {
		if info, err := os.Stat(path); err == nil {
			b.WriteString(path + "|" + info.ModTime().String() + "|" + strconv.FormatInt(info.Size(), 10) + "\n")
		}
	}
	for _, source := range sources {
		b.WriteString(source.Name + "|" + source.Description + "|")
		note(source.File)
		children, _ := filepath.Glob(filepath.Join(filepath.Dir(source.File), "skills", "*", "SKILL.md"))
		for _, child := range children {
			note(child)
		}
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && !entry.IsDir() && entry.Name() == "SKILL.md" {
				note(path)
			}
			return nil
		})
	}
	return b.String()
}

// Scan walks each root for SKILL.md. A skills/ child of a skill is a nested skill.
func Scan(roots []string) (Catalog, error) { return Build(nil, roots) }

// Build merges discovered sources with a walk of roots; the first skill of a name wins and sources
// come first. A source keeps its discovered name and description; its skills/ children are nested skills.
func Build(sources []Source, roots []string) (Catalog, error) {
	var catalog Catalog
	seen := map[string]bool{}
	var found []Skill
	for _, source := range sources {
		dir := filepath.Dir(source.File)
		skill, err := readSkill(source.File, filepath.Dir(dir))
		if err != nil {
			continue
		}
		skill.Parent = ""
		if source.Name != "" {
			skill.Name = source.Name
		}
		if source.Description != "" {
			skill.Description = source.Description
		}
		found = append(found, skill)
		children, _ := filepath.Glob(filepath.Join(dir, "skills", "*", "SKILL.md"))
		for _, child := range children {
			if nested, err := readSkill(child, filepath.Dir(dir)); err == nil {
				found = append(found, nested)
			}
		}
	}
	for _, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() || entry.Name() != "SKILL.md" {
				return nil
			}
			skill, err := readSkill(path, root)
			if err != nil {
				return nil
			}
			found = append(found, skill)
			return nil
		})
		if err != nil {
			return catalog, err
		}
	}
	// A nested skill names its parent by the parent's catalog name, which can differ from the directory.
	parents := map[string]string{}
	for _, skill := range found {
		if skill.Parent == "" {
			if _, ok := parents[filepath.Clean(skill.Dir)]; !ok {
				parents[filepath.Clean(skill.Dir)] = skill.Name
			}
		}
	}
	for _, pass := range []bool{false, true} {
		for _, skill := range found {
			if (skill.Parent != "") != pass {
				continue
			}
			if name, ok := parents[filepath.Dir(filepath.Dir(skill.Dir))]; ok && pass {
				skill.Parent = name
			}
			if seen[skill.Name] && pass {
				skill.Name = skill.Parent + "/" + skill.Name
			}
			if seen[skill.Name] {
				continue
			}
			seen[skill.Name] = true
			catalog.Skills = append(catalog.Skills, skill)
		}
	}
	return catalog, nil
}

func readSkill(path, root string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	dir := filepath.Dir(path)
	skill := Skill{Name: filepath.Base(dir), Dir: dir, File: path}
	body := data
	if bytes.HasPrefix(data, []byte("---\n")) {
		rest := data[4:]
		if end := bytes.Index(rest, []byte("\n---")); end >= 0 {
			body = bytes.TrimLeft(rest[end+4:], "\n")
			parseFront(string(rest[:end]), &skill)
		}
	}
	if filepath.Clean(filepath.Dir(dir)) != filepath.Clean(root) && filepath.Base(filepath.Dir(dir)) == "skills" {
		skill.Parent = filepath.Base(filepath.Dir(filepath.Dir(dir)))
	}
	skill.Body = string(body)
	return skill, nil
}

// parseFront reads the frontmatter subset: name, description, bundle, and the when and auto_load
// blocks whose keys each hold a list, inline or as "- item" lines.
func parseFront(front string, skill *Skill) {
	blocks := map[string]map[string][]string{}
	section, key := "", ""
	sc := bufio.NewScanner(strings.NewReader(front))
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		nested := line != strings.TrimLeft(line, " \t")
		if nested && section != "" {
			if item, ok := strings.CutPrefix(trim, "- "); ok {
				blocks[section][key] = append(blocks[section][key], unquote(item))
				continue
			}
			name, value, _ := strings.Cut(trim, ":")
			key = strings.TrimSpace(name)
			blocks[section][key] = append(blocks[section][key], splitList(strings.TrimSpace(value))...)
			continue
		}
		name, value, ok := strings.Cut(trim, ":")
		if !ok || nested {
			continue
		}
		section = ""
		value = strings.TrimSpace(value)
		switch name {
		case "name":
			if value = unquote(value); value != "" {
				skill.Name = value
			}
		case "description":
			skill.Description = unquote(value)
		case "bundle":
			skill.Bundled = value == "true"
		case "when", "auto_load":
			section, key = name, ""
			blocks[section] = map[string][]string{}
		}
	}
	if when, ok := blocks["when"]; ok {
		if problem := checkBlock(when, "files", "env"); problem != "" {
			skill.Problems = append(skill.Problems, "when ignored, skill listed unconditionally: "+problem)
		} else {
			skill.WhenFiles, skill.WhenEnv = when["files"], when["env"]
		}
	}
	if auto, ok := blocks["auto_load"]; ok {
		if problem := checkBlock(auto, "files", "touched"); problem != "" {
			skill.Problems = append(skill.Problems, "auto_load ignored: "+problem)
		} else {
			skill.AutoFiles, skill.AutoTouched = auto["files"], auto["touched"]
		}
	}
}

// checkBlock names what makes a condition block invalid: an unknown key, an empty list or a bad glob.
func checkBlock(block map[string][]string, allowed ...string) string {
	if len(block) == 0 {
		return "no conditions"
	}
	for _, key := range slices.Sorted(maps.Keys(block)) {
		if !slices.Contains(allowed, key) {
			return "unknown key " + strconv.Quote(key)
		}
		if len(block[key]) == 0 {
			return key + " is empty"
		}
		if key == "env" {
			continue
		}
		for _, item := range block[key] {
			if !doublestar.ValidatePattern(item) {
				return "invalid pattern " + strconv.Quote(item)
			}
		}
	}
	return ""
}

func unquote(value string) string { return strings.Trim(strings.TrimSpace(value), `"'`) }

// splitList reads an inline list or one scalar; an empty value yields nothing.
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(strings.Trim(value, "[]"), ",") {
		if part = unquote(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Find returns one skill by name.
func Find(catalog Catalog, name string) (Skill, error) {
	for _, skill := range catalog.Skills {
		if skill.Name == name {
			return skill, nil
		}
	}
	return Skill{}, errors.New("skill not found")
}
