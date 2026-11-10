//go:build integration

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// WB_CAPTURE_DIR, when set, receives the collapsed and expanded transcript at each review size.
func TestLiveFramesRenderAsRowsNotEnvelopes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Fixture\n\n- Keep rows short.\n- Never print envelopes.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skill := filepath.Join(root, "agent", "skills", "row-fixture")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: row-fixture\ndescription: a skill for the row capture\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	s.typeText("hello")
	s.keys("Enter")
	s.waitFor("session.date", 20*time.Second)
	time.Sleep(2 * time.Second)
	out := os.Getenv("WB_CAPTURE_DIR")
	home, _ := os.UserHomeDir()
	redact := strings.NewReplacer(root, "$ROOT", home, "$HOME")
	for _, size := range [][2]int{{36, 32}, {57, 32}, {80, 32}, {140, 32}, {80, 16}} {
		s.tmux("resize-window", "-t", "wb", "-x", fmt.Sprint(size[0]), "-y", fmt.Sprint(size[1]))
		time.Sleep(time.Second)
		collapsed := s.history()
		if strings.Contains(collapsed, "<system-") || strings.Contains(collapsed, "[pig-workbench.context]") {
			t.Fatalf("%dx%d: a frame is shown as its envelope:\n%s", size[0], size[1], collapsed)
		}
		if size[1] >= 32 && (!strings.Contains(collapsed, "i session.skills · ") || strings.Contains(collapsed, "- skill://")) {
			t.Fatalf("%dx%d: the skill listing is not one collapsed row with its count:\n%s", size[0], size[1], collapsed)
		}
		if !strings.Contains(collapsed, "◆ instructions.steering") {
			t.Fatalf("%dx%d: no frame row:\n%s", size[0], size[1], collapsed)
		}
		s.keys("C-o")
		time.Sleep(time.Second)
		expanded := s.history()
		s.keys("C-o")
		if size[0] >= 57 && !strings.Contains(collapsed, "AGENTS.md · loaded") {
			t.Fatalf("%dx%d: the instruction row does not name its file and state:\n%s", size[0], size[1], collapsed)
		}
		if !strings.Contains(expanded, "│ ") || !strings.Contains(expanded, "└ ") {
			t.Fatalf("%dx%d: expansion shows no body or provenance:\n%s", size[0], size[1], expanded)
		}
		if out != "" && size == [2]int{80, 32} {
			if err := os.WriteFile(filepath.Join(out, "w80-h32-collapsed.ansi"), []byte(redact.Replace(s.tmux("capture-pane", "-e", "-p", "-S", "-", "-t", "wb"))), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if out != "" {
			for name, text := range map[string]string{"collapsed": collapsed, "expanded": expanded} {
				if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("w%d-h%d-%s.txt", size[0], size[1], name)), []byte(redact.Replace(text)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestLiveThinkingLevelChangeSendsOnlyTheChangeNeverTheModelFrameAgain(t *testing.T) {
	tuiModel = "mock-reasoning"
	t.Cleanup(func() { tuiModel = "mock" })
	s := startTUI(t, t.TempDir())
	s.typeText("hello")
	s.keys("Enter")
	s.waitFor("session.model · workbench-fixture/mock-reasoning:medium", 20*time.Second)
	s.waitFor("done", 20*time.Second)
	s.keys("BTab")
	s.typeText("again")
	s.keys("Enter")
	screen := s.waitFor("● session.model · thinking medium → ", 20*time.Second)
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "session.model") {
			t.Log(strings.TrimSpace(line))
		}
	}
	time.Sleep(2 * time.Second)
	s.recorder.mu.Lock()
	last := s.recorder.bodies[len(s.recorder.bodies)-1]
	s.recorder.mu.Unlock()
	if full := strings.Count(last, "training_data_notice"); full != 1 {
		t.Fatalf("the model frame is in the request %d times, want once", full)
	}
	if !strings.Contains(last, "Changed in session.model; every other field is unchanged.") || !strings.Contains(last, "thinking: medium -> ") {
		t.Fatalf("the request carries no change notice:\n%s", last)
	}
}

func TestLiveCdSendsTheLocationChangeAsOneSentence(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := startTUIScripted(t, root, []scriptedCall{{tool: "cd", args: map[string]any{"path": "sub"}}})
	s.typeText("go")
	s.keys("Enter")
	s.waitFor("done", 20*time.Second)
	s.typeText("next")
	s.keys("Enter")
	screen := s.waitFor("● session.location · ", 20*time.Second)
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "session.location") {
			t.Log(strings.TrimSpace(line))
		}
	}
	time.Sleep(2 * time.Second)
	s.recorder.mu.Lock()
	last := s.recorder.bodies[len(s.recorder.bodies)-1]
	s.recorder.mu.Unlock()
	if !strings.Contains(last, "Working location: "+root+"; plain paths and cwd resolve there") || !strings.Contains(last, "Working location changed from "+root+" to "+filepath.Join(root, "sub")+"; plain paths and cwd now resolve there") {
		t.Fatalf("the request lacks the location sentence or its change:\n%s", last)
	}
}

func TestLiveFrameVisibilityHidesOrOpensRowsWithoutChangingDelivery(t *testing.T) {
	for visibility, check := range map[string]func(string) bool{
		"hidden": func(screen string) bool { return !strings.Contains(screen, "session.date") },
		// The screen holds only the tail of the transcript, so any opened body's rail is the evidence.
		"shown": func(screen string) bool { return strings.Contains(screen, " │ ") },
	} {
		t.Run(visibility, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[frames.display]\ndefault-visibility = '"+visibility+"'\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			s := startTUI(t, root)
			s.typeText("hello")
			s.keys("Enter")
			s.waitFor("done", 20*time.Second)
			time.Sleep(time.Second)
			screen := s.history()
			if !check(screen) || strings.Contains(screen, "<system-") {
				t.Fatalf("visibility %s:\n%s", visibility, screen)
			}
			s.recorder.mu.Lock()
			request := strings.Join(s.recorder.bodies, "\n")
			s.recorder.mu.Unlock()
			if !strings.Contains(request, "session.date") {
				t.Fatalf("visibility %s changed delivery: the provider request has no date frame", visibility)
			}
		})
	}
}

