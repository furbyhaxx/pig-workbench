//go:build integration

package host

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tuiSession drives a real pig TUI in a private tmux server.
type tuiSession struct {
	t      *testing.T
	socket string
	env    []string
}

func startTUI(t *testing.T, root string, fixtures ...string) *tuiSession {
	t.Helper()
	runtime := shortRuntime(t)
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	agent := filepath.Join(root, "agent")
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, agent, server.URL)
	s := &tuiSession{t: t, socket: filepath.Join(runtime, "tmux.sock"), env: append(os.Environ(), "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))}
	command := []string{"new-session", "-d", "-s", "wb", "-x", "100", "-y", "32", "-c", root, "pig", "--offline", "--no-session", "-ne", "-e", repoRoot(t)}
	for _, fixture := range fixtures {
		command = append(command, "-e", filepath.Join(repoRoot(t), "internal/host/testdata", fixture))
	}
	s.tmux(append(command, "--model", "workbench-fixture/mock")...)
	stopTUI(t, s.socket, "wb")
	s.waitFor("0.0%/32k", 20*time.Second)
	return s
}

// stopTUI registers a cleanup that ends the tmux-hosted pig and waits until it has exited,
// so TempDir removal never races a process still writing its logs and state.
func stopTUI(t *testing.T, socket, session string) {
	t.Helper()
	pid := ""
	if out, err := exec.Command("tmux", "-S", socket, "display-message", "-p", "-t", session, "#{pane_pid}").Output(); err == nil {
		pid = strings.TrimSpace(string(out))
	}
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-S", socket, "kill-server").Run()
		if pid == "" {
			return
		}
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat("/proc/" + pid); err != nil {
				time.Sleep(300 * time.Millisecond)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		_ = exec.Command("kill", "-9", "-"+pid).Run()
		time.Sleep(time.Second)
	})
}

func (s *tuiSession) tmux(args ...string) string {
	s.t.Helper()
	cmd := exec.Command("tmux", append([]string{"-S", s.socket}, args...)...)
	cmd.Env = s.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.t.Fatalf("tmux %v: %v: %s", args, err, out)
	}
	return string(out)
}

func (s *tuiSession) screen() string { return s.tmux("capture-pane", "-p", "-t", "wb") }

func (s *tuiSession) keys(keys ...string) {
	s.t.Helper()
	for _, key := range keys {
		s.tmux("send-keys", "-t", "wb", key)
		time.Sleep(300 * time.Millisecond)
	}
}

func (s *tuiSession) typeText(text string) {
	s.t.Helper()
	s.tmux("send-keys", "-t", "wb", "-l", text)
	time.Sleep(400 * time.Millisecond)
}

func (s *tuiSession) waitFor(want string, within time.Duration) string {
	s.t.Helper()
	deadline := time.Now().Add(within)
	var screen string
	for time.Now().Before(deadline) {
		if screen = s.screen(); strings.Contains(screen, want) {
			return screen
		}
		time.Sleep(150 * time.Millisecond)
	}
	s.t.Fatalf("never saw %q:\n%s", want, screen)
	return ""
}

// openHub types /workbench and submits it; the first Enter may only accept an argument completion.
func (s *tuiSession) openHub() string {
	s.typeText("/workbench")
	s.keys("Enter")
	time.Sleep(time.Second)
	if screen := s.screen(); !strings.Contains(screen, "Workbench ·") {
		s.keys("Enter")
	}
	return s.waitFor("Workbench ·", 10*time.Second)
}

