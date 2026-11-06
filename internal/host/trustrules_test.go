package host

import "testing"

func TestDecideTrustNearestRuleWinsAndDenyBeatsAllowAtEqualDepth(t *testing.T) {
	rules := func(texts ...string) []trustRule {
		var out []trustRule
		for _, text := range texts {
			rule, err := parseTrustRule(text)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, rule)
		}
		return out
	}
	cases := []struct {
		name         string
		rules        []trustRule
		dir          string
		allowed, hit bool
	}{
		{"no rule is not an allow", rules(), "/work/app/vendor/lib", false, false},
		{"allow covers descendants", rules("allow /work/app"), "/work/app/vendor/lib", true, true},
		{"sibling prefix is not a descendant", rules("allow /work/app"), "/work/application", false, false},
		{"nearest deny overrides a broader allow", rules("allow /work", "deny /work/app/vendor"), "/work/app/vendor/lib", false, true},
		{"nearest allow overrides a broader deny", rules("deny /work", "allow /work/app"), "/work/app/x", true, true},
		{"deny beats allow at equal depth", rules("allow /work/app", "deny /work/app"), "/work/app/x", false, true},
	}
	for _, c := range cases {
		allowed, matched := decideTrust(c.rules, c.dir)
		if allowed != c.allowed || matched != c.hit {
			t.Errorf("%s: got allowed=%v matched=%v", c.name, allowed, matched)
		}
	}
	for _, bad := range []string{"", "allow", "permit /x", "allow relative/dir"} {
		if _, err := parseTrustRule(bad); err == nil {
			t.Errorf("rule %q must be rejected", bad)
		}
	}
}
