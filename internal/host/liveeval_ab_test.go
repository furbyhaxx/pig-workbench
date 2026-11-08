//go:build integration && liveeval

package host

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// abTask is one fixed task with an objective check judged from disk and tool events.
type abTask struct {
	name    string
	files   map[string]string
	prompt  string
	check   func(work string, r turnResult) error
	expects []string
}

func abRead(work, name string) string {
	data, _ := os.ReadFile(filepath.Join(work, name))
	return string(data)
}

var abTasks = []abTask{
	{
		name: "fix-bug",
		files: map[string]string{
			"stats.py": "def average(values):\n    total = 0\n    for v in values:\n        total += v\n    return total / (len(values) + 1)\n\nif __name__ == '__main__':\n    print(average([2, 4, 6]))\n",
		},
		prompt: "stats.py prints the wrong average for [2, 4, 6] (it should print 4.0). Find the bug and fix it in the file. Do not change anything else.",
		check: func(work string, _ turnResult) error {
			out, err := exec.Command("python3", filepath.Join(work, "stats.py")).Output()
			if err != nil || strings.TrimSpace(string(out)) != "4.0" {
				return fmt.Errorf("stats.py prints %q (%v)", out, err)
			}
			return nil
		},
	},
	{
		name: "rename-across-files",
		files: map[string]string{
			"a.txt": "call fetchUser(1)\n",
			"b.txt": "def fetchUser(id): pass\n",
			"c.txt": "# uses fetchUser twice: fetchUser\n",
			"d.txt": "unrelated\n",
		},
		prompt: "Rename the identifier fetchUser to loadUser in every file of this directory that contains it.",
		check: func(work string, _ turnResult) error {
			for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
				if strings.Contains(abRead(work, name), "fetchUser") {
					return fmt.Errorf("%s still contains fetchUser", name)
				}
			}
			if strings.Count(abRead(work, "c.txt"), "loadUser") != 2 || !strings.Contains(abRead(work, "a.txt"), "loadUser(1)") || abRead(work, "d.txt") != "unrelated\n" {
				return fmt.Errorf("unexpected rename result: %q %q %q", abRead(work, "a.txt"), abRead(work, "c.txt"), abRead(work, "d.txt"))
			}
			return nil
		},
	},
	{
		name: "find-and-answer",
		files: map[string]string{
			"app/server.go": "package app\n\nfunc Serve() {}\nfunc Stop() {}\n",
			"app/client.go": "package app\n\nfunc Dial() {}\nfunc Close() {}\nfunc Retry() {}\n",
			"lib/util.go":   "package lib\n\nfunc Helper() {}\n",
		},
		prompt: "How many functions are declared in the app directory in total? Answer with just the number.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(r.text, "5") {
				return fmt.Errorf("answer %q lacks 5", r.text)
			}
			return nil
		},
	},
	{
		name:   "run-and-write",
		files:  map[string]string{"gen.sh": "#!/bin/sh\necho alpha\necho beta\n"},
		prompt: "Run gen.sh with the shell, and write its exact standard output into out.txt in this directory.",
		check: func(work string, _ turnResult) error {
			if abRead(work, "out.txt") != "alpha\nbeta\n" {
				return fmt.Errorf("out.txt = %q", abRead(work, "out.txt"))
			}
			return nil
		},
	},
	{
		name: "read-range",
		files: map[string]string{
			"big.txt": func() string {
				var b strings.Builder
				for i := 1; i <= 400; i++ {
					if i == 317 {
						b.WriteString("line 317 holds the secret word: PELICAN\n")
						continue
					}
					fmt.Fprintf(&b, "line %d filler\n", i)
				}
				return b.String()
			}(),
		},
		prompt: "big.txt is long. What secret word does line 317 hold? Answer with just the word.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(strings.ToUpper(r.text), "PELICAN") {
				return fmt.Errorf("answer %q lacks PELICAN", r.text)
			}
			return nil
		},
	},
}

type abRun struct {
	Arm        string  `json:"arm"`
	Task       string  `json:"task"`
	Rep        int     `json:"rep"`
	Pass       bool    `json:"pass"`
	Failure    string  `json:"failure,omitempty"`
	ToolCalls  int     `json:"toolCalls"`
	ToolErrors int     `json:"toolErrors"`
	Input      int     `json:"input"`
	Output     int     `json:"output"`
	CacheRead  int     `json:"cacheRead"`
	Seconds    float64 `json:"seconds"`
}

