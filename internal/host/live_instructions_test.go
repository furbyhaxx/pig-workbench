//go:build integration

package host

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/project"
)

// toolResults returns every tool message of one recorded provider request, in order.
func toolResults(t *testing.T, body string) []string {
	t.Helper()
	var request struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &request); err != nil {
		t.Fatal(err)
	}
	var results []string
	for _, message := range request.Messages {
		if message["role"] == "tool" {
			results = append(results, toolMessageText(message))
		}
	}
	return results
}

func TestLiveProjectPrivateNotesRespectTrustAndProjectIdentity(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		t.Run(fmt.Sprintf("trusted=%t", trusted), func(t *testing.T) {
			root := t.TempDir()
			if _, err := project.Create(root, "prj-privateone", "private-one"); err != nil {
				t.Fatal(err)
			}
			agent := filepath.Join(root, "agent")
			private := filepath.Join(agent, "projects", "prj-privateone", "private")
			if err := os.MkdirAll(private, 0700); err != nil {
				t.Fatal(err)
			}
			note := filepath.Join(private, "notes.md")
			if err := os.WriteFile(note, []byte("PRIVATE-ONE-MARKER\n"), 0600); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(agent, "projects", "prj-privateother", "private", "BACKLOG.md")
			if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(other, []byte("OTHER-PROJECT-SECRET\n"), 0600); err != nil {
				t.Fatal(err)
			}
			recorder := &requestRecorder{calls: []scriptedCall{
				{"write", map[string]any{"path": "session://self/instructions/ctl", "content": "pin " + note}},
				{"write", map[string]any{"path": "session://self/instructions/ctl", "content": "pin " + other}},
			}}
			server := recorder.serve(t)
			defer server.Close()
			writeFixtureProvider(t, agent, server.URL)
			args := []string{"--model", "workbench-fixture/mock", "--tools", "write"}
			if trusted {
				args = append(args, "--approve")
			} else {
				args = append(args, "--no-approve")
			}
			fixture := startRPCWithExtensions(t, root, true, nil, args...)
			fixture.send("go", "prompt", "Inspect private instructions.")
			fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			if len(recorder.bodies) != 3 {
				t.Fatalf("got %d provider requests", len(recorder.bodies))
			}
			first := recorder.bodies[0]
			if got := strings.Contains(first, "PRIVATE-ONE-MARKER"); got != trusted {
				t.Fatalf("private note present=%v; trusted=%v", got, trusted)
			}
			if trusted && (!strings.Contains(first, "instructions.project-private") || !strings.Contains(first, "user, instruction")) {
				t.Fatal("private note lacks its user instruction provenance")
			}
			if strings.Contains(strings.Join(recorder.bodies, ""), "OTHER-PROJECT-SECRET") {
				t.Fatal("another project's private content reached the provider")
			}
			results := toolResults(t, recorder.bodies[2])
			if len(results) < 2 || !strings.Contains(results[len(results)-1], "active trusted project") {
				t.Fatalf("cross-project pin was not refused: %v", results)
			}
			if trusted && !strings.Contains(results[len(results)-2], "pinned") {
				t.Fatalf("own private note pin was refused: %v", results)
			}
		})
	}
}

