package host

import "testing"

func TestFrameSourceVisibilityUsesTheLongestMatchingPrefix(t *testing.T) {
	rules := []string{"hidden session", "shown session.date", "bogus session.model", "hidden instructions.host"}
	for source, want := range map[string]string{
		"session.date":           "shown",
		"session.model":          "hidden",
		"session":                "hidden",
		"sessions.message":       "collapsed",
		"instructions.host":      "hidden",
		"instructions.steering":  "collapsed",
		"reference.research.run": "collapsed",
	} {
		if got := frameSourceVisibility(rules, source, "collapsed"); got != want {
			t.Errorf("%s: got %s, want %s", source, got, want)
		}
	}
}
