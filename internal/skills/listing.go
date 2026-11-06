package skills

import (
	"path/filepath"
	"strings"
)

// Listed is one prompt line after the budget ladder.
type Listed struct {
	Name        string
	Description string
	Location    string
}

// List applies the listing ladder: bundled descriptions go first, then other
// descriptions shrink to 80 characters, then entries become names only.
// Every skill remains listed by name.
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
		if len(out[i].Description) > 80 {
			out[i].Description = out[i].Description[:80]
		}
	}
	if bytesOf(out) <= budget {
		return out
	}
	for i := range out {
		out[i].Description = ""
	}
	return out
}

func bytesOf(items []Listed) int {
	n := 0
	for _, item := range items {
		n += len(item.Name) + len(item.Description) + len(item.Location) + 2
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
