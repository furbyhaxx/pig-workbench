// Package skills catalogs SKILL.md files and tracks which ones a session loaded.
package skills

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Skill is one discovered skill.
type Skill struct {
	Name        string
	Description string
	Dir         string
	File        string
	Parent      string
	WhenFiles   []string
	AutoFiles   []string
	AutoTouched []string
	Bundled     bool
	Body        string
}

// Catalog is a point-in-time scan.
type Catalog struct {
	Skills []Skill
}

// Scan walks each root for SKILL.md. A skills/ child of a skill is a nested skill.
func Scan(roots []string) (Catalog, error) {
	var catalog Catalog
	seen := map[string]bool{}
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
			if seen[skill.Name] {
				return nil
			}
			seen[skill.Name] = true
			catalog.Skills = append(catalog.Skills, skill)
			return nil
		})
		if err != nil {
			return catalog, err
		}
	}
	return catalog, nil
}

func readSkill(path, root string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	name := filepath.Base(filepath.Dir(path))
	description := ""
	var when, autoFiles, autoTouched []string
	var bundled bool
	body := data
	if bytes.HasPrefix(data, []byte("---\n")) {
		rest := data[4:]
		end := bytes.Index(rest, []byte("\n---"))
		if end >= 0 {
			front := string(rest[:end])
			body = bytes.TrimLeft(rest[end+4:], "\n")
			name, description, when, autoFiles, autoTouched, bundled = parseFront(front, name)
		}
	}
	parent := ""
	dir := filepath.Dir(path)
	if filepath.Clean(filepath.Dir(dir)) != filepath.Clean(root) && filepath.Base(filepath.Dir(dir)) == "skills" {
		parent = filepath.Base(filepath.Dir(filepath.Dir(dir)))
	}
	return Skill{Name: name, Description: description, Dir: dir, File: path, Parent: parent, WhenFiles: when, AutoFiles: autoFiles, AutoTouched: autoTouched, Bundled: bundled, Body: string(body)}, nil
}

func parseFront(front, name string) (string, string, []string, []string, []string, bool) {
	var description string
	var when, autoFiles, autoTouched []string
	var bundled bool
	section := ""
	inFiles := false
	sc := bufio.NewScanner(strings.NewReader(front))
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  - ") && inFiles {
			item := strings.Trim(strings.TrimPrefix(trim, "- "), `"'`)
			switch section {
			case "when":
				when = append(when, item)
			case "auto-files":
				autoFiles = append(autoFiles, item)
			case "auto-touched":
				autoTouched = append(autoTouched, item)
			}
			continue
		}
		inFiles = false
		key, value, ok := strings.Cut(trim, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "name":
			if value != "" {
				name = value
			}
		case "description":
			description = value
		case "files":
			target := &when
			if section == "auto_load" {
				target = &autoFiles
			}
			if strings.HasPrefix(value, "[") {
				*target = append(*target, splitList(value)...)
			} else {
				inFiles = true
				if section == "auto_load" {
					section = "auto-files"
				} else {
					section = "when"
				}
			}
		case "touched":
			if strings.HasPrefix(value, "[") {
				autoTouched = append(autoTouched, splitList(value)...)
			} else {
				inFiles = true
				section = "auto-touched"
			}
		case "when", "auto_load":
			section = key
		case "bundle":
			bundled = value == "true"
		}
	}
	return name, description, when, autoFiles, autoTouched, bundled
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(strings.Trim(value, "[]"), ",") {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Visible reports skills whose when.files match root, hiding nested skills until parent is loaded.
func Visible(catalog Catalog, root string, loaded map[string]bool) []Skill {
	var out []Skill
	for _, skill := range catalog.Skills {
		if skill.Parent != "" && !loaded[skill.Parent] {
			continue
		}
		if !whenHolds(root, skill.WhenFiles) {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func whenHolds(root string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
		if _, err := os.Stat(filepath.Join(root, pattern)); err == nil {
			return true
		}
	}
	return false
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