// TestLiveEvalPromptAB runs the fixed tasks under PiG's own prompt (delivery=auto) and
// builtin/workbench (delivery=force). Pass/fail is judged from disk, not from the model.
func TestLiveEvalPromptAB(t *testing.T) {
	requireLiveKey(t)
	reps := 3
	var runs []abRun
	for _, arm := range []struct{ name, delivery string }{{"pig-prompt", "auto"}, {"workbench-prompt", "force"}} {
		for _, task := range abTasks {
			for rep := 1; rep <= reps; rep++ {
				root := t.TempDir()
				work := t.TempDir()
				writeFiles(t, work, task.files)
				started := time.Now()
				s := startEvalSession(t, root, work, arm.delivery)
				r := s.turn("task", task.prompt, func(map[string]any) string { return "" })
				run := abRun{Arm: arm.name, Task: task.name, Rep: rep, ToolCalls: len(r.calls), Input: s.input, Output: s.output, CacheRead: s.cacheRead, Seconds: time.Since(started).Seconds()}
				for _, call := range r.calls {
					if call.isError {
						run.ToolErrors++
					}
				}
				if err := task.check(work, r); err != nil {
					run.Failure = err.Error()
				} else {
					run.Pass = true
				}
				s.save(fmt.Sprintf("ab-%s-%s-%d.jsonl", arm.name, task.name, rep))
				s.f.close()
				runs = append(runs, run)
				t.Logf("%s %s #%d pass=%v calls=%d errors=%d in=%d out=%d cacheRead=%d %.0fs %s", arm.name, task.name, rep, run.Pass, run.ToolCalls, run.ToolErrors, run.Input, run.Output, run.CacheRead, run.Seconds, run.Failure)
			}
		}
	}
	summarizeAB(t, runs)
}

func summarizeAB(t *testing.T, runs []abRun) {
	type total struct {
		n, pass, calls, errors, in, out, cache int
		seconds                                float64
	}
	byArm := map[string]*total{}
	for _, run := range runs {
		a := byArm[run.Arm]
		if a == nil {
			a = &total{}
			byArm[run.Arm] = a
		}
		a.n++
		if run.Pass {
			a.pass++
		}
		a.calls += run.ToolCalls
		a.errors += run.ToolErrors
		a.in += run.Input
		a.out += run.Output
		a.cache += run.CacheRead
		a.seconds += run.Seconds
	}
	var arms []string
	for arm := range byArm {
		arms = append(arms, arm)
	}
	sort.Strings(arms)
	for _, arm := range arms {
		a := byArm[arm]
		hit := 0.0
		if a.in+a.cache > 0 {
			hit = float64(a.cache) / float64(a.in+a.cache)
		}
		t.Logf("SUMMARY %-17s runs=%d pass=%d toolCalls=%d toolErrors=%d input=%d output=%d cacheRead=%d cacheHit=%.1f%% seconds=%.0f", arm, a.n, a.pass, a.calls, a.errors, a.in, a.out, a.cache, 100*hit, a.seconds)
	}
	if out := os.Getenv("LIVE_EVAL_OUT"); out != "" {
		encoded, _ := json.MarshalIndent(runs, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "ab-results.json"), encoded, 0644); err != nil {
			t.Error(err)
		}
	}
}

// TestLiveEvalServedPrompts records the exact system prompt each arm serves, without spending tokens.
func TestLiveEvalServedPrompts(t *testing.T) {
	for _, arm := range []struct{ name, delivery string }{{"pig-prompt", "auto"}, {"workbench-prompt", "force"}} {
		root := t.TempDir()
		recorder := &requestRecorder{}
		server := recorder.serve(t)
		defer server.Close()
		agent := filepath.Join(root, "agent")
		writeFixtureProvider(t, agent, server.URL)
		if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[prompt]\ndelivery = '"+arm.delivery+"'\n"), 0600); err != nil {
			t.Fatal(err)
		}
		f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock")
		f.send("go", "prompt", "hello")
		f.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
		recorder.mu.Lock()
		body := recorder.bodies[0]
		recorder.mu.Unlock()
		var request struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
			Tools []any `json:"tools"`
		}
		if err := json.Unmarshal([]byte(body), &request); err != nil {
			t.Fatal(err)
		}
		system := ""
		for _, m := range request.Messages {
			if m.Role == "system" || m.Role == "developer" {
				system += fmt.Sprint(m.Content)
			}
		}
		t.Logf("%s: system prompt %d bytes, %d tools", arm.name, len(system), len(request.Tools))
		if out := os.Getenv("LIVE_EVAL_OUT"); out != "" {
			_ = os.MkdirAll(out, 0755)
			text := strings.ReplaceAll(system, root, "$ROOT")
			if err := os.WriteFile(filepath.Join(out, "system-prompt-"+arm.name+".txt"), []byte(text), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