func TestLiveWorkbenchMenuOpensInteractivePanelsInTUI(t *testing.T) {
	root := t.TempDir()
	s := startTUI(t, root)

	hub := s.openHub()
	for _, want := range []string{"Tools", "Jobs", "Settings", "Instructions", "Integrations"} {
		if !strings.Contains(hub, want) {
			t.Fatalf("hub lacks %q:\n%s", want, hub)
		}
	}
	if strings.Contains(hub, "extension error") || strings.Contains(hub, "not found") {
		t.Fatalf("hub opened with an error:\n%s", hub)
	}

	s.keys("Enter")
	tools := s.waitFor("Workbench › Tools", 10*time.Second)
	if !strings.Contains(tools, "lines 1-") || !strings.Contains(tools, "read [active]") {
		t.Fatalf("tools view is not a scrollable panel:\n%s", tools)
	}
	s.keys("Escape")
	s.waitFor("Workbench ·", 10*time.Second)

	s.keys("Down", "Down", "Enter")
	s.waitFor("Workbench › Settings · user", 10*time.Second)
	s.keys("Enter", "Down", "Enter")
	staged := s.waitFor("unsaved", 10*time.Second)
	if !strings.Contains(staged, `"replace" [unsaved]`) || !strings.Contains(staged, "false [unsaved]") {
		t.Fatalf("choice or boolean edit was not staged:\n%s", staged)
	}
	s.keys("C-s")
	s.waitFor("Saved 2 to user", 10*time.Second)
	data, err := os.ReadFile(filepath.Join(root, "agent", "Workbench.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `mode = "replace"`) || !strings.Contains(string(data), "enforce-seen-lines = false") {
		t.Fatalf("panel save did not persist both edits:\n%s", data)
	}

	s.keys("Down", "Down", "Down", "Enter")
	s.waitFor("edit.snapshot.max-versions", 10*time.Second)
	s.keys("Down", "Enter")
}

func TestLiveWorkbenchSettingsPanelRefusesInvalidIntegerAndGuardsDiscard(t *testing.T) {
	s := startTUI(t, t.TempDir())
	s.openHub()
	s.keys("Down", "Down", "Enter")
	s.waitFor("Workbench › Settings · user", 10*time.Second)
	s.keys("Down", "Down", "Down", "Enter")
	s.waitFor("whole number 1 to 65536", 10*time.Second)
	s.typeText("99999999")
	s.keys("Enter")
	s.waitFor("must be between 1 and 65536", 10*time.Second)
	s.keys("Escape")
	s.waitFor("Workbench › Settings · user", 10*time.Second)
	s.keys("Up", "Enter")
	s.waitFor("1 unsaved", 10*time.Second)
	s.keys("Escape")
	screen := s.waitFor("Unsaved changes", 10*time.Second)
	if !strings.Contains(screen, "Settings") {
		t.Fatalf("first escape left the panel with unsaved changes:\n%s", screen)
	}
	s.keys("Escape")
	s.waitFor("Workbench ·", 10*time.Second)
}

func TestLiveToolsWithAnUnknownNameWarnsInsteadOfFailingTheCommand(t *testing.T) {
	s := startTUI(t, t.TempDir())
	s.typeText("/workbench tools settings")
	s.keys("Enter")
	time.Sleep(time.Second)
	s.keys("Enter")
	screen := s.waitFor("No tool named", 10*time.Second)
	if strings.Contains(screen, "extension error") || strings.Contains(screen, `Extension "command:workbench" error`) {
		t.Fatalf("an unknown tool name still fails the command:\n%s", screen)
	}
}

func TestLiveEveryHubEntryOpensATitledPanelWithoutAnError(t *testing.T) {
	s := startTUI(t, t.TempDir())
	s.openHub()
	for i, item := range (&app{}).hubItemsStatic() {
		s.keys("Enter")
		screen := s.waitFor("Workbench › "+item.label, 15*time.Second)
		for _, bad := range []string{`Extension "command:workbench"`, "extension error", "not found", "usage:", "Error:"} {
			if strings.Contains(screen, bad) {
				t.Fatalf("hub entry %d (%s) failed with %q:\n%s", i, item.label, bad, screen)
			}
		}
		for attempts := 0; attempts < 4 && !strings.Contains(s.screen(), "Workbench · "); attempts++ {
			s.keys("Escape")
		}
		s.waitFor("Workbench · ", 10*time.Second)
		s.keys("Down")
	}
}

func TestLiveToolDisplayViewExplainsAnEmptyStateAndEditsWhenASchemaExists(t *testing.T) {
	t.Run("plain Workbench", func(t *testing.T) {
		s := startTUI(t, t.TempDir())
		hub := s.openHub()
		if !strings.Contains(hub, "Tool display") || !strings.Contains(hub, "Nothing to configure") {
			t.Fatalf("hub does not say the entry is empty:\n%s", hub)
		}
		s.keys("Down", "Down", "Down", "Enter")
		screen := s.waitFor("Workbench › Tool display", 10*time.Second)
		for _, want := range []string{"Nothing to configure yet", "another extension registers", "/workbench integrations"} {
			if !strings.Contains(strings.Join(strings.Fields(screen), " "), want) {
				t.Fatalf("empty state does not explain itself (%q missing):\n%s", want, screen)
			}
		}
		if strings.Contains(screen, "No registered or dormant") {
			t.Fatalf("the bare empty-state message came back:\n%s", screen)
		}
	})
	t.Run("with a registered presentation", func(t *testing.T) {
		root := t.TempDir()
		s := startTUI(t, root, "reference-producer")
		s.waitFor("web ready", 20*time.Second)
		hub := s.openHub()
		if !strings.Contains(hub, "tool(s) with display preferences") {
			t.Fatalf("hub does not count the registered schema:\n%s", hub)
		}
		s.keys("Down", "Down", "Down", "Enter")
		panel := s.waitFor("Workbench › Tool display · user", 10*time.Second)
		if !strings.Contains(panel, "density") || !strings.Contains(panel, "preview-lines") || !strings.Contains(panel, "reference-producer/web_search") {
			t.Fatalf("display preferences are not an editable settings list:\n%s", panel)
		}
		if strings.Contains(panel, "edit.mode") {
			t.Fatalf("the panel is not limited to tool display settings:\n%s", panel)
		}
		s.typeText("preview")
		s.keys("Enter")
		s.waitFor("preview-lines", 10*time.Second)
	})
}
