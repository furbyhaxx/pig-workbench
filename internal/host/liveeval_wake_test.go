//go:build integration && liveeval

package host

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// unpromptedTurn waits for the agent to start a turn nobody asked for and returns what it said.
// It reports false when no turn starts within the window.
func (s *evalSession) unpromptedTurn(within time.Duration) (string, bool) {
	s.t.Helper()
	deadline := time.After(within)
	started := false
	var text strings.Builder
	for {
		select {
		case record := <-s.f.records:
			s.keep(record)
			switch record["type"] {
			case "extension_error":
				s.t.Fatalf("extension error: %+v", record)
			case "agent_start":
				started = true
			case "message_end":
				if message, ok := record["message"].(map[string]any); ok && message["role"] == "assistant" {
					text.Reset()
					if content, ok := message["content"].([]any); ok {
						for _, block := range content {
							if b, ok := block.(map[string]any); ok && b["type"] == "text" {
								text.WriteString(fmt.Sprint(b["text"]))
							}
						}
					}
				}
			case "agent_settled":
				if started {
					return text.String(), true
				}
			}
		case <-deadline:
			return text.String(), started
		case <-s.f.done:
			s.t.Fatalf("pig exited early: %v stderr: %s", s.f.exitErr, s.f.stderr.String())
		}
	}
}

func TestLiveEvalFinishedJobWakesTheModelAndAQuietFrameDoesNot(t *testing.T) {
	requireLiveKey(t)
	t.Setenv("WORKBENCH_WAKE_FIXTURE", "1")
	root, work := t.TempDir(), t.TempDir()
	repo := repoRoot(t)
	s := startEvalSession(t, root, work, "auto", "-e", filepath.Join(repo, "internal/host/testdata/reference-producer"), "--tools", "read,bash,jobs")

	first := s.turn("t1", "Run this shell command as a background job with the bash tool (background true): `sleep 4; echo WAKE-LIVE-DONE`. Then reply with the single word started and stop. Do not wait for it and do not poll it.", func(map[string]any) string { return "" })
	if !first.succeeded("bash", "background") && !first.succeeded("bash", "yield") {
		t.Fatalf("the model did not start a background job: %v", first.tools)
	}
	toolsBefore := len(first.tools)

	text, woke := s.unpromptedTurn(60 * time.Second)
	if !woke {
		t.Fatal("the finished job did not start a turn")
	}
	t.Logf("unprompted turn said: %.300s", text)
	if !strings.Contains(strings.ToLower(text), "finish") && !strings.Contains(text, "WAKE-LIVE-DONE") && !strings.Contains(strings.ToLower(text), "complete") && !strings.Contains(strings.ToLower(text), "done") {
		t.Errorf("the woken turn did not act on the notice: %q", text)
	}
	_ = toolsBefore

	// A frame published with wake=false must not start a turn.
	s.f.send("quiet", "prompt", "/reference wake quiet-1 false")
	if _, started := s.unpromptedTurn(12 * time.Second); started {
		t.Fatal("a wake=false frame started a turn")
	}
	second := s.turn("t2", "What did the latest alert say? Quote its body text exactly.", func(map[string]any) string { return "" })
	if !strings.Contains(second.text, "ALERT-BODY-quiet-1") {
		t.Errorf("the quiet frame did not reach the model with the next prompt: %q", second.text)
	}

	// A frame published with wake=true starts a turn on the idle session.
	s.f.send("loud", "prompt", "/reference wake loud-1 true")
	loud, woke := s.unpromptedTurn(60 * time.Second)
	if !woke {
		t.Fatal("a wake=true frame did not start a turn")
	}
	t.Logf("wake=true turn said: %.300s", loud)
	s.save("wake-live.jsonl")
	t.Logf("tokens in=%d out=%d cacheRead=%d", s.input, s.output, s.cacheRead)
}