func TestLivePrivateNotesUpdateAndPinnedNotesWithdrawOnProjectChange(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	if _, err := project.Create(root, "prj-privateone", "private-one"); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Create(other, "prj-privateother", "private-other"); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(root, "agent")
	private := filepath.Join(agent, "projects", "prj-privateone", "private")
	otherPrivate := filepath.Join(agent, "projects", "prj-privateother", "private")
	for _, dir := range []string{private, otherPrivate} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	var body strings.Builder
	for i := range 40 {
		fmt.Fprintf(&body, "PRIVATE-RULE-%02d must be obeyed.\n", i)
	}
	note := filepath.Join(private, "notes.md")
	if err := os.WriteFile(note, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherPrivate, "notes.md"), []byte("OTHER-PRIVATE-MARKER\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "b.txt"), []byte("b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"write", map[string]any{"path": "session://self/instructions/ctl", "content": "pin " + note}},
		{"bash", map[string]any{"command": fmt.Sprintf("printf 'PRIVATE-UPDATED-MARKER\\n' >> %q", note)}},
		{"read", map[string]any{"path": "a.txt"}},
		{"cd", map[string]any{"path": other}},
		{"read", map[string]any{"path": "b.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--approve", "--model", "workbench-fixture/mock", "--tools", "read,write,bash,cd")
	fixture.send("go", "prompt", "Check the project instructions.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 6 {
		t.Fatalf("got %d provider requests", len(recorder.bodies))
	}
	if !strings.Contains(recorder.bodies[0], "PRIVATE-RULE-00") {
		t.Fatal("initial private note was not delivered")
	}
	if !strings.Contains(recorder.bodies[3], "Instruction update for") || !strings.Contains(recorder.bodies[3], "PRIVATE-UPDATED-MARKER") {
		t.Fatal("private note update did not arrive as an instruction diff")
	}
	last := recorder.bodies[5]
	if !strings.Contains(last, "does not belong to the active trusted project") {
		t.Fatal("pinned private note was not withdrawn after changing projects")
	}
	if strings.Contains(strings.Join(recorder.bodies, ""), "OTHER-PRIVATE-MARKER") {
		t.Fatal("untrusted other project's private note reached the provider")
	}
}

func TestLivePrivateNotesFollowExplicitlyTrustedProjectChange(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	if _, err := project.Create(root, "prj-first", "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Create(other, "prj-second", "second"); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(root, "agent")
	for id, marker := range map[string]string{"prj-first": "FIRST-NOTE-MARKER", "prj-second": "SECOND-NOTE-MARKER"} {
		dir := filepath.Join(agent, "projects", id, "private")
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(marker+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(other, "b.txt"), []byte("b\n"), 0600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"cd", map[string]any{"path": other}},
		{"read", map[string]any{"path": "b.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	settings := fmt.Sprintf("[trust]\nrules = [%q]\n", "allow "+other)
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--approve", "--model", "workbench-fixture/mock", "--tools", "cd,read")
	fixture.send("go", "prompt", "Inspect both projects.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 3 {
		t.Fatalf("got %d provider requests", len(recorder.bodies))
	}
	if !strings.Contains(recorder.bodies[0], "FIRST-NOTE-MARKER") || strings.Contains(recorder.bodies[0], "SECOND-NOTE-MARKER") {
		t.Fatal("first project did not receive only its own private note")
	}
	if !strings.Contains(recorder.bodies[2], "SECOND-NOTE-MARKER") {
		t.Fatal("explicitly trusted second project did not receive its private note")
	}
}

func TestLiveInstructionSettingsControlFramesAndPointerValidation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		enabled     bool
		validate    bool
		wantFrame   bool
		wantMissing bool
	}{
		{"enabled-validate", true, true, true, true},
		{"enabled-no-validation", true, false, true, false},
		{"disabled", false, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "BACKLOG.md"), []byte("- B-0001 task high — missing → missing.md\n"), 0600); err != nil {
				t.Fatal(err)
			}
			recorder := &requestRecorder{}
			server := recorder.serve(t)
			defer server.Close()
			agent := filepath.Join(root, "agent")
			writeFixtureProvider(t, agent, server.URL)
			settings := fmt.Sprintf("[instructions]\nenabled = %t\n[instructions.backlog]\nvalidate-pointers = %t\n", tc.enabled, tc.validate)
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte(settings), 0600); err != nil {
				t.Fatal(err)
			}
			fixture := startRPCWithExtensions(t, root, true, nil, "--approve", "--model", "workbench-fixture/mock")
			fixture.send("go", "prompt", "Test instruction delivery.")
			fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			if len(recorder.bodies) != 1 {
				t.Fatalf("got %d provider requests", len(recorder.bodies))
			}
			body := recorder.bodies[0]
			if got := strings.Contains(body, "instructions.backlog"); got != tc.wantFrame {
				t.Fatalf("backlog frame present=%v, want %v", got, tc.wantFrame)
			}
			if got := strings.Contains(body, "target missing"); got != tc.wantMissing {
				t.Fatalf("missing-target annotation present=%v, want %v", got, tc.wantMissing)
			}
		})
	}
}

