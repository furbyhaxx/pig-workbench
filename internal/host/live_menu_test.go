//go:build integration

package host

import (
	"fmt"

	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tuiSession drives a real pig TUI in a private tmux server.
type tuiSession struct {
	t      *testing.T
	socket string
	env    []string
	// recorder holds the provider requests the mock model received.
	recorder *requestRecorder
}

// tuiModel is the fixture model the next TUI starts with.
var tuiModel = "mock"

func startTUI(t *testing.T, root string, fixtures ...string) *tuiSession {
	t.Helper()
	return startTUIScripted(t, root, nil, fixtures...)
}

// startTUIScripted starts a TUI whose mock provider answers the first prompts with these tool calls.
func startTUIScripted(t *testing.T, root string, calls []scriptedCall, fixtures ...string) *tuiSession {
	t.Helper()
	runtime := shortRuntime(t)
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	agent := filepath.Join(root, "agent")
	isolateHostFiles(t, agent)
	recorder := &requestRecorder{calls: calls}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, agent, server.URL)
	s := &tuiSession{t: t, recorder: recorder, socket: filepath.Join(runtime, "tmux.sock"), env: append(os.Environ(), "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+agent, "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))}
	command := []string{"new-session", "-d", "-s", "wb", "-x", "100", "-y", "32", "-c", root, "pig", "--offline", "--no-session", "-ne", "-e", repoRoot(t)}
	for _, fixture := range fixtures {
		command = append(command, "-e", filepath.Join(repoRoot(t), "internal/host/testdata", fixture))
	}
	s.tmux(append(command, "--model", "workbench-fixture/"+tuiModel)...)
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

// history captures the whole scrollback, for notices a long reply has pushed off the visible pane.
func (s *tuiSession) history() string { return s.tmux("capture-pane", "-p", "-S", "-", "-t", "wb") }

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