func TestLiveFramesVerbsInspectAndSetTranscriptVisibilityInRPC(t *testing.T) {
	root := t.TempDir()
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("p", "prompt", "say done")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	notice := func(id, command, want string) string {
		fixture.send(id, "prompt", command)
		event := fixture.wait(func(event map[string]any) bool {
			return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), want)
		})
		return fmt.Sprint(event["message"])
	}
	list := notice("ls", "/workbench frames ls", "session.date")
	if !strings.Contains(list, "#1 ") || !strings.Contains(list, "[delivered]") {
		t.Fatalf("listing lacks sequence and state:\n%s", list)
	}
	if one := notice("one", "/workbench frames ls #1", "state delivered"); !strings.Contains(one, "source ") || !strings.Contains(one, "digest ") {
		t.Fatalf("frame view lacks provenance:\n%s", one)
	}
	notice("hide", "/workbench frames hide session.date", "Frames from session.date are now hidden")
	if marked := notice("ls2", "/workbench frames ls", "session.date"); !strings.Contains(marked, "(hidden)") {
		t.Fatalf("listing does not show the override:\n%s", marked)
	}
	saved, err := os.ReadFile(filepath.Join(root, "agent", "Workbench.toml"))
	if err != nil || !strings.Contains(string(saved), "hidden session.date") {
		t.Fatalf("rule not saved: %v\n%s", err, saved)
	}
	fixture.send("p2", "prompt", "say done again")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" && len(recorder.bodies) >= 2 })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if !strings.Contains(recorder.bodies[0], "session.date") {
		t.Fatal("the model must still receive the hidden source")
	}
}
