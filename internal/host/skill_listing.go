package host

import (
	"fmt"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/skills"
)

const (
	skillsListingIntro = "Skills available in this session. Read skill://<name> to load one when the task matches its description; write unload to skill://<name>/ctl to stop applying it."
	skillsChangeIntro  = "Skill availability changed; every skill not named here is as before."
	skillsNowHeading   = "Available now:"
	skillsGoneHeading  = "No longer available:"
)

// skillsListing is the body of the session.skills frame: one line per available skill, or "" for none.
func skillsListing(listed []skills.Listed) string {
	if len(listed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(skillsListingIntro)
	for _, skill := range listed {
		b.WriteString("\n- " + skill.Location)
		if skill.Description != "" {
			b.WriteString(": " + skill.Description)
		}
	}
	return b.String()
}

// skillLines maps each skill location of a listing or change body to its line, in order.
func skillLines(body string) ([]string, map[string]string) {
	var order []string
	lines := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "- skill://") {
			continue
		}
		location, _, _ := strings.Cut(strings.TrimPrefix(line, "- "), ": ")
		order = append(order, location)
		lines[location] = line
	}
	return order, lines
}

// skillsChange names only the skills that became available and those that no longer are, against the
// listing the model holds. It returns "" when the same skills are available.
func skillsChange(previous, current string) string {
	before, _ := skillLines(previous)
	after, lines := skillLines(current)
	held := map[string]bool{}
	for _, location := range before {
		held[location] = true
	}
	var b strings.Builder
	var added []string
	for _, location := range after {
		if !held[location] {
			added = append(added, lines[location])
		}
		delete(held, location)
	}
	if len(added) == 0 && len(held) == 0 {
		return ""
	}
	b.WriteString(skillsChangeIntro)
	if len(added) > 0 {
		b.WriteString("\n" + skillsNowHeading + "\n" + strings.Join(added, "\n"))
	}
	if len(held) > 0 {
		b.WriteString("\n" + skillsGoneHeading)
		for _, location := range before {
			if held[location] {
				b.WriteString("\n- " + location)
			}
		}
	}
	return b.String()
}

// skillsRowSummary is the transcript fact of a session.skills frame: a count for a listing, the names for a change.
func skillsRowSummary(first string, rest []string) string {
	if first != skillsChangeIntro {
		order, _ := skillLines(strings.Join(rest, "\n"))
		if len(order) == 1 {
			return "1 skill"
		}
		return fmt.Sprintf("%d skills", len(order))
	}
	var parts []string
	sign := "+"
	for _, line := range rest {
		switch {
		case line == skillsNowHeading:
			sign = "+"
		case line == skillsGoneHeading:
			sign = "-"
		case strings.HasPrefix(line, "- skill://"):
			location, _, _ := strings.Cut(strings.TrimPrefix(line, "- skill://"), ": ")
			parts = append(parts, sign+location)
		}
	}
	return strings.Join(parts, ", ")
}