// typeSlowly sends one character at a time and waits for the full text to be echoed; a burst
// can be partly dropped while the panel re-renders, and Enter must not follow a half-typed value.
func (s *tuiSession) typeSlowly(text string) {
	s.t.Helper()
	for _, r := range text {
		s.tmux("send-keys", "-t", "wb", "-l", string(r))
		time.Sleep(80 * time.Millisecond)
	}
	s.waitFor(text, 5*time.Second)
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

// capture writes the screen at 140, 80, 57 and 36 columns by 16 rows to
// $WORKBENCH_CAPTURE_DIR/<name>/w<width>.txt; without the variable it does nothing.
func (s *tuiSession) capture(name string) {
	dir := os.Getenv("WORKBENCH_CAPTURE_DIR")
	if dir == "" {
		return
	}
	target := filepath.Join(dir, name)
	if err := os.MkdirAll(target, 0o755); err != nil {
		s.t.Fatal(err)
	}
	for _, width := range []int{140, 80, 57, 36} {
		s.tmux("resize-window", "-t", "wb", "-x", strconv.Itoa(width), "-y", "16")
		time.Sleep(900 * time.Millisecond)
		if err := os.WriteFile(filepath.Join(target, fmt.Sprintf("w%d.txt", width)), []byte(s.screen()), 0o644); err != nil {
			s.t.Fatal(err)
		}
	}
	s.tmux("resize-window", "-t", "wb", "-x", "100", "-y", "32")
	time.Sleep(900 * time.Millisecond)
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

	s.keys("Down", "Enter")
	s.waitFor("Settings › edit.snapshot.", 10*time.Second)
	s.keys("Down", "Enter")
}

func TestLiveWorkbenchSettingsPanelRefusesInvalidIntegerAndGuardsDiscard(t *testing.T) {
	s := startTUI(t, t.TempDir())
	s.openHub()
	s.keys("Down", "Down", "Enter")
	s.waitFor("Workbench › Settings · user", 10*time.Second)
	s.keys("Down", "Down", "Down", "Enter")
	s.waitFor("whole number 1 to ", 10*time.Second)
	s.typeSlowly("99999999")
	s.keys("Enter")
	s.waitFor("must be between 1 and ", 10*time.Second)
	s.keys("Escape")
	s.waitFor("Workbench › Settings · user", 10*time.Second)
	s.keys("Up", "Up", "Enter")
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

func TestLiveHubSearchNarrowsEntriesAndOpensTheMatch(t *testing.T) {
	s := startTUI(t, t.TempDir())
	hub := s.openHub()
	if !strings.Contains(hub, "Tool display") || !strings.Contains(hub, "Sessions") {
		t.Fatalf("hub lacks entries before searching:\n%s", hub)
	}
	s.typeText("messag")
	screen := s.waitFor("search: messag", 10*time.Second)
	if !strings.Contains(screen, "Sessions") || strings.Contains(screen, "Tool display") || strings.Contains(screen, "Integrations") {
		t.Fatalf("the search did not narrow the list to the messaging entry:\n%s", screen)
	}
	s.capture("hub-search")
	s.keys("Escape")
	cleared := s.waitFor("Integrations", 10*time.Second)
	if strings.Contains(cleared, "search: messag") {
		t.Fatalf("escape must clear the search before closing the hub:\n%s", cleared)
	}
	s.typeText("zzzz")
	if screen := s.waitFor("No entry matches", 10*time.Second); strings.Contains(screen, "Sessions") {
		t.Fatalf("a search without a match still lists entries:\n%s", screen)
	}
	s.keys("Escape")
	s.typeText("jobs")
	s.keys("Enter")
	s.waitFor("Workbench › Jobs", 10*time.Second)
}

func TestLiveSettingsPanelEditsAnArraySettingAndPersistsIt(t *testing.T) {
	root := t.TempDir()
	s := startTUI(t, root)
	s.openHub()
	s.keys("Down", "Down", "Enter")
	s.waitFor("Workbench › Settings · user", 10*time.Second)
	s.typeText("guides.disabled")
	s.waitFor("search: guides.disabled", 10*time.Second)
	s.keys("Enter")
	s.waitFor("+ add item", 10*time.Second)
	s.keys("Enter")
	s.waitFor("New item", 10*time.Second)
	s.typeText("kb")
	s.keys("Enter")
	screen := s.waitFor("changed;", 10*time.Second)
	if !strings.Contains(screen, "kb") {
		t.Fatalf("the added item is not listed:\n%s", screen)
	}
	s.capture("settings-array")
	s.keys("Escape")
	staged := s.waitFor("[unsaved]", 10*time.Second)
	if !strings.Contains(staged, `["kb"] [unsaved]`) {
		t.Fatalf("the array was not staged as a whole:\n%s", staged)
	}
	s.keys("C-s")
	s.waitFor("Saved 1 to user", 10*time.Second)
	data, err := os.ReadFile(filepath.Join(root, "agent", "Workbench.toml"))
	if err != nil || !strings.Contains(string(data), `disabled = ["kb"]`) {
		t.Fatalf("array setting was not persisted: %v\n%s", err, data)
	}
	s.keys("Enter")
	s.waitFor("kb", 10*time.Second)
	s.typeText("d")
	s.keys("Escape")
	s.waitFor("[unsaved]", 10*time.Second)
	s.keys("C-s")
	s.waitFor("Saved 1 to user", 10*time.Second)
	data, err = os.ReadFile(filepath.Join(root, "agent", "Workbench.toml"))
	if err != nil || strings.Contains(string(data), `"kb"`) {
		t.Fatalf("the removed item is still persisted: %v\n%s", err, data)
	}
}

func TestLiveJobsPanelKillsWaitsAndRefreshesARealJob(t *testing.T) {
	root := t.TempDir()
	calls := []scriptedCall{
		{"bash", map[string]any{"command": "sh -c 'echo panel-job-output; sleep 300'", "background": true}},
		{"bash", map[string]any{"command": "sh -c 'echo quick-done'", "background": true}},
	}
	s := startTUIScripted(t, root, calls)
	s.typeText("start two jobs")
	s.keys("Enter")
	s.waitFor("done", 30*time.Second)
	s.openHub()
	s.keys("Down", "Enter")
	list := s.waitFor("Workbench › Jobs", 10*time.Second)
	if !strings.Contains(list, "running") || !strings.Contains(list, "enter open · k stop") {
		t.Fatalf("the jobs panel does not list the running job with its actions:\n%s", list)
	}
	s.capture("jobs-list")
	s.keys("Enter")
	detail := s.waitFor("panel-job-output", 10*time.Second)
	s.capture("jobs-detail")
	s.keys("r")
	s.waitFor("Refreshed", 10*time.Second)
	s.keys("w")
	s.waitFor("Waited on", 15*time.Second)
	if !strings.Contains(s.screen(), "running") {
		t.Fatalf("a timed-out wait must leave the job running:\n%s\n%s", detail, s.screen())
	}
	s.keys("k")
	s.waitFor("Stop job", 10*time.Second)
	s.keys("n")
	s.waitFor("Stop cancelled", 10*time.Second)
	if !strings.Contains(s.screen(), "running") {
		t.Fatalf("cancelling the confirmation stopped the job:\n%s", s.screen())
	}
	s.keys("k", "y")
	stopped := s.waitFor("Stopped", 10*time.Second)
	if !strings.Contains(stopped, "cancelled") {
		t.Fatalf("the job did not report cancelled after the confirmed stop:\n%s", stopped)
	}
	s.keys("Escape")
	s.waitFor("enter open · k stop", 10*time.Second)
}

func TestLiveInstructionsPanelPinsAndUnpinsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "BACKLOG.md"), []byte("# Backlog\n- [ ] one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	s.openHub()
	s.keys("Down", "Down", "Down", "Down", "Enter")
	empty := s.waitFor("Workbench › Instructions", 10*time.Second)
	if !strings.Contains(empty, "No pinned instruction files") {
		t.Fatalf("the panel does not start empty:\n%s", empty)
	}
	s.keys("p")
	s.waitFor("Path of the file to pin", 10*time.Second)
	s.typeSlowly("BACKLOG.md")
	s.keys("Enter")
	pinned := s.waitFor("pinned", 10*time.Second)
	if !strings.Contains(pinned, "BACKLOG.md") {
		t.Fatalf("the pin is not listed:\n%s", pinned)
	}
	s.capture("instructions-pinned")
	s.keys("v")
	s.waitFor("› rules", 10*time.Second)
	s.keys("Escape")
	s.waitFor("p pin · u unpin", 10*time.Second)
	s.keys("Escape", "Escape")
	s.typeText("/workbench instructions ls")
	s.keys("Enter")
	time.Sleep(time.Second)
	if !strings.Contains(s.screen(), "Pinned instruction files") {
		s.keys("Enter")
	}
	live := s.waitFor("Pinned instruction files", 10*time.Second)
	if !strings.Contains(live, "BACKLOG.md") {
		t.Fatalf("the live pin state lacks the pin made in the panel:\n%s", live)
	}
	s.keys("Escape")
	s.openHub()
	s.keys("Down", "Down", "Down", "Down", "Enter")
	s.waitFor("p pin · u unpin", 10*time.Second)
	s.keys("u")
	s.waitFor("unpinned", 10*time.Second)
}

func TestLiveTrustPanelAddsAndRemovesARulePersistently(t *testing.T) {
	root := t.TempDir()
	rule := "allow " + filepath.Join(root, "nested")
	s := startTUI(t, root)
	s.openHub()
	s.keys("Down", "Down", "Down", "Down", "Down", "Down", "Down", "Down", "Down", "Down", "Down", "Enter")
	empty := s.waitFor("Workbench › Trust", 10*time.Second)
	if !strings.Contains(empty, "No trust rules") {
		t.Fatalf("the panel does not start empty:\n%s", empty)
	}
	s.keys("a")
	s.waitFor("allow <absolute dir>", 10*time.Second)
	s.typeSlowly(rule)
	s.keys("Enter")
	saved := s.waitFor("Saved allow", 15*time.Second)
	s.capture("trust-rules")
	if !strings.Contains(saved, "nested") {
		t.Fatalf("the rule is not listed:\n%s", saved)
	}
	data, err := os.ReadFile(filepath.Join(root, "agent", "Workbench.toml"))
	if err != nil || !strings.Contains(string(data), rule) {
		t.Fatalf("the rule was not persisted: %v\n%s", err, data)
	}
	s.keys("d")
	s.waitFor("Removed allow", 15*time.Second)
	data, err = os.ReadFile(filepath.Join(root, "agent", "Workbench.toml"))
	if err != nil || strings.Contains(string(data), rule) {
		t.Fatalf("the removed rule is still persisted: %v\n%s", err, data)
	}
	s.waitFor("No trust rules", 10*time.Second)
}

func TestLiveSessionsPanelMessagesALivePeerAndTogglesPolicy(t *testing.T) {
	s := startTUI(t, t.TempDir())
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	receiver := startLiveSession(t, runtime, nil)
	waitForRecords(t, runtime, 2)
	s.openHub()
	s.keys("Down", "Down", "Down", "Down", "Down", "Down", "Down", "Down", "Enter")
	list := s.waitFor("Workbench › Sessions", 10*time.Second)
	if !strings.Contains(list, "m message") {
		t.Fatalf("the sessions panel does not offer its actions:\n%s", list)
	}
	s.waitFor("idle", 15*time.Second)
	s.capture("sessions-panel")
	s.keys("t")
	s.waitFor("trust:", 10*time.Second)
	s.waitFor("trusted", 10*time.Second)
	s.keys("t")
	s.waitFor("untrust:", 10*time.Second)
	s.keys("b")
	s.waitFor("blocked", 10*time.Second)
	s.keys("b")
	s.waitFor("unblock:", 10*time.Second)
	s.keys("m")
	s.waitFor("Message to", 10*time.Second)
	s.typeSlowly("PANEL-MSG")
	s.keys("Enter")
	s.waitFor("accepted by", 15*time.Second)
	waitForRequest(t, receiver, "PANEL-MSG")
}

func TestLiveSubmittedPromptSurvivesAnUnavailableSecretsBoundary(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "Secrets.toml"), []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	prompt := "my carefully typed five minute prompt"
	s.typeSlowly(prompt)
	s.keys("Enter")
	screen := s.waitFor("back in the editor", 10*time.Second)
	if strings.Count(screen, prompt) != 1 {
		t.Fatalf("the prompt must be back in the editor exactly once:\n%s", screen)
	}
}

