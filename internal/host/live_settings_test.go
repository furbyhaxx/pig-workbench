//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func settingsNotice(t *testing.T, f *rpcFixture, id, args string) map[string]any {
	t.Helper()
	f.send(id, "prompt", "/workbench settings "+args)
	event := f.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), `"settings"`)
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(event["message"].(string)), &result); err != nil {
		t.Fatal(err)
	}
	f.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == id })
	return result
}

func TestLiveSettingsScopesAndReload(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	project := filepath.Join(root, ".pig")
	for _, dir := range []string{agent, project, filepath.Join(root, "next")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[read]\nmax-lines = 100\nmax-bytes = 4096\n"), 0600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(project, "Workbench.toml")
	if err := os.WriteFile(projectPath, []byte("[read]\nmax-lines = 40\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := startRPC(t, root, "--approve")
	check := func(f *rpcFixture, id string, lines float64, scope string) {
		t.Helper()
		output := settingsNotice(t, f, id, "show read.max-lines")
		entry := output["entry"].(map[string]any)
		if entry["value"] != lines || entry["scope"] != scope {
			t.Fatalf("wrong effective value/source: %+v", output)
		}
	}
	check(f, "initial", 40, "project")
	saved := settingsNotice(t, f, "runtime", "set runtime read.max-lines 10")["result"].(map[string]any)
	if saved["saved"] != true || saved["published"] != true {
		t.Fatalf("runtime change did not publish: %+v", saved)
	}
	check(f, "runtime-show", 10, "runtime")
	settingsNotice(t, f, "reset", "reset runtime read.max-lines")
	check(f, "inherited", 40, "project")
	settingsNotice(t, f, "user-save", "set user read.max-bytes 2048")
	f.command("cd", "/cd next")
	check(f, "after-cd", 40, "project")
	if err := os.WriteFile(projectPath, []byte("[read]\nmax-lines = 30\n"), 0600); err != nil {
		t.Fatal(err)
	}
	settingsNotice(t, f, "reload", "reload")
	check(f, "reloaded", 30, "project")
	settingsNotice(t, f, "temporary", "set runtime read.max-lines 7")
	f.close()
	untrusted := startRPC(t, root, "--no-approve")
	check(untrusted, "ignored-project", 100, "user")
	value := settingsNotice(t, untrusted, "persisted-user", "show read.max-bytes")["entry"].(map[string]any)
	if value["value"] != float64(2048) || value["scope"] != "user" {
		t.Fatalf("user save was not persisted: %+v", value)
	}
}
