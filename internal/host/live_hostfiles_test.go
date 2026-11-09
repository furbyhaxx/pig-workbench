//go:build integration

package host

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func hostFilesFixture(t *testing.T, mode os.FileMode) (root, systemDir string) {
	t.Helper()
	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	systemDir = filepath.Join(root, "etc-agents")
	if err := os.Mkdir(systemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"AGENTS.md": "SYSTEM-RULE-MARKER never reboot this host.\n",
		"HOST.md":   "HOST-FACT-MARKER Debian 13 with 64 GiB.\n",
	} {
		path := filepath.Join(systemDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "AGENTS.md"), []byte("USER-RULE-MARKER prefer small commits.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("PROJECT-RULE-MARKER run just check.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, systemDir
}

func TestLiveHostFilesAreDeliveredOnceAboveUserSteeringAndUpdateAsADiff(t *testing.T) {
	root, systemDir := hostFilesFixture(t, 0o644)
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "a.txt"}},
		{"bash", map[string]any{"command": fmt.Sprintf("sed -i 's/Debian 13 with 64 GiB./Debian 13 with 128 GiB./' %s/HOST.md", systemDir)}},
		{"read", map[string]any{"path": "a.txt"}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	settings := fmt.Sprintf("[instructions]\nsystem-dir = %q\n", systemDir)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--approve", "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 4 {
		t.Fatalf("got %d requests, want 4", len(recorder.bodies))
	}
	first := recorder.bodies[0]
	system, user, project := strings.Index(first, "SYSTEM-RULE-MARKER"), strings.Index(first, "USER-RULE-MARKER"), strings.Index(first, "PROJECT-RULE-MARKER")
	if system < 0 || user < 0 || project < 0 || !(system < user && user < project) {
		t.Fatalf("host steering must precede user and project steering (system=%d user=%d project=%d)", system, user, project)
	}
	if !strings.Contains(first, "HOST-FACT-MARKER") || !strings.Contains(first, `src=\"instructions.host\"`) {
		t.Fatalf("HOST.md must arrive as an instructions.host frame")
	}
	last := recorder.bodies[3]
	if got := strings.Count(last, "SYSTEM-RULE-MARKER"); got != 1 {
		t.Fatalf("host steering delivered %d times, want once", got)
	}
	if got := strings.Count(last, `src=\"instructions.host\"`); got != 2 || !strings.Contains(last, "128 GiB") {
		t.Fatalf("the HOST.md change must arrive once as a second host frame (frames=%d)", got)
	}
	if strings.Contains(last, "no longer applies") {
		t.Fatalf("host files must never be withdrawn")
	}
}

func TestLiveHostFilesWritableByOthersAreRefusedWithANotice(t *testing.T) {
	root, systemDir := hostFilesFixture(t, 0o666)
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	settings := fmt.Sprintf("[instructions]\nsystem-dir = %q\n", systemDir)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--approve", "--model", "workbench-fixture/mock")
	fixture.send("go", "prompt", "Go.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	body := recorder.bodies[0]
	if strings.Contains(body, "SYSTEM-RULE-MARKER") || strings.Contains(body, "HOST-FACT-MARKER") {
		t.Fatalf("a file writable by others must not reach the model")
	}
	if !strings.Contains(body, "Host file not loaded") || !strings.Contains(body, "USER-RULE-MARKER") {
		t.Fatalf("the refusal must be visible and the other instructions unaffected")
	}
}

func TestLiveHostFilesSurviveCompactionExactlyOnce(t *testing.T) {
	root, systemDir := hostFilesFixture(t, 0o644)
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := fmt.Sprintf("[instructions]\nsystem-dir = %q\n", systemDir)
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	for i, prompt := range []string{"first", "second"} {
		f.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
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
	for _, marker := range []string{"SYSTEM-RULE-MARKER", "HOST-FACT-MARKER"} {
		if got := strings.Count(last, marker); got != 1 {
			t.Fatalf("%s appears %d times after compaction, want exactly 1", marker, got)
		}
	}
}