func TestLiveSubmittedPromptIsSentDespiteAVaultRecordFromAnotherKey(t *testing.T) {
	root := t.TempDir()
	vaultPath := filepath.Join(root, "data", "pig-workbench", "secrets", "vault.jsonl")
	foreignKey := make([]byte, 32)
	foreignKey[0] = 9
	vault, err := secrets.OpenVault(vaultPath, foreignKey)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := secrets.NewDetector(foreignKey, vault, secrets.DetectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := foreign.Learn([]byte("written-under-another-agent-dir"), "token", "env"); err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	// The notice is a toast that the next render replaces within a poll interval, so the byte stream is recorded instead.
	stream := filepath.Join(root, "pane.log")
	s.tmux("pipe-pane", "-t", "wb", "cat >> "+stream)
	prompt := "hello after the vault was polluted"
	s.typeSlowly(prompt)
	s.tmux("send-keys", "-t", "wb", "Enter")
	var screen string
	reported := false
	for deadline := time.Now().Add(10 * time.Second); !reported && time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		data, _ := os.ReadFile(stream)
		reported = strings.Contains(string(data), "cannot be decrypted")
	}
	screen = s.screen()
	if !reported {
		t.Fatalf("the ignored record was not reported:\n%s", screen)
	}
	if strings.Contains(screen, "back in the editor") || strings.Contains(screen, "withheld") {
		t.Fatalf("the boundary still failed:\n%s", screen)
	}
	for deadline := time.Now().Add(15 * time.Second); strings.Contains(screen, "0.0%/32k") && time.Now().Before(deadline); time.Sleep(150 * time.Millisecond) {
		screen = s.screen()
	}
	if strings.Contains(screen, "0.0%/32k") {
		t.Fatalf("the prompt never reached the model:\n%s", screen)
	}
}

