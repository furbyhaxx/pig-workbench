//go:build integration && liveeval

package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workflowProject(t *testing.T) string {
	work := t.TempDir()
	writeFiles(t, work, map[string]string{
		"config.txt":            "mode=slow\nlevel=3\n",
		"notes.txt":             "TODO: write docs\nkeep this\nTODO: add tests\n",
		"src/main.go":           "package main\n\n// TODO: handle errors\nfunc main() {}\n",
		".project/README.md":    "# fixture artifact tree\n",
		".project/plans/one.md": "# plan one\n",
		"AGENTS.md":             "Fixture project. Keep answers short.\n",
	})
	return work
}

// TestLiveEvalWorkflow drives a real model through every tool, the core schemes and ask_user.
// Each step is judged from disk, events and tool arguments, never from the model's own report.
func TestLiveEvalWorkflow(t *testing.T) {
	requireLiveKey(t)
	root := t.TempDir()
	work := workflowProject(t)
	s := startEvalSession(t, root, work, "force")
	defer s.save("workflow.jsonl")
	noDialog := func(map[string]any) string { t.Fatal("unexpected dialog"); return "" }

	t.Run("search-and-read", func(t *testing.T) {
		r := s.turn("t1", "Use the ls tool on the current directory, the find tool for *.txt files, the grep tool to search the whole project for the word TODO, and the read tool on src/main.go. Then answer with only the number of TODO lines you found.", noDialog)
		for _, tool := range []string{"ls", "find", "grep", "read"} {
			if !r.succeeded(tool) {
				t.Errorf("no successful %s call:\n%s", tool, r.startArgs)
			}
		}
		if !strings.Contains(r.text, "3") {
			t.Errorf("expected the count 3, got %q", r.text)
		}
	})

	t.Run("edit-and-write", func(t *testing.T) {
		r := s.turn("t2", "Use the read tool on config.txt, then the edit tool to change mode=slow to mode=fast, then the write tool to create result.txt containing exactly: done", noDialog)
		for _, tool := range []string{"read", "edit", "write"} {
			if !r.succeeded(tool) {
				t.Errorf("no successful %s call:\n%s", tool, r.startArgs)
			}
		}
		config, _ := os.ReadFile(filepath.Join(work, "config.txt"))
		if string(config) != "mode=fast\nlevel=3\n" {
			t.Errorf("config.txt = %q", config)
		}
		result, _ := os.ReadFile(filepath.Join(work, "result.txt"))
		if strings.TrimSpace(string(result)) != "done" {
			t.Errorf("result.txt = %q", result)
		}
	})

	t.Run("bash-jobs-cd", func(t *testing.T) {
		r := s.turn("t3", "Use the bash tool to run: echo bash-ok > bash.txt . Then use the bash tool with background=true to run: sleep 2; echo job-ok > job.txt . Then use the jobs tool to wait for that job to finish. Finally use the cd tool to move into the src directory.", noDialog)
		for _, tool := range []string{"bash", "jobs", "cd"} {
			if !r.succeeded(tool) {
				t.Errorf("no successful %s call:\n%s", tool, r.startArgs)
			}
		}
		for name, want := range map[string]string{"bash.txt": "bash-ok", "job.txt": "job-ok"} {
			data, _ := os.ReadFile(filepath.Join(work, name))
			if strings.TrimSpace(string(data)) != want {
				t.Errorf("%s = %q, want %q", name, data, want)
			}
		}
		if !strings.Contains(r.startArgs, "src") {
			t.Errorf("cd did not name src: %s", r.startArgs)
		}
	})

	t.Run("ask-user", func(t *testing.T) {
		answered := false
		r := s.turn("t4", "Use the ask_user tool to ask me which color I prefer, with the choices red and blue. After I answer, use the write tool to create color.txt in the project root (an absolute path under the original project directory "+work+") containing only the chosen color.", func(event map[string]any) string {
			answered = true
			if options, ok := event["options"].([]any); ok {
				for _, option := range options {
					if strings.Contains(strings.ToLower(asString(option)), "blue") {
						return asString(option)
					}
				}
				if len(options) > 0 {
					return asString(options[0])
				}
			}
			return "blue"
		})
		if !answered || r.dialogs == 0 {
			t.Fatal("ask_user never opened a dialog")
		}
		for _, tool := range []string{"ask_user", "write"} {
			if !r.succeeded(tool) {
				t.Errorf("no successful %s call:\n%s", tool, r.startArgs)
			}
		}
		data, _ := os.ReadFile(filepath.Join(work, "color.txt"))
		if !strings.Contains(strings.ToLower(string(data)), "blue") {
			t.Errorf("color.txt = %q", data)
		}
	})

	t.Run("core-schemes", func(t *testing.T) {
		r := s.turn("t5", "Use the read tool to read each of these URIs, one call each: cfg://workbench/status , docs://workbench/ , session://self , artifact:// , skill:// . Use the write tool to create local://note.txt containing scratch-ok, then read local://note.txt back. Answer with one word: done.", noDialog)
		for _, uri := range []string{"cfg://workbench/status", "docs://workbench/", "session://self", "artifact://", "skill://"} {
			if !r.succeeded("read", uri) {
				t.Errorf("no successful read of %s:\n%s", uri, r.startArgs)
			}
		}
		if !r.succeeded("write", "local://note.txt") {
			t.Errorf("write to local://note.txt did not succeed:\n%s", r.startArgs)
		}
		if !r.succeeded("read", "local://note.txt") {
			t.Errorf("read of local://note.txt did not succeed:\n%s", r.startArgs)
		}
	})

	t.Logf("tools used: %v; tokens in=%d out=%d cacheRead=%d", s.tools, s.input, s.output, s.cacheRead)
	if missing := hasAll(toolKeys(s.tools), "read", "write", "edit", "grep", "find", "ls", "bash", "jobs", "cd", "ask_user"); len(missing) > 0 {
		t.Errorf("tools never used across the workflow: %v", missing)
	}
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

func toolKeys(values map[string]int) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	return out
}
