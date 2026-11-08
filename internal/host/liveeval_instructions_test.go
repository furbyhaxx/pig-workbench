//go:build integration && liveeval

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveEvalInstructionsInjectOnceDiffAndAreObeyed drives a real model through root and nested
// AGENTS.md delivery, an external edit and a BACKLOG.md, counting each rule per provider request.
func TestLiveEvalInstructionsInjectOnceDiffAndAreObeyed(t *testing.T) {
	requireLiveKey(t)
	root, work := t.TempDir(), t.TempDir()
	var filler strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&filler, "Style note %d: keep functions short and name things for what they hold.\n", i)
	}
	writeFiles(t, work, map[string]string{
		".git/HEAD":            "ref: refs/heads/main\n",
		"AGENTS.md":            "Every reply you write must end with the exact token ROOT-RULE-7.\n" + filler.String(),
		"pkg/sub/AGENTS.md":    "Every file you create under pkg/sub must begin with the exact first line `// SUB-HEADER-9`.\n",
		"pkg/sub/old.go":       "package sub\n",
		"BACKLOG.md":           "- B-0001 task high \u2014 BACKLOG-ITEM-ONE rewrite the ledger \u2192 docs/one.md\n- B-0002 task low \u2014 BACKLOG-ITEM-TWO tidy the docs \u2192 docs/two.md\n",
		"docs/one.md":          "# one\n",
		"docs/two.md":          "# two\n",
		"unrelated/readme.txt": "nothing here\n",
	})
	tap := startProviderTap(t, root)
	s := startEvalSession(t, root, work, "auto", "--tools", "read,write,edit")
	none := func(map[string]any) string { return "" }
	// The rules are counted by their own sentence: the model echoes the tokens in its replies and calls.
	const rootRule, subRule = "must end with the exact token ROOT-RULE-", "must begin with the exact first line"
	counts := func(marker string) []int {
		var out []int
		for i := range tap.requests() {
			out = append(out, tap.count(i, marker))
		}
		return out
	}

	r1 := s.turn("t1", "Say hello in one short sentence.", none)
	if got := counts(rootRule + "7"); len(got) == 0 || got[0] != 1 {
		t.Errorf("the root rule appears %v times in the first request, want exactly once (a host duplicate would show 2)", got)
	}
	if !strings.Contains(r1.text, "ROOT-RULE-7") {
		t.Errorf("the model did not obey the root rule: %q", r1.text)
	}
	if got := counts(subRule); got[0] != 0 {
		t.Errorf("the nested rule arrived before anything under pkg/sub was touched: %v", got)
	}

	s.turn("t2", "Read pkg/sub/old.go and tell me its package name.", none)
	after := counts(subRule)
	if after[len(after)-1] != 1 {
		t.Errorf("the nested rule must reach the model once after the touch: %v", after)
	}
	for i, n := range counts(rootRule + "7") {
		if n != 1 {
			t.Errorf("request %d carries the root rule %d times, want once", i, n)
		}
	}

	s.turn("t3", "Create pkg/sub/new.go containing just `package sub`. Use the write tool. Then confirm in one sentence.", none)
	data, err := os.ReadFile(filepath.Join(work, "pkg/sub/new.go"))
	if err != nil {
		t.Fatalf("the model did not create the file: %v", err)
	}
	if !strings.HasPrefix(string(data), "// SUB-HEADER-9\n") {
		t.Errorf("the nested rule was not obeyed; file begins %q", firstLine(string(data)))
	}
	for i, n := range counts(subRule) {
		if n > 1 {
			t.Errorf("request %d carries the nested rule %d times, want at most once", i, n)
		}
	}

	// An external edit must reach the model as an update, once, and be obeyed.
	before := len(tap.requests())
	if err := os.WriteFile(filepath.Join(work, "AGENTS.md"), []byte("Every reply you write must end with the exact token ROOT-RULE-8.\n"+filler.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	r4 := s.turn("t4", "Say hello again in one short sentence.", none)
	last := len(tap.requests()) - 1
	if last < before {
		t.Fatal("no request after the edit")
	}
	if got := tap.count(last, rootRule+"8"); got != 1 {
		t.Errorf("the changed rule appears %d times after the edit, want exactly once", got)
	}
	if !strings.Contains(tap.requests()[last], "Instruction update for") {
		t.Errorf("the change did not arrive as an instruction update")
	}
	if !strings.Contains(r4.text, "ROOT-RULE-8") || strings.Contains(r4.text, "ROOT-RULE-7") {
		t.Errorf("the model did not follow the updated rule: %q", r4.text)
	}

	r5 := s.turn("t5", "What are the backlog items? List their IDs and titles.", none)
	if !strings.Contains(r5.text, "B-0001") || !strings.Contains(r5.text, "B-0002") {
		t.Errorf("the model did not use the delivered BACKLOG pointers: %q", r5.text)
	}
	s.save("instructions-live.jsonl")
	tap.save(t, "instructions-live-requests.txt")
	t.Logf("requests=%d in=%d out=%d cacheRead=%d", len(tap.requests()), s.input, s.output, s.cacheRead)
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