func TestLiveWideSecretKeyIsNarrowedAndThePromptIsSent(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(agent, "secret.key")
	if _, err := secrets.CreateKey(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	s.typeSlowly("prompt with a world-readable key")
	s.tmux("send-keys", "-t", "wb", "Enter")
	// The one-time notice is covered by a unit test; the response floods the screen too fast to catch it reliably.
	var screen string
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if screen = s.screen(); !strings.Contains(screen, "0.0%/32k") {
			break
		}
	}
	if strings.Contains(screen, "back in the editor") || strings.Contains(screen, "0.0%/32k") {
		t.Fatalf("the prompt was not sent:\n%s", screen)
	}
	if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode after the run: %v %v", info, err)
	}
}

// The projects panel creates a project at the current repository: Project.toml, registry and the session.project frame.
func TestLiveProjectsPanelCreatesAProjectAndTheFrameNamesIt(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	slug := project.SlugFromName(filepath.Base(root))
	s := startTUI(t, root)
	s.openHub()
	s.typeSlowly("projects")
	s.keys("Enter")
	panel := s.waitFor("+ New project here", 10*time.Second)
	if !strings.Contains(panel, "Here: global") {
		t.Fatalf("the panel must state the current recognition:\n%s", panel)
	}
	s.capture("projects-panel")
	s.keys("Enter")
	form := s.waitFor("Slug (lowercase, kebab) (1/3)", 10*time.Second)
	if !strings.Contains(form, slug) {
		t.Fatalf("the slug is not prefilled from the directory name:\n%s", form)
	}
	s.capture("projects-form")
	s.keys("Enter")
	s.waitFor("Name (optional) (2/3)", 10*time.Second)
	s.typeSlowly("Panel Demo")
	s.keys("Enter")
	s.waitFor("Description (optional) (3/3)", 10*time.Second)
	s.keys("Enter")
	done := s.waitFor("Created project", 15*time.Second)
	if !strings.Contains(done, slug) {
		t.Fatalf("the list does not show the new project:\n%s", done)
	}
	loaded, found, err := project.Load(root)
	if err != nil || !found || loaded.Slug != slug || loaded.Name != "Panel Demo" {
		t.Fatalf("Project.toml wrong: %+v %v %v", loaded, found, err)
	}
	registry, err := os.ReadFile(filepath.Join(root, "agent", "projects", loaded.ID, "project.toml"))
	if err != nil || !strings.Contains(string(registry), "Panel Demo") {
		t.Fatalf("project not registered: %v\n%s", err, registry)
	}
	s.keys("Escape", "Escape")
	s.typeSlowly("hello")
	s.tmux("send-keys", "-t", "wb", "Enter")
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s.recorder.mu.Lock()
		bodies := strings.Join(s.recorder.bodies, "\n")
		s.recorder.mu.Unlock()
		if strings.Contains(bodies, loaded.ID) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the session.project frame never named %s", loaded.ID)
}

