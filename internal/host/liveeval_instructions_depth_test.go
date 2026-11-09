//go:build integration && liveeval

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func backlogFile(items int, edits map[int]string) string {
	var b strings.Builder
	for i := 1; i <= items; i++ {
		title := fmt.Sprintf("ITEM-%02d routine upkeep number %d", i, i)
		if edit, ok := edits[i]; ok {
			title = edit
		}
		priority := "low"
		if i == 7 {
			priority = "high"
		}
		fmt.Fprintf(&b, "- B-%04d task %s \u2014 %s \u2192 docs/item-%02d.md\n", i, priority, title, i)
	}
	return b.String()
}

// TestLiveEvalPinnedBacklogSurvivesCdAndIsUsed has the model pin a BACKLOG.md and then change its
// working directory; the pointers must stay delivered once and answer a question without a read.
func TestLiveEvalPinnedBacklogSurvivesCdAndIsUsed(t *testing.T) {
	requireLiveKey(t)
	root, work := t.TempDir(), t.TempDir()
	files := map[string]string{
		".git/HEAD":         "ref: refs/heads/main\n",
		"AGENTS.md":         "Keep answers short.\n",
		"docs/BACKLOG.md":   backlogFile(12, nil),
		"other/place/x.txt": "x\n",
	}
	writeFiles(t, work, files)
	tap := startProviderTap(t, root)
	s := startEvalSession(t, root, work, "auto", "--tools", "read,write,cd")
	none := func(map[string]any) string { return "" }

	r1 := s.turn("t1", "Use the write tool to write the single line `pin docs/BACKLOG.md` to the path session://self/instructions/ctl, then confirm in one sentence.", none)
	if !r1.succeeded("write", "instructions/ctl") {
		t.Fatalf("the model did not pin through the control file: %+v", r1.calls)
	}
	r2 := s.turn("t2", "Use the cd tool to move into other/place. Without reading any file, tell me the ID of the one high-priority backlog item.", none)
	if !strings.Contains(r2.text, "B-0007") {
		t.Errorf("the pinned BACKLOG was not usable after cd: %q", r2.text)
	}
	for _, call := range r2.calls {
		if call.name == "read" && strings.Contains(call.args, "BACKLOG") {
			t.Errorf("the model read the BACKLOG instead of using the pinned pointers: %s", call.args)
		}
	}
	last := len(tap.requests()) - 1
	if got := tap.count(last, "ITEM-07"); got != 1 {
		t.Errorf("the pinned item appears %d times in the last request, want once", got)
	}
	s.save("instructions-pin-live.jsonl")
	tap.save(t, "instructions-pin-live-requests.txt")
	t.Logf("requests=%d in=%d out=%d", len(tap.requests()), s.input, s.output)
}