func TestLiveChangedInstructionsArriveAsADiffAgainstTheDeliveredView(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	var steering strings.Builder
	for i := 1; i <= 40; i++ {
		steering.WriteString("Steering rule " + strings.Repeat("r", i%9) + string(rune('A'+i%26)) + " must hold.\n")
	}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte(steering.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a.txt"}},
		{"bash", map[string]any{"command": "sed -i 's/^Steering rule rB must hold./Rule CHANGED-BY-TEST./' AGENTS.md"}},
		{"read", map[string]any{"path": "a.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 4 {
		t.Fatalf("got %d requests, want 4", len(recorder.bodies))
	}
	if !strings.Contains(recorder.bodies[0], "Steering rule") {
		t.Fatal("the first request must carry the full steering view")
	}
	if after, _ := os.ReadFile(agents); !strings.Contains(string(after), "CHANGED-BY-TEST") {
		t.Fatal("the scripted edit did not change AGENTS.md")
	}
	last := recorder.bodies[3]
	if !strings.Contains(last, "Instruction update for") || !strings.Contains(last, `+Rule CHANGED-BY-TEST.`) {
		t.Fatalf("the change must arrive as an instruction update with a diff hunk")
	}
	// Request 0 carries the full 40-rule view once; the update adds only its hunk, never the whole view again.
	if got := strings.Count(last, "Steering rule "); got > 40+10 {
		t.Fatalf("the update repeated the view (%d rule mentions)", got)
	}
}

func TestLivePinnedBacklogStaysDeliveredAndPoliciesHold(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"AGENTS.md":                         "Steering stays unpinnable.\n",
		"a.txt":                             "a\n",
		filepath.Join("docs", "BACKLOG.md"): "- B-0001 task high — PINNED-ITEM-MARKER → docs/item.md\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctl := "session://self/instructions/ctl"
	recorder := &requestRecorder{calls: []scriptedCall{
		{"write", map[string]any{"path": ctl, "content": "pin AGENTS.md"}},
		{"write", map[string]any{"path": ctl, "content": "pin docs/BACKLOG.md"}},
		{"read", map[string]any{"path": "a.txt"}},
		{"write", map[string]any{"path": ctl, "content": "unpin docs/BACKLOG.md"}},
		{"read", map[string]any{"path": "a.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write")
	fixture.send("script", "prompt", "Pin things.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 6 {
		t.Fatalf("got %d requests, want 6", len(recorder.bodies))
	}
	results := toolResults(t, recorder.bodies[5])
	if !strings.Contains(results[0], "does not allow pins") {
		t.Fatalf("pinning steering must be refused: %q", results[0])
	}
	if !strings.Contains(results[1], "pinned ") {
		t.Fatalf("pinning a backlog must succeed: %q", results[1])
	}
	if strings.Contains(recorder.bodies[1], "PINNED-ITEM-MARKER") {
		t.Fatal("the backlog was delivered before it was pinned")
	}
	if !strings.Contains(recorder.bodies[3], "PINNED-ITEM-MARKER") {
		t.Fatal("the pinned backlog must reach the model at the next boundary even though the touched file is elsewhere")
	}
	if strings.Contains(recorder.bodies[5], "no longer applies") {
		t.Fatal("unpinning must not send the model a withdrawal frame")
	}
}

func TestLiveNewSteeringIsAdmittedAndDeletedSteeringStaysSilent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a.txt"}},
		{"bash", map[string]any{"command": "echo 'Late rule LATE-RULE-MARKER.' > AGENTS.md"}},
		{"read", map[string]any{"path": "a.txt"}},
		{"bash", map[string]any{"command": "rm AGENTS.md"}},
		{"read", map[string]any{"path": "a.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 6 {
		t.Fatalf("got %d requests, want 6", len(recorder.bodies))
	}
	if strings.Contains(recorder.bodies[1], "LATE-RULE-MARKER.") && strings.Contains(recorder.bodies[1], "Instructions from") {
		t.Fatal("the file did not exist yet")
	}
	if !strings.Contains(recorder.bodies[3], "Instructions from: ") || !strings.Contains(recorder.bodies[3], "LATE-RULE-MARKER") {
		t.Fatal("a steering file created mid-session must be admitted at the next tool boundary")
	}
	if strings.Contains(recorder.bodies[5], "no longer applies") {
		t.Fatal("a deleted steering file must not send the model a withdrawal frame")
	}
}

func TestLiveInstructionChangedAcrossCompactionArrivesAsOneFullView(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	var original strings.Builder
	for i := 1; i <= 30; i++ {
		original.WriteString("Rule number " + strings.Repeat("n", i) + " applies.\n")
	}
	if err := os.WriteFile(agents, []byte(original.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	for i, prompt := range []string{"first", "second"} {
		f.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	}
	changed := strings.Replace(original.String(), "Rule number nnnnn applies.", "Rule CHANGED-ACROSS-COMPACTION applies.", 1)
	if err := os.WriteFile(agents, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	f.sendRaw("compact", "compact", nil)
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "compact" })
	recorder.mu.Lock()
	before := len(recorder.bodies)
	recorder.mu.Unlock()
	f.send("p2", "prompt", "third")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) <= before {
		t.Fatal("no provider request after compaction")
	}
	last := recorder.bodies[len(recorder.bodies)-1]
	if strings.Contains(last, "Instruction update for") {
		t.Fatal("a diff cannot apply after compaction: its base was summarized away")
	}
	if got := strings.Count(last, "CHANGED-ACROSS-COMPACTION"); got != 1 {
		t.Fatalf("the changed view appears %d times after compaction, want exactly 1", got)
	}
	if got := strings.Count(last, "Rule number nnnnnnnnnn applies."); got != 1 {
		t.Fatalf("the unchanged rules must arrive once as part of the full view, got %d", got)
	}
}

func TestLiveBacklogShowsAMissingPointerTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"BACKLOG.md": "- B-0001 task high — POINTER-MARKER → gone.md\n",
		"a.txt":      "a\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) == 0 || !strings.Contains(recorder.bodies[0], "gone.md [target missing]") {
		t.Fatal("a backlog line whose target does not exist must be shown with a marker, not dropped")
	}
}

func TestLiveTruncatedSteeringBlocksMutationsUntilFullyRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(strings.Repeat("Steering rule.\n", 250)), 0o600); err != nil {
		t.Fatal(err)
	}
	write := map[string]any{"path": "made.txt", "content": "made\n"}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"write", write},
		{"write", write},
		{"bash", map[string]any{"command": "touch should-not-exist"}},
		{"read", map[string]any{"path": "AGENTS.md:1-125"}},
		{"write", write},
		{"read", map[string]any{"path": "AGENTS.md:126-250"}},
		{"write", write},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write,bash")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 7 || !strings.Contains(results[0], "incompleteInstruction") || !strings.Contains(results[1], "incompleteInstruction") || !strings.Contains(results[2], "incompleteInstruction") || !strings.Contains(results[4], "incompleteInstruction") || !strings.Contains(results[6], "Wrote") {
		t.Fatalf("truncated steering prerequisite was not enforced until a complete read: %q", results)
	}
	if _, err := os.Stat(filepath.Join(root, "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("blocked bash command ran: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "made.txt")); err != nil || string(data) != "made\n" {
		t.Fatalf("post-read write failed: %v %q", err, data)
	}
}

