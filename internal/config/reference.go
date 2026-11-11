package config

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Reference renders every catalog setting as a Markdown table, sorted by key.
func Reference() string {
	definitions := Catalog()
	slices.SortFunc(definitions, func(a, b Definition) int { return strings.Compare(a.Key, b.Key) })
	var out strings.Builder
	out.WriteString("| Key | Type | Default | Project file | Description |\n|---|---|---|---|---|\n")
	for _, d := range definitions {
		kind := string(d.Kind)
		switch {
		case len(d.Choices) > 0:
			kind = "`" + strings.Join(d.Choices, "`, `") + "`"
		case d.Kind == Integer:
			kind = fmt.Sprintf("integer %d–%d", d.Min, d.Max)
		}
		value, _ := json.Marshal(d.Default)
		description := strings.ReplaceAll(d.Description, "|", "\\|")
		if d.Application != "live" {
			description = strings.TrimSpace("Not applied yet. " + description)
		}
		fmt.Fprintf(&out, "| `%s` | %s | `%s` | %s | %s |\n", d.Key, kind, value, projectWords[d.Project], description)
	}
	return out.String()
}

var projectWords = map[ProjectPolicy]string{
	DenyProject:        "no",
	AllowProject:       "yes",
	ReduceProject:      "may only lower",
	RequireTrueProject: "may only set true",
}
