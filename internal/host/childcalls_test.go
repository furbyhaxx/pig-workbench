package host

import "testing"

func TestChildCallsAreBoundedAndOnlyTrustTheDeclaredParentPrefix(t *testing.T) {
	c := newChildCalls()
	c.observe("p", "other/1", "read", "running")
	for i := 0; i < maxChildrenEach+3; i++ {
		c.observe("p", "p/"+string(rune('a'+i%26))+string(rune('a'+i/26)), "read", "running")
	}
	c.observe("p", "p/aa", "read", "error")
	c.observe("p", "p/aa", "read", "ok")
	got := c.snapshot("p")
	if len(got) != 1 || len(got[0].Children) != maxChildrenEach || got[0].Omitted != 3 || got[0].Children[0].Status != "error" {
		t.Fatalf("bounds or first-outcome rule lost: %+v", got)
	}
	for i := 0; i < maxChildParents+2; i++ {
		id := string(rune('A'+i%26)) + string(rune('a'+i/26))
		c.observe(id, id+"/1", "read", "ok")
	}
	if len(c.snapshot("")) > maxChildParents {
		t.Fatal("parent records are unbounded")
	}
}