func TestLiveMutationIsBlockedOnceWhenNewInstructionsApply(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	write := map[string]any{"path": "b.txt", "content": "made\n"}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a.txt"}},
		{"bash", map[string]any{"command": "echo 'Never write before reading BARRIER-RULE.' > AGENTS.md"}},
		{"write", write},
		{"write", write},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash,write")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) < 4 || !strings.Contains(results[2], "instructionContextChanged") {
		t.Fatalf("the first write must be blocked with instructionContextChanged: %q", results)
	}
	if strings.Contains(results[3], "instructionContextChanged") {
		t.Fatalf("the retry must go through once the instructions were delivered: %q", results[3])
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "made\n" {
		t.Fatalf("the retried write did not land: %v %q", err, data)
	}
}

func TestLiveWriteIntoMissingDirectoryReportsTheDirectoryNotInstructionDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"write", map[string]any{"path": "notes-missing/guide.md", "content": "# Guide\n"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "write")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 1 || strings.Contains(results[0], "stat target") || !strings.Contains(results[0], "create it first") {
		t.Fatalf("a write into a missing directory must say so and how to fix it: %q", results)
	}
}

func TestLiveFramesAndInstructionRulesCommands(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Rules.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	notified := func(want string) func(map[string]any) bool {
		return func(record map[string]any) bool {
			return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), want)
		}
	}
	fixture.send("frames", "prompt", "/workbench frames")
	fixture.wait(notified("session.capabilities"))
	fixture.send("rules", "prompt", "/workbench instructions rules")
	fixture.wait(notified("steering  " + filepath.Join(root, "AGENTS.md")))
}

func TestLiveNestedRepositoryInstructionsLoadOnlyAfterATrustRule(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "vendor", "lib")
	for _, dir := range []string{filepath.Join(root, ".git"), filepath.Join(nested, ".git")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(nested, "AGENTS.md"), []byte("NESTED-RULE-MARKER applies here.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "x.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "vendor/lib/x.txt"}}}}
	recorder.next = func(index int) *scriptedCall {
		if index == 2 {
			return &scriptedCall{"read", map[string]any{"path": "vendor/lib/x.txt"}}
		}
		return nil
	}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("first", "prompt", "Read the vendored file.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	fixture.send("trust", "prompt", "/workbench trust add allow "+nested)
	fixture.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), "Trust rules saved")
	})
	fixture.send("second", "prompt", "Read the vendored file again.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" && len(recorder.bodies) >= 4 })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i := 0; i < 2; i++ {
		if strings.Contains(recorder.bodies[i], "NESTED-RULE-MARKER") {
			t.Fatalf("request %d carried nested instructions without a trust rule", i)
		}
	}
	if !strings.Contains(recorder.bodies[1], "Nested repository instructions were not loaded") {
		t.Fatal("the boundary notice must explain why nothing loaded")
	}
	if !strings.Contains(recorder.bodies[len(recorder.bodies)-1], "NESTED-RULE-MARKER") {
		t.Fatal("an allow rule must let the nested repository's instructions load")
	}
}

