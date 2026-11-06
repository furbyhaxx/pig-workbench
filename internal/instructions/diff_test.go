package instructions

import (
	"strings"
	"testing"
)

func TestUnifiedDiffAppliesByteForByte(t *testing.T) {
	long := func(n int) []string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = "line " + strings.Repeat("x", i%7) + string(rune('a'+i%26))
		}
		return lines
	}
	base := long(60)
	cases := map[string][]string{
		"edit near the top":      append([]string{"changed"}, base[1:]...),
		"edit in the middle":     append(append(append([]string{}, base[:30]...), "inserted", "twice"), base[31:]...),
		"two distant edits":      append(append(append(append(append([]string{}, base[:5]...), "A"), base[6:50]...), "B"), base[51:]...),
		"deleted tail":           base[:40],
		"appended lines":         append(append([]string{}, base...), "new tail"),
		"everything replaced":    {"completely", "different"},
		"empty to content":       {"x"},
		"trailing newline added": append(append([]string{}, base...), ""),
	}
	for name, next := range cases {
		for _, context := range []int{0, 1, 3, 10} {
			old, new := strings.Join(base, "\n"), strings.Join(next, "\n")
			diff := UnifiedDiff(old, new, context)
			got, err := ApplyDiff(old, diff)
			if err != nil || got != new {
				t.Fatalf("%s context %d: apply = %v, equal=%v\n%s", name, context, err, got == new, diff)
			}
		}
	}
	if UnifiedDiff("same\n", "same\n", 3) != "" {
		t.Fatal("equal texts must produce no diff")
	}
}

func TestApplyDiffRefusesAWrongBase(t *testing.T) {
	diff := UnifiedDiff("one\ntwo\nthree", "one\n2\nthree", 1)
	if _, err := ApplyDiff("one\nTWO\nthree", diff); err == nil {
		t.Fatal("a diff must not apply to a base that differs from the delivered view")
	}
}
