package skills

import "testing"

func TestListDropsBundledDescriptionsFirst(t *testing.T) {
	items := []Skill{
		{Name: "bundled", Description: "long bundled description", Bundled: true},
		{Name: "other", Description: "keep"},
	}
	listed := List(items, len("bundled")+len("skill://bundled")+len("other")+len("keep")+len("skill://other")+4)
	if listed[0].Description != "" || listed[1].Description != "keep" {
		t.Fatalf("ladder = %+v", listed)
	}
}