func instructionTree(t *testing.T, root string, rules map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for dir, rule := range rules {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "AGENTS.md"), []byte(rule+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, "x.txt"), []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLiveCodemodeParallelReadsDeliverTheUnionOfInstructions(t *testing.T) {
	root := t.TempDir()
	instructionTree(t, root, map[string]string{"a": "UNION-RULE-A applies.", "b": "UNION-RULE-B applies."})
	recorder := &requestRecorder{calls: []scriptedCall{
		{"codemode", map[string]any{"code": "const r = await Promise.all([tools.read({path:'a/x.txt'}), tools.read({path:'b/x.txt'})]); text(String(r.length));"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,codemode")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := recorder.bodies[len(recorder.bodies)-1]
	for _, marker := range []string{"UNION-RULE-A", "UNION-RULE-B"} {
		if got := strings.Count(last, marker); got != 1 {
			t.Errorf("%s reached the model %d times, want exactly once", marker, got)
		}
	}
}

func TestLiveCodemodeCannotHideAMutationTargetFromTheInstructionBarrier(t *testing.T) {
	root := t.TempDir()
	instructionTree(t, root, map[string]string{"c": "HIDDEN-TARGET-RULE applies."})
	write := "try { text(await tools.write({path:'c/new.txt', content:'made\\n'})); } catch (e) { text('REFUSED: ' + e.message); }"
	recorder := &requestRecorder{calls: []scriptedCall{
		{"codemode", map[string]any{"code": write}},
		{"codemode", map[string]any{"code": write}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "write,codemode")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) < 2 || !strings.Contains(results[0], "instructionContextChanged") {
		t.Fatalf("the nested write must be blocked with instructionContextChanged: %q", results)
	}
	if strings.Contains(results[1], "instructionContextChanged") {
		t.Fatalf("the retry must pass once the instructions were delivered: %q", results[1])
	}
	if data, err := os.ReadFile(filepath.Join(root, "c", "new.txt")); err != nil || string(data) != "made\n" {
		t.Fatalf("the retried write did not land: %v %q", err, data)
	}
}

// A nested rule delivered by a touch stays applicable to the next prompt: SPEC-0007 retains the most
// recent completed target set, so the next prompt neither withdraws it nor blocks a write beside it.
func TestLiveNestedInstructionsSurviveTheNextPrompt(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		".git/HEAD":         "ref: refs/heads/main\n",
		"AGENTS.md":         "Root rule.\n",
		"pkg/sub/AGENTS.md": "NESTED-RULE-MARKER applies under pkg/sub.\n",
		"pkg/sub/a.txt":     "a\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scripted := map[int]scriptedCall{
		0: {"read", map[string]any{"path": "pkg/sub/a.txt"}},
		2: {"write", map[string]any{"path": "pkg/sub/b.txt", "content": "b\n"}},
	}
	recorder := &requestRecorder{next: func(index int) *scriptedCall {
		if call, ok := scripted[index]; ok {
			return &call
		}
		return nil
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write")
	for _, id := range []string{"first", "second"} {
		fixture.send(id, "prompt", "Go.")
		fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 4 {
		t.Fatalf("got %d requests, want 4", len(recorder.bodies))
	}
	last := recorder.bodies[len(recorder.bodies)-1]
	if strings.Contains(last, "no longer applies") {
		t.Fatal("the next prompt withdrew a nested rule that still applies")
	}
	if strings.Contains(last, "instructionContextChanged") {
		t.Fatal("the write beside an already delivered nested rule was blocked")
	}
	if got := strings.Count(last, "NESTED-RULE-MARKER"); got != 1 {
		t.Fatalf("the nested rule reached the model %d times, want exactly once", got)
	}
	if data, err := os.ReadFile(filepath.Join(root, "pkg/sub/b.txt")); err != nil || string(data) != "b\n" {
		t.Fatalf("the write did not land: %v %q", err, data)
	}
}

// TestLiveEqualBytesInUserAndProjectRootsAreNotMerged covers SPEC-0007 §10: identical text in a
// user root and a project root stays two frames, because sameness is never inferred from bytes.
func TestLiveEqualBytesInUserAndProjectRootsAreNotMerged(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	const rule = "Identical rule text that appears twice.\n"
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "agent", "AGENTS.md"), filepath.Join(root, "AGENTS.md")} {
		if err := os.WriteFile(path, []byte(rule), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "AGENTS.md"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("script", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	body := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\"`, `"`).Replace(recorder.bodies[0])
	if got := strings.Count(body, `src="instructions.steering"`); got != 2 {
		t.Fatalf("identical bytes in two roots produced %d steering frames, want 2:\n%s", got, body)
	}
	if got := strings.Count(body, "Identical rule text that appears twice."); got != 2 {
		t.Fatalf("rule text delivered %d times, want 2", got)
	}
}

// TestLiveCdIntoANestedTreeDeliversItsInstructionsWithoutRepeatingTheRoot covers a location change
// between instruction deliveries: the new nearest file arrives once, the root view is not resent.
func TestLiveCdIntoANestedTreeDeliversItsInstructionsWithoutRepeatingTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"AGENTS.md": "ROOT-RULE-MARKER\n", "a.txt": "a\n", filepath.Join("sub", "AGENTS.md"): "SUB-RULE-MARKER\n", filepath.Join("sub", "x.txt"): "x\n"}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a.txt"}},
		{"cd", map[string]any{"path": "sub"}},
		{"read", map[string]any{"path": "x.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,cd")
	fixture.send("script", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 4 {
		t.Fatalf("got %d requests, want 4", len(recorder.bodies))
	}
	last := recorder.bodies[3]
	if !strings.Contains(last, "SUB-RULE-MARKER") {
		t.Fatalf("nearest instructions of the new location never arrived")
	}
	if strings.Contains(recorder.bodies[1], "SUB-RULE-MARKER") {
		t.Fatalf("nested instructions arrived before the location changed")
	}
	if got := strings.Count(last, "ROOT-RULE-MARKER"); got != 1 {
		t.Fatalf("root rule appears %d times after the location change, want 1", got)
	}
	if got := strings.Count(last, "SUB-RULE-MARKER"); got != 1 {
		t.Fatalf("nested rule appears %d times, want 1", got)
	}
}

// Reading a file in another repository, or in a nested worktree below the project, must neither
// withdraw this project's root rule nor make it arrive twice: the cwd-derived scope stays active
// beside any tool target (SPEC-0007 §4).
func TestLiveTouchingAnotherRepositoryKeepsTheRootRuleActive(t *testing.T) {
	for name, foreignOf := range map[string]func(root string) string{
		"sibling repository": func(string) string { return t.TempDir() },
		"nested worktree":    func(root string) string { return filepath.Join(root, ".worktrees", "feat") },
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			foreign := foreignOf(root)
			for path, text := range map[string]string{
				filepath.Join(root, ".git", "HEAD"):    "ref: refs/heads/main\n",
				filepath.Join(root, "AGENTS.md"):       "ROOT-RULE-MARKER applies everywhere.\n",
				filepath.Join(root, "a.txt"):           "a\n",
				filepath.Join(foreign, ".git", "HEAD"): "ref: refs/heads/main\n",
				filepath.Join(foreign, "f.txt"):        "f\n",
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			scripted := map[int]scriptedCall{
				0: {"read", map[string]any{"path": filepath.Join(foreign, "f.txt")}},
				1: {"read", map[string]any{"path": "a.txt"}},
				2: {"write", map[string]any{"path": "b.txt", "content": "b\n"}},
				3: {"read", map[string]any{"path": filepath.Join(foreign, "f.txt")}},
			}
			recorder := &requestRecorder{next: func(index int) *scriptedCall {
				if call, ok := scripted[index]; ok {
					return &call
				}
				return nil
			}}
			server := recorder.serve(t)
			defer server.Close()
			writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
			fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write")
			fixture.send("go", "prompt", "Go.")
			fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			last := recorder.bodies[len(recorder.bodies)-1]
			if strings.Contains(last, "no longer applies to the active tool target: "+filepath.Join(root, "AGENTS.md")) {
				t.Fatal("touching another repository withdrew the project's root rule")
			}
			if strings.Contains(last, "instructionContextChanged") {
				t.Fatal("the write in the project was blocked by redelivery")
			}
			if got := strings.Count(last, "ROOT-RULE-MARKER"); got != 1 {
				t.Fatalf("the root rule reached the model %d times, want exactly once", got)
			}
			if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "b\n" {
				t.Fatalf("the write did not land: %v %q", err, data)
			}
		})
	}
}

// A nested git worktree under the project is a repository boundary: reading it must not withdraw
// the root rule, and its boundary notice must neither repeat nor be withdrawn when work moves away.
func TestLiveNestedWorktreeTouchesDoNotChurnInstructions(t *testing.T) {
	root := t.TempDir()
	for path, text := range map[string]string{
		filepath.Join(root, ".git", "HEAD"):                    "ref: refs/heads/main\n",
		filepath.Join(root, "AGENTS.md"):                       "ROOT-RULE-MARKER applies everywhere.\n",
		filepath.Join(root, "a.txt"):                           "a\n",
		filepath.Join(root, ".worktrees", "feat", ".git"):      "gitdir: " + filepath.Join(root, ".git", "worktrees", "feat") + "\n",
		filepath.Join(root, ".worktrees", "feat", "AGENTS.md"): "WORKTREE-RULE-MARKER.\n",
		filepath.Join(root, ".worktrees", "feat", "w.txt"):     "w\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	worktree := filepath.Join(".worktrees", "feat", "w.txt")
	scripted := map[int]scriptedCall{
		0: {"read", map[string]any{"path": worktree}},
		1: {"read", map[string]any{"path": "a.txt"}},
		2: {"read", map[string]any{"path": worktree}},
		3: {"write", map[string]any{"path": "b.txt", "content": "b\n"}},
	}
	recorder := &requestRecorder{next: func(index int) *scriptedCall {
		if call, ok := scripted[index]; ok {
			return &call
		}
		return nil
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write")
	fixture.send("go", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := recorder.bodies[len(recorder.bodies)-1]
	if strings.Contains(last, "no longer applies") {
		t.Fatal("moving between the project and a nested worktree withdrew an instruction source")
	}
	if strings.Contains(last, "instructionContextChanged") {
		t.Fatal("a write was blocked by redelivery")
	}
	for _, marker := range []string{"ROOT-RULE-MARKER", "Nested repository instructions were not loaded"} {
		if got := strings.Count(last, marker); got > 1 {
			t.Fatalf("%q reached the model %d times, want at most once", marker, got)
		}
	}
	if !strings.Contains(last, "ROOT-RULE-MARKER") {
		t.Fatal("the root rule is missing")
	}
	if data, err := os.ReadFile(filepath.Join(root, "b.txt")); err != nil || string(data) != "b\n" {
		t.Fatalf("the write did not land: %v %q", err, data)
	}
}

func TestLiveLinkedWorktreeInstructionsApplyOnceWithoutTrust(t *testing.T) {
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable or failed: %v: %s", err, out)
		}
	}
	for name, body := range map[string]string{"AGENTS.md": "ROOT-RULE-MARKER applies.\n", "x.txt": "x\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(root, "init", "-q", "-b", "main")
	git(root, "add", "AGENTS.md", "x.txt")
	git(root, "commit", "-q", "-m", "init")
	git(root, "worktree", "add", "-q", ".worktrees/same", "-b", "same")
	git(root, "worktree", "add", "-q", ".worktrees/feat", "-b", "feat")
	feat := filepath.Join(root, ".worktrees", "feat")
	if err := os.WriteFile(filepath.Join(feat, "AGENTS.md"), []byte("WORKTREE-RULE-MARKER applies.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": ".worktrees/feat/x.txt"}},
		{"read", map[string]any{"path": ".worktrees/feat/x.txt:1"}},
		{"read", map[string]any{"path": ".worktrees/same/x.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("first", "prompt", "Read the files in the worktrees.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := recorder.bodies[len(recorder.bodies)-1]
	if strings.Contains(last, "Nested repository instructions were not loaded") {
		t.Fatal("a linked worktree must not be reported as an untrusted nested repository")
	}
	if got := strings.Count(last, "WORKTREE-RULE-MARKER"); got != 1 {
		t.Fatalf("the worktree's own rules must arrive exactly once, got %d", got)
	}
	if got := strings.Count(last, "ROOT-RULE-MARKER"); got != 1 {
		t.Fatalf("the root rules, repeated byte for byte in a worktree, must arrive exactly once, got %d", got)
	}
}

func TestLiveMultiReadDeliversTheInstructionsOfEveryTarget(t *testing.T) {
	root := t.TempDir()
	instructionTree(t, root, map[string]string{"a": "PATHS-RULE-A applies.", "a/deep": "PATHS-RULE-DEEP applies.", "b": "PATHS-RULE-B applies."})
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"paths": []string{"a/deep/x.txt", "b/x.txt:1"}}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("p", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if strings.Contains(recorder.bodies[0], "PATHS-RULE") {
		t.Fatal("nested instructions must not load before a target is touched")
	}
	last := recorder.bodies[len(recorder.bodies)-1]
	for _, marker := range []string{"PATHS-RULE-A", "PATHS-RULE-DEEP", "PATHS-RULE-B"} {
		if got := strings.Count(last, marker); got != 1 {
			t.Errorf("%s reached the model %d times, want exactly once", marker, got)
		}
	}
}

func TestLiveSessionSelfInstructionsListActiveFilesAndCtlInJSONMode(t *testing.T) {
	root, work := t.TempDir(), t.TempDir()
	instructionTree(t, work, map[string]string{".": "LISTED-ROOT-RULE applies.", "a": "LISTED-NESTED-RULE applies."})
	nested := filepath.Join(work, "a", "AGENTS.md")
	if err := os.WriteFile(filepath.Join(work, "a", "BACKLOG.md"), []byte("- [B-1] LISTED-BACKLOG item\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a/x.txt"}},
		{"write", map[string]any{"path": "session://self/instructions/ctl", "content": "pin a/BACKLOG.md"}},
		{"ls", map[string]any{"path": "session://self/instructions/"}},
		{"ls", map[string]any{"path": "session://self/"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runPigJSON(t, ctx, repoRoot(t), root, work, []string{"--tools", "read,write,ls"}, "inspect")
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 4 {
		t.Fatalf("want 4 tool results, have %d: %q", len(results), results)
	}
	listing := results[2]
	for _, want := range []string{"file://" + nested, "file://" + filepath.Join(work, "AGENTS.md"), "steering · delivered", "backlog-a-BACKLOG-md", "pinned by agent", "ctl"} {
		if !strings.Contains(listing, want) {
			t.Errorf("instruction listing lacks %q:\n%s", want, listing)
		}
	}
	if !strings.Contains(results[3], "instructions") {
		t.Errorf("self listing lacks instructions:\n%s", results[3])
	}
}

func TestLiveSelfEditedInstructionsFollowTheSelfEditsSetting(t *testing.T) {
	for mode, want := range map[string]struct{ notice, diff bool }{"notice": {notice: true}, "diff": {diff: true}, "silent": {}} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			agent := filepath.Join(root, "agent")
			if err := os.MkdirAll(agent, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[instructions.updates]\nself-edits = \""+mode+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			steering := strings.Repeat("Steering rule that must hold.\n", 40)
			if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(steering), 0o600); err != nil {
				t.Fatal(err)
			}
			recorder := &requestRecorder{calls: []scriptedCall{
				{"read", map[string]any{"path": "AGENTS.md"}},
				{"write", map[string]any{"path": "AGENTS.md", "content": steering + "SELF-AUTHORED-RULE applies.\n"}},
				{"write", map[string]any{"path": "b.txt", "content": "made\n"}},
			}}
			server := recorder.serve(t)
			defer server.Close()
			writeFixtureProvider(t, agent, server.URL)
			fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write")
			fixture.send("p", "prompt", "Go.")
			fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			last := recorder.bodies[len(recorder.bodies)-1]
			results := toolResults(t, last)
			if len(results) != 3 || !strings.Contains(results[1], "Wrote") {
				t.Fatalf("the self-edit did not land: %q", results)
			}
			if got := strings.Contains(last, "You changed this instruction file yourself"); got != want.notice {
				t.Errorf("self-edit notice present = %t, want %t", got, want.notice)
			}
			if got := strings.Contains(last, "Instruction update for"); got != want.diff {
				t.Errorf("instruction diff present = %t, want %t", got, want.diff)
			}
			if mode != "diff" && strings.Contains(results[2], "instructionContextChanged") {
				t.Errorf("the agent's own edit must not block its next mutation: %q", results[2])
			}
		})
	}
}

func TestLiveInstructionBoundarySettingSelectsHowFarDiscoveryAscends(t *testing.T) {
	for boundary, want := range map[string]bool{"project": false, "filesystem": true} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			work := filepath.Join(root, "outer", "repo")
			instructionTree(t, work, map[string]string{".": "INSIDE-PROJECT-RULE applies."})
			if err := os.WriteFile(filepath.Join(root, "outer", "AGENTS.md"), []byte("ABOVE-PROJECT-RULE applies.\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			agent := filepath.Join(root, "agent")
			if err := os.MkdirAll(agent, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[instructions]\nboundary = \""+boundary+"\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "x.txt"}}}}
			server := recorder.serve(t)
			defer server.Close()
			writeFixtureProvider(t, agent, server.URL)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runPigJSON(t, ctx, repoRoot(t), root, work, []string{"--tools", "read"}, "go")
			recorder.mu.Lock()
			defer recorder.mu.Unlock()
			last := recorder.bodies[len(recorder.bodies)-1]
			if strings.Count(last, "INSIDE-PROJECT-RULE") != 1 {
				t.Fatal("the project's own steering must arrive exactly once")
			}
			if got := strings.Contains(last, "ABOVE-PROJECT-RULE"); got != want {
				t.Errorf("steering above the project delivered = %t, want %t", got, want)
			}
		})
	}
}