// TestLiveEvalCappedBacklogUpdatesAsADiffAndIsReadable changes one item inside the delivered cap and
// one beyond it; the model must see a diff, never the whole view again, and answer from the update.
func TestLiveEvalCappedBacklogUpdatesAsADiffAndIsReadable(t *testing.T) {
	requireLiveKey(t)
	root, work := t.TempDir(), t.TempDir()
	writeFiles(t, work, map[string]string{
		".git/HEAD":   "ref: refs/heads/main\n",
		"AGENTS.md":   "Keep answers short.\n",
		"BACKLOG.md":  backlogFile(40, nil),
		"notes/n.txt": "n\n",
	})
	tap := startProviderTap(t, root)
	s := startEvalSession(t, root, work, "auto", "--tools", "read")
	none := func(map[string]any) string { return "" }

	r1 := s.turn("t1", "How many backlog items are there in total, and how many of them do you see listed in full? Answer with two numbers.", none)
	first := tap.requests()[0]
	if got := tap.count(0, "ITEM-10 "); got != 1 {
		t.Errorf("an item inside the cap appears %d times in the first request, want once", got)
	}
	if strings.Contains(first, "ITEM-40 ") {
		t.Errorf("an item beyond the cap was delivered in full")
	}
	if !strings.Contains(r1.text, "40") {
		t.Errorf("the model did not report the true total of 40: %q", r1.text)
	}

	edited := backlogFile(40, map[int]string{3: "CHANGED-THREE urgent rewrite", 38: "CHANGED-THIRTY-EIGHT beyond the cap"})
	if err := os.WriteFile(filepath.Join(work, "BACKLOG.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	r2 := s.turn("t2", "What does backlog item B-0003 say now? Quote its title.", none)
	last := len(tap.requests()) - 1
	body := tap.requests()[last]
	if !strings.Contains(body, "Instruction update for") {
		t.Errorf("the change did not arrive as an instruction update")
	}
	if got := tap.count(last, "CHANGED-THREE"); got < 1 {
		t.Errorf("the changed item never reached the model")
	}
	if got := tap.count(last, "ITEM-10 "); got != 1 {
		t.Errorf("an unchanged item appears %d times after the update, want once (the update must be a diff)", got)
	}
	if !strings.Contains(r2.text, "CHANGED-THREE") {
		t.Errorf("the model did not answer from the update: %q", r2.text)
	}
	s.save("instructions-cap-live.jsonl")
	tap.save(t, "instructions-cap-live-requests.txt")
	t.Logf("requests=%d in=%d out=%d", len(tap.requests()), s.input, s.output)
}

// TestLiveEvalCompactionKeepsInstructionsAndFramesOnce compacts a real session and checks that the
// rules come back as one full view, are still obeyed, and nothing is delivered twice.
func TestLiveEvalCompactionKeepsInstructionsAndFramesOnce(t *testing.T) {
	requireLiveKey(t)
	root, work := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agent", "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, work, map[string]string{
		".git/HEAD":         "ref: refs/heads/main\n",
		"AGENTS.md":         "Every reply you write must end with the exact token ROOT-RULE-7.\n",
		"pkg/sub/AGENTS.md": "Every file you create under pkg/sub must begin with the exact first line `// SUB-HEADER-9`.\n",
		"pkg/sub/old.go":    "package sub\n",
		"BACKLOG.md":        backlogFile(5, nil),
	})
	tap := startProviderTap(t, root)
	s := startEvalSession(t, root, work, "auto", "--tools", "read,write")
	none := func(map[string]any) string { return "" }
	const rootRule, subRule = "must end with the exact token ROOT-RULE-", "must begin with the exact first line"

	s.turn("t1", "Read pkg/sub/old.go and tell me its package name.", none)
	s.turn("t2", "Say hello in one short sentence.", none)
	before := len(tap.requests())

	s.f.sendRaw("compact", "compact", nil)
	s.f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "compact" })

	r3 := s.turn("t3", "Create pkg/sub/new.go containing just `package sub`. Use the write tool, then confirm in one sentence.", none)
	requests := tap.requests()
	last := len(requests) - 1
	if last < before {
		t.Fatal("no provider request after compaction")
	}
	for _, source := range []string{"session.date", "session.environment", "session.project", "session.location", "session.model", "session.capabilities"} {
		if got := tap.count(last, `src="`+source+`"`) + tap.count(last, `src=\"`+source+`\"`); got != 1 {
			t.Errorf("%s appears %d times after compaction, want once", source, got)
		}
	}
	if got := tap.count(last, rootRule+"7"); got < 1 {
		t.Errorf("the root rule did not come back after compaction")
	} else {
		t.Logf("root rule occurrences after compaction (summary may quote it): %d", got)
	}
	if got := strings.Count(requests[last], "Instruction update for"); got != 0 {
		t.Errorf("compaction produced %d update frames; instructions must return as full views", got)
	}
	data, err := os.ReadFile(filepath.Join(work, "pkg/sub/new.go"))
	if err != nil {
		t.Fatalf("the model did not create the file after compaction: %v", err)
	}
	if !strings.HasPrefix(string(data), "// SUB-HEADER-9\n") {
		t.Errorf("the nested rule was not obeyed after compaction; file begins %q", firstLine(string(data)))
	}
	if !strings.Contains(r3.text, "ROOT-RULE-7") {
		t.Errorf("the root rule was not obeyed after compaction: %q", r3.text)
	}
	s.save("instructions-compaction-live.jsonl")
	tap.save(t, "instructions-compaction-live-requests.txt")
	t.Logf("requests=%d in=%d out=%d", len(tap.requests()), s.input, s.output)
}