// Selecting a registered project in the panel and joining by marker writes only git config and changes recognition.
func TestLiveProjectsPanelJoinsARegisteredProjectByMarker(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "elsewhere")
	gitInit(t, root)
	gitInit(t, other)
	registry, err := project.Open(filepath.Join(root, "agent"))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := registry.CreateProject(t.Context(), other, "shared-thing", "Shared Thing", "")
	if err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	s.openHub()
	s.typeSlowly("projects")
	s.keys("Enter")
	s.waitFor("shared-thing", 10*time.Second)
	s.keys("Down", "Enter")
	choice := s.waitFor("Join by local marker", 10*time.Second)
	s.capture("projects-join")
	if !strings.Contains(choice, "Join by file") {
		t.Fatalf("both join modes must be offered:\n%s", choice)
	}
	s.keys("Down", "Enter")
	s.waitFor("Joined", 15*time.Second)
	out, err := exec.Command("git", "-C", root, "config", "--local", "pig.projectId").Output()
	if err != nil || strings.TrimSpace(string(out)) != entry.Project.ID {
		t.Fatalf("marker not written: %q %v", out, err)
	}
	if _, found, _ := project.Load(root); found {
		t.Fatal("joining by marker must not write Project.toml")
	}
}

// Joining the project this checkout already declares (just created here) completes instead of looping on a raw file error.
func TestLiveProjectsPanelJoinOfTheProjectDeclaredHereCompletes(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	s := startTUI(t, root)
	s.openHub()
	s.typeSlowly("projects")
	s.keys("Enter")
	s.waitFor("+ New project here", 10*time.Second)
	s.keys("Enter")
	s.waitFor("Slug (lowercase, kebab) (1/3)", 10*time.Second)
	s.keys("Enter")
	s.waitFor("Name (optional) (2/3)", 10*time.Second)
	s.keys("Enter")
	s.waitFor("Description (optional) (3/3)", 10*time.Second)
	s.keys("Enter")
	s.waitFor("Created project", 15*time.Second)
	before, err := os.ReadFile(filepath.Join(root, ".project", "Project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s.keys("Down", "Enter")
	s.waitFor("Join by file", 10*time.Second)
	s.keys("Enter")
	done := s.waitFor("already declares it", 15*time.Second)
	if strings.Contains(done, "Component slug") || strings.Contains(done, "file exists") {
		t.Fatalf("the flow must complete without a form or a raw error:\n%s", done)
	}
	s.capture("projects-join-declared-here")
	after, _ := os.ReadFile(filepath.Join(root, ".project", "Project.toml"))
	if string(before) != string(after) {
		t.Fatal("the join rewrote the Project.toml")
	}
}

// Joining another project over an existing Project.toml explains itself in full, keeps the typed value and lets the user leave.
func TestLiveProjectsPanelJoinOverAnotherProjectFileIsRefusedReadably(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(t.TempDir(), "elsewhere")
	gitInit(t, root)
	gitInit(t, other)
	registry, err := project.Open(filepath.Join(root, "agent"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.CreateProject(t.Context(), root, "home-base", "Home", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.CreateProject(t.Context(), other, "zz-other", "Other", ""); err != nil {
		t.Fatal(err)
	}
	s := startTUI(t, root)
	s.openHub()
	s.typeSlowly("projects")
	s.keys("Enter")
	s.waitFor("zz-other", 10*time.Second)
	// The panel lists projects in registry order, which follows the random ids; walk to the other project's row.
	entries, err := registry.List()
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range entries {
		if entry.Project.Slug == "zz-other" {
			for range i + 1 {
				s.keys("Down")
			}
		}
	}
	s.keys("Enter")
	s.waitFor("Join by file", 10*time.Second)
	s.keys("Enter")
	s.waitFor("Component slug of this repository (1/1)", 10*time.Second)
	s.keys("Enter")
	refused := s.waitFor("never replaced", 15*time.Second)
	if !strings.Contains(refused, "home-base") || strings.Contains(refused, "link ") {
		t.Fatalf("the refusal must name the declared project and no raw error:\n%s", refused)
	}
	if !strings.Contains(refused, "Component slug of this repository (1/1)") || !strings.Contains(refused, filepath.Base(root)) {
		t.Fatalf("the form must stay with the typed value:\n%s", refused)
	}
	s.capture("projects-join-refused")
	s.keys("Escape")
	s.waitFor("Join by local marker", 10*time.Second)
	s.keys("Escape")
	s.waitFor("+ New project here", 10*time.Second)
}

// A repository in no project is offered a project once; a second session in it stays quiet and nothing is created.
func TestLiveRepositoryInNoProjectIsOfferedAProjectOnce(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	s := startTUI(t, root)
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(s.history(), "belongs to no project yet") {
		if time.Now().After(deadline) {
			t.Fatalf("no offer:\n%s", s.history())
		}
		time.Sleep(200 * time.Millisecond)
	}
	if _, found, _ := project.Load(root); found {
		t.Fatal("the offer must never create a project")
	}
	second := startTUI(t, root)
	time.Sleep(3 * time.Second)
	if strings.Contains(second.history(), "belongs to no project yet") {
		t.Fatal("the same repository was offered a second time")
	}
}

// With the offer switched off, a repository in no project starts quietly and keeps its one offer for later.
func TestLiveProjectOfferIsSuppressedByItsSetting(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	config := filepath.Join(root, "agent", "Workbench.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[projects]\noffer-registration = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	quiet := startTUI(t, root)
	time.Sleep(3 * time.Second)
	if strings.Contains(quiet.history(), "belongs to no project yet") {
		t.Fatal("the offer appeared although projects.offer-registration is false")
	}
	if err := os.WriteFile(config, []byte("[projects]\noffer-registration = true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	offered := startTUI(t, root)
	offered.waitFor("belongs to no project yet", 15*time.Second)
}

func TestLiveUserBashListsASchemeDirectoryAndRunsACommandWithAURL(t *testing.T) {
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "fetched-by-user-curl") }))
	defer web.Close()
	s := startTUI(t, t.TempDir())
	s.typeText("!ls session://self/")
	s.keys("Enter")
	if screen := s.waitFor("system-prompt", 15*time.Second); strings.Contains(screen, "No such file") {
		t.Fatalf("the scheme directory went to the shell:\n%s", screen)
	}
	s.typeText("!curl -s " + web.URL + "/page")
	s.keys("Enter")
	s.waitFor("fetched-by-user-curl", 15*time.Second)
	s.typeText("!cat cfg://workbench/status | wc -c | sed 's/^/bytes=/'")
	s.keys("Enter")
	if screen := s.waitFor("bytes=", 15*time.Second); strings.Contains(screen, "bytes=0\n") {
		t.Fatalf("the scheme path was not passed as a file:\n%s", screen)
	}
}
