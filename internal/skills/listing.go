package skills

import (
	"path/filepath"
	"strings"
)

// Listed is one prompt line after the budget ladder.
type Listed struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Location    string `json:"location"`
}

// List applies the listing ladder until the listing fits the budget in bytes: bundled skills lose their
// descriptions, then other descriptions are cut to 80 characters, then entries lose their description from
// the last-ranked upwards. Every skill remains listed by name, so a listing of names alone may exceed a
// budget that is smaller than the names.
func List(items []Skill, budget int) []Listed {
	out := make([]Listed, len(items))
	for i, skill := range items {
		out[i] = Listed{Name: skill.Name, Description: skill.Description, Location: "skill://" + skill.Name}
	}
	if budget <= 0 || bytesOf(out) <= budget {
		return out
	}
	for i := range out {
		if items[i].Bundled {
			out[i].Description = ""
		}
	}
	if bytesOf(out) <= budget {
		return out
	}
	for i := range out {
		if cut := []rune(out[i].Description); len(cut) > 80 {
			out[i].Description = strings.TrimSpace(string(cut[:79])) + "…"
		}
	}
	for i := len(out) - 1; i >= 0 && bytesOf(out) > budget; i-- {
		out[i].Description = ""
	}
	return out
}

// bytesOf is the size of the listing as delivered: one "- location: description" line per skill.
func bytesOf(items []Listed) int {
	n := 0
	for _, item := range items {
		n += len("- ") + len(item.Location) + 1
		if item.Description != "" {
			n += len(": ") + len(item.Description)
		}
	}
	return n
}

// AutoLoad reports skills whose auto_load files exist or whose touched globs match a touched path.
func AutoLoad(items []Skill, root string, touched []string, declined map[string]bool) []Skill {
	var out []Skill
	for _, skill := range items {
		if skill.Parent != "" || declined[skill.Name] {
			continue
		}
		if len(skill.AutoFiles) == 0 && len(skill.AutoTouched) == 0 {
			continue
		}
		if len(skill.AutoFiles) > 0 && !whenHolds(root, skill.AutoFiles) {
			continue
		}
		if len(skill.AutoTouched) > 0 && !touchedAny(skill.AutoTouched, touched) {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func touchedAny(patterns, touched []string) bool {
	for _, path := range touched {
		for _, pattern := range patterns {
			ok, err := matchPath(pattern, path)
			if err == nil && ok {
				return true
			}
		}
	}
	return false
}

func matchPath(pattern, path string) (bool, error) {
	pattern = strings.ReplaceAll(pattern, "**", "*")
	return filepath.Match(pattern, filepath.Base(path))
}
