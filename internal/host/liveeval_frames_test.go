//go:build integration && liveeval

package host

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveEvalFramesInjectOnceAndDeduplicate runs a real model through several turns, a duplicate
// publish, a restart on the same session and a fork, counting each frame's text per provider request.
func TestLiveEvalFramesInjectOnceAndDeduplicate(t *testing.T) {
	requireLiveKey(t)
	t.Setenv("WORKBENCH_WAKE_FIXTURE", "1")
	root, work := t.TempDir(), t.TempDir()
	tap := startProviderTap(t, root)
	producer := filepath.Join(repoRoot(t), "internal/host/testdata/reference-producer")
	s := startEvalSession(t, root, work, "auto", "-e", producer, "--tools", "read")
	none := func(map[string]any) string { return "" }

	s.turn("t1", "Reply with the single word ready.", none)
	s.turn("t2", "Reply with the single word two.", none)
	s.turn("t3", "Reply with the single word three.", none)

	const notice = "REFERENCE-NOTICE: research run 7 finished"
	requests := tap.requests()
	if len(requests) < 3 {
		t.Fatalf("expected at least 3 provider requests, have %d", len(requests))
	}
	for i := range requests {
		if got := tap.count(i, notice); got != 1 {
			t.Errorf("request %d carries the startup notice %d times, want exactly once", i, got)
		}
	}

	s.f.send("a1", "prompt", "/reference wake quiet-1 false")
	s.turn("t4", "Reply with the single word four.", none)
	s.f.send("a2", "prompt", "/reference wake quiet-1 false")
	s.f.send("a2b", "prompt", "/reference wake abandoned-1 false")
	s.turn("t5", "Reply with the single word five.", none)
	all := tap.requests()
	for i := len(requests); i < len(all); i++ {
		if got := tap.count(i, "ALERT-BODY-quiet-1"); got != 1 {
			t.Errorf("request %d carries the alert %d times after a duplicate publish, want exactly once", i, got)
		}
		if got := tap.count(i, notice); got != 1 {
			t.Errorf("request %d carries the startup notice %d times after a later frame, want once", i, got)
		}
	}

	// Restart on the same session: history replays, the alert's stable key stays deduplicated.
	s.f.close()
	resumed := startEvalSession(t, root, work, "auto", "-e", producer, "--tools", "read", "--continue")
	resumed.f.send("a3", "prompt", "/reference wake quiet-1 false")
	resumed.turn("t6", "Reply with the single word six.", none)
	afterResume := tap.requests()
	last := len(afterResume) - 1
	if got := tap.count(last, "ALERT-BODY-quiet-1"); got != 1 {
		t.Errorf("the resumed session's request carries the alert %d times, want exactly once", got)
	}
	if got := tap.count(last, "Reply with the single word four."); got != 1 {
		t.Errorf("the resumed session replays its history %d times, want once", got)
	}
	t.Logf("startup notices after resume (a new generation publishes its own keyed notice): %d", tap.count(last, notice))

	// Fork before the user message that delivered the alert: the other branch's frame must not leak.
	resumed.f.sendRaw("gf", "get_fork_messages", nil)
	var forkPoint string
	forks := resumed.f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "gf" })
	data, _ := json.Marshal(forks["data"])
	var listed struct {
		Messages []struct {
			EntryID string `json:"entryId"`
			Text    string `json:"text"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(data, &listed); err != nil {
		t.Fatalf("fork list: %v: %s", err, data)
	}
	for _, m := range listed.Messages {
		if strings.Contains(m.Text, "single word four") {
			forkPoint = m.EntryID
		}
	}
	if forkPoint == "" {
		t.Fatalf("no fork point for the fourth prompt in %s", data)
	}
	resumed.f.sendRaw("fork", "fork", map[string]any{"entryId": forkPoint})
	resumed.f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "fork" })
	resumed.turn("t7", "Reply with the single word seven.", none)
	forked := tap.requests()
	end := len(forked) - 1
	if got := tap.count(end, "ALERT-BODY-abandoned-1"); got != 0 {
		t.Errorf("the fork carries a frame that was delivered only on the abandoned branch (%d): it leaked", got)
	}
	if got := tap.count(end, "ALERT-BODY-quiet-1"); got != 1 {
		t.Errorf("the fork's own ancestor alert appears %d times, want exactly once", got)
	}
	if got := tap.count(end, "single word five"); got != 0 {
		t.Errorf("the forked branch still holds the other branch's later prompt (%d)", got)
	}
	if got := tap.count(end, "Reply with the single word three."); got != 1 {
		t.Errorf("the forked branch lost or repeated its own history (%d)", got)
	}
	resumed.f.send("a4", "prompt", "/reference wake abandoned-1 false")
	resumed.turn("t8", "Reply with the single word eight.", none)
	again := tap.requests()
	t.Logf("alert delivered only on the abandoned branch, republished on the fork: appears %d times in the next request", tap.count(len(again)-1, "ALERT-BODY-abandoned-1"))
	s.save("frames-live.jsonl")
	tap.save(t, "frames-live-requests.txt")
	t.Logf("requests=%d tokens in=%d out=%d cacheRead=%d", len(forked), s.input+resumed.input, s.output+resumed.output, s.cacheRead+resumed.cacheRead)
}
