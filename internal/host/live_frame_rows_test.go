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
