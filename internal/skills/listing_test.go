package skills

import (
	"fmt"
	"strings"
	"testing"
)

func listingBytes(listed []Listed) int {
	n := 0
	for _, item := range listed {
		line := "- " + item.Location
		if item.Description != "" {
			line += ": " + item.Description
		}
		n += len(line) + 1
	}
	return n
}

func TestListDropsBundledDescriptionsFirst(t *testing.T) {
	items := []Skill{
		{Name: "bundled", Description: "long bundled description", Bundled: true},
		{Name: "other", Description: "keep"},
	}
	listed := List(items, len("- skill://bundled\n- skill://other: keep\n"))
	if listed[0].Description != "" || listed[1].Description != "keep" {
		t.Fatalf("ladder = %+v", listed)
	}
}

func TestListLadderKeepsFortySkillsNamedWithinTheBudget(t *testing.T) {
	var items []Skill
	for i := range 40 {
		items = append(items, Skill{Name: fmt.Sprintf("skill-%02d", i), Description: strings.Repeat("routing wörds ", 30), Bundled: i >= 36})
	}
	full := List(items, 0)
	if listingBytes(full) < 16000 || full[39].Description == "" {
		t.Fatalf("budget 0 must leave the listing whole, got %d bytes", listingBytes(full))
	}
	// Rung 1 is not enough, rung 2 is: 36 descriptions of 80 characters fit 4096 bytes.
	cut := List(items, 4096)
	if got := listingBytes(cut); got > 4096 {
		t.Fatalf("listing is %d bytes over a 4096 budget", got)
	}
	if cut[36].Description != "" || cut[39].Description != "" {
		t.Fatal("bundled skills keep their descriptions while others were cut")
	}
	for i, item := range cut[:36] {
		if n := len([]rune(item.Description)); n != 80 || !strings.HasSuffix(item.Description, "…") || !strings.HasPrefix(items[i].Description, strings.TrimSuffix(item.Description, "…")) {
			t.Fatalf("skill %d description is not an 80-character cut: %d %q", i, n, item.Description)
		}
	}
	// Rung 3: the lowest-ranked entries lose their description first; the leading ones keep theirs.
	tight := List(items, 2048)
	if got := listingBytes(tight); got > 2048 {
		t.Fatalf("listing is %d bytes over a 2048 budget", got)
	}
	if tight[0].Description == "" || tight[35].Description != "" {
		t.Fatalf("names-only must start at the last-ranked entry: first %q, last %q", tight[0].Description, tight[35].Description)
	}
	for i := 1; i < len(tight); i++ {
		if tight[i].Description != "" && tight[i-1].Description == "" {
			t.Fatalf("entry %d keeps a description after a higher-ranked entry lost its own", i)
		}
	}
	// A budget below the names: every skill is still listed by name.
	names := List(items, 10)
	for i, item := range names {
		if item.Location != "skill://"+items[i].Name || item.Description != "" {
			t.Fatalf("entry %d = %+v", i, item)
		}
	}
}
