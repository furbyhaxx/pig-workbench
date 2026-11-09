//go:build integration && liveeval

package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// toolTask is one fixed task for tool-text tuning. Held-out tasks never drive a text change.
type toolTask struct {
	name    string
	holdout bool
	files   map[string]string
	prompt  string
	check   func(work string, r turnResult) error
}

var shellReaders = regexp.MustCompile(`^\s*(cat|head|tail|sed|awk|grep|rg|find|ls|tree|tee|echo\s+.*>)\b`)

func numbered(n int, format string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, format, i)
	}
	return b.String()
}

var toolTasks = []toolTask{
	{
		name: "edit-three-sites",
		files: map[string]string{"cfg.py": "import os\n\n" +
			"def connect():\n    timeout = 30\n    return timeout\n\n" +
			"def read():\n    retries = 3\n    return retries\n\n" +
			"def write():\n    buffer = 4096\n    return buffer\n"},
		prompt: "In cfg.py set timeout to 60, retries to 5 and buffer to 8192. Change nothing else.",
		check: func(work string, _ turnResult) error {
			got := abRead(work, "cfg.py")
			want := "import os\n\ndef connect():\n    timeout = 60\n    return timeout\n\ndef read():\n    retries = 5\n    return retries\n\ndef write():\n    buffer = 8192\n    return buffer\n"
			if got != want {
				return fmt.Errorf("cfg.py = %q", got)
			}
			return nil
		},
	},
	{
		name:   "edit-insert-function",
		files:  map[string]string{"util.go": "package util\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\nfunc Sub(a, b int) int {\n\treturn a - b\n}\n"},
		prompt: "In util.go add a function Mul(a, b int) int that returns a*b, placed between Add and Sub. Keep everything else byte-identical.",
		check: func(work string, _ turnResult) error {
			got := abRead(work, "util.go")
			add, mul, sub := strings.Index(got, "func Add"), strings.Index(got, "func Mul"), strings.Index(got, "func Sub")
			if add < 0 || mul < add || sub < mul || !strings.Contains(got, "a * b") && !strings.Contains(got, "a*b") {
				return fmt.Errorf("util.go = %q", got)
			}
			if !strings.Contains(got, "return a + b\n}") || !strings.Contains(got, "return a - b\n}") {
				return fmt.Errorf("existing functions changed: %q", got)
			}
			return nil
		},
	},
	{
		name:   "background-job",
		files:  map[string]string{},
		prompt: "Start the command `sleep 4 && echo ready > flag.txt` as a background job so you stay responsive. When it has finished, tell me exactly what flag.txt contains. Answer with just that content.",
		check: func(work string, r turnResult) error {
			if abRead(work, "flag.txt") != "ready\n" || !strings.Contains(strings.ToLower(r.text), "ready") {
				return fmt.Errorf("flag=%q answer=%q", abRead(work, "flag.txt"), r.text)
			}
			if !r.succeeded("bash", "background") && !r.succeeded("bash", "yield_seconds") {
				return fmt.Errorf("the command was not run as a background job")
			}
			return nil
		},
	},
	{
		name:   "noisy-output",
		files:  map[string]string{"noisy.sh": "#!/bin/sh\ni=1\nwhile [ $i -le 5000 ]; do\n  echo \"row $i padding padding padding\"\n  if [ $i -eq 2750 ]; then echo \"NEEDLE=42\"; fi\n  i=$((i+1))\ndone\n"},
		prompt: "Run `sh noisy.sh`. It prints a line NEEDLE=<number> somewhere in its output. What is the number? Answer with just the number.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(r.text, "42") {
				return fmt.Errorf("answer %q lacks 42", r.text)
			}
			return nil
		},
	},
	{
		name: "grep-files",
		files: func() map[string]string {
			m := map[string]string{}
			for i := 1; i <= 12; i++ {
				body := fmt.Sprintf("module m%d\n// ordinary file\n", i)
				if i == 3 || i == 7 || i == 11 {
					body += "// this API is deprecated\n"
				}
				m[fmt.Sprintf("src/m%d.txt", i)] = body
			}
			return m
		}(),
		prompt: "Which files under src mention the word deprecated? Reply with only the file names, comma separated.",
		check: func(_ string, r turnResult) error {
			for _, want := range []string{"m3.txt", "m7.txt", "m11.txt"} {
				if !strings.Contains(r.text, want) {
					return fmt.Errorf("answer %q lacks %s", r.text, want)
				}
			}
			if strings.Count(r.text, ".txt") != 3 {
				return fmt.Errorf("answer %q names the wrong number of files", r.text)
			}
			return nil
		},
	},
	{
		name:   "multi-read-sum",
		files:  map[string]string{"a.cfg": "value=11\n", "b.cfg": "value=22\n", "c.cfg": "value=33\n"},
		prompt: "Read a.cfg, b.cfg and c.cfg and tell me the sum of their value fields. Answer with just the number.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(r.text, "66") {
				return fmt.Errorf("answer %q lacks 66", r.text)
			}
			return nil
		},
	},
	{
		name:   "cd-relative",
		files:  map[string]string{"sub/deep/settings.ini": "[net]\nport=8123\n", "settings.ini": "[net]\nport=1\n"},
		prompt: "Change your working location to sub/deep, then read settings.ini using a relative path from there. What is the port? Answer with just the number.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(r.text, "8123") {
				return fmt.Errorf("answer %q lacks 8123", r.text)
			}
			if !r.succeeded("cd", "sub/deep") {
				return fmt.Errorf("cd was not used")
			}
			return nil
		},
	},
	{
		name:   "create-in-new-dir",
		files:  map[string]string{},
		prompt: "Create the file docs/guide.md containing exactly the single line `# Guide` followed by a newline. The docs directory does not exist yet.",
		check: func(work string, _ turnResult) error {
			if abRead(work, "docs/guide.md") != "# Guide\n" {
				return fmt.Errorf("docs/guide.md = %q", abRead(work, "docs/guide.md"))
			}
			return nil
		},
	},
	{
		name:    "edit-two-functions",
		holdout: true,
		files:   map[string]string{"calc.py": "def double(x):\n    return x * 3\n\ndef square(x):\n    return x + x\n\ndef half(x):\n    return x / 2\n"},
		prompt:  "calc.py has two bugs: double should return x * 2 and square should return x * x. Fix both and leave half untouched.",
		check: func(work string, _ turnResult) error {
			want := "def double(x):\n    return x * 2\n\ndef square(x):\n    return x * x\n\ndef half(x):\n    return x / 2\n"
			if got := abRead(work, "calc.py"); got != want {
				return fmt.Errorf("calc.py = %q", got)
			}
			return nil
		},
	},
	{
		name:    "background-output",
		holdout: true,
		files:   map[string]string{"tick.sh": "#!/bin/sh\nfor i in 1 2 3 4 5 6; do echo \"tick $i\"; sleep 1; done\necho final-token-ZEBRA\n"},
		prompt:  "Run `sh tick.sh` in the background. When it is done, tell me the last line it printed. Answer with just that line.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(r.text, "final-token-ZEBRA") {
				return fmt.Errorf("answer %q lacks the last line", r.text)
			}
			return nil
		},
	},
	{
		name:    "grep-line-number",
		holdout: true,
		files: map[string]string{
			"a/one.txt":   numbered(40, "alpha %d\n"),
			"a/two.txt":   numbered(25, "beta %d\n") + "release v2.19.4 is current\n" + numbered(5, "gamma %d\n"),
			"b/three.txt": numbered(60, "delta %d\n"),
		},
		prompt: "Which file and line number contain a version string of the form v2.<digits>.<digits>? Answer as path:line.",
		check: func(_ string, r turnResult) error {
			if !strings.Contains(r.text, "two.txt") || !strings.Contains(r.text, "26") {
				return fmt.Errorf("answer %q is not two.txt:26", r.text)
			}
			return nil
		},
	},
}

type toolRun struct {
	Label      string   `json:"label"`
	Arm        string   `json:"arm"`
	Task       string   `json:"task"`
	Holdout    bool     `json:"holdout"`
	Rep        int      `json:"rep"`
	Pass       bool     `json:"pass"`
	Failure    string   `json:"failure,omitempty"`
	ToolCalls  int      `json:"toolCalls"`
	ToolErrors int      `json:"toolErrors"`
	ShellReads int      `json:"shellReads"`
	Sequence   []string `json:"sequence"`
	Errors     []string `json:"errors,omitempty"`
	Input      int      `json:"input"`
	Output     int      `json:"output"`
	CacheRead  int      `json:"cacheRead"`
	Seconds    float64  `json:"seconds"`
}

// TestLiveEvalToolText runs the tool-text suite under both prompt deliveries. LIVE_LABEL
// names the text revision being measured; LIVE_REPS and LIVE_TASKS narrow the run.
func TestLiveEvalToolText(t *testing.T) {
	requireLiveKey(t)
	label := os.Getenv("LIVE_LABEL")
	if label == "" {
		label = "unlabelled"
	}
	reps := 3
	if v := os.Getenv("LIVE_REPS"); v != "" {
		fmt.Sscan(v, &reps)
	}
	only := os.Getenv("LIVE_TASKS")
	var runs []toolRun
	for _, arm := range []struct{ name, delivery string }{{"pig-prompt", "pig"}, {"workbench-prompt", "force"}} {
		if v := os.Getenv("LIVE_ARM"); v != "" && v != arm.name {
			continue
		}
		for _, task := range toolTasks {
			if only != "" && !strings.Contains(","+only+",", ","+task.name+",") {
				continue
			}
			for rep := 1; rep <= reps; rep++ {
				root, work := t.TempDir(), t.TempDir()
				writeFiles(t, work, task.files)
				started := time.Now()
				s := startEvalSession(t, root, work, arm.delivery)
				r := s.turn("task", task.prompt, func(map[string]any) string { return "" })
				run := toolRun{Label: label, Arm: arm.name, Task: task.name, Holdout: task.holdout, Rep: rep, ToolCalls: len(r.calls), Input: s.input, Output: s.output, CacheRead: s.cacheRead, Seconds: time.Since(started).Seconds()}
				for _, call := range r.calls {
					run.Sequence = append(run.Sequence, call.name)
					if call.isError {
						run.ToolErrors++
						run.Errors = append(run.Errors, call.name+": "+truncateText(call.text, 220))
					}
					if call.name == "bash" {
						var args struct {
							Command string `json:"command"`
						}
						_ = json.Unmarshal([]byte(call.args), &args)
						if shellReaders.MatchString(args.Command) {
							run.ShellReads++
						}
					}
				}
				if err := task.check(work, r); err != nil {
					run.Failure = err.Error()
				} else {
					run.Pass = true
				}
				s.save(fmt.Sprintf("tools-%s-%s-%s-%d.jsonl", label, arm.name, task.name, rep))
				s.f.close()
				runs = append(runs, run)
				t.Logf("%s %s %s #%d pass=%v calls=%d errors=%d shellReads=%d %s %s", label, arm.name, task.name, rep, run.Pass, run.ToolCalls, run.ToolErrors, run.ShellReads, strings.Join(run.Sequence, ">"), run.Failure)
			}
		}
	}
	summarizeTools(t, label, runs)
}

func truncateText(text string, n int) string {
	if len(text) > n {
		return text[:n]
	}
	return text
}

func summarizeTools(t *testing.T, label string, runs []toolRun) {
	type total struct{ n, pass, calls, errors, shell, in, out int }
	groups := map[string]*total{}
	add := func(key string, run toolRun) {
		g := groups[key]
		if g == nil {
			g = &total{}
			groups[key] = g
		}
		g.n++
		if run.Pass {
			g.pass++
		}
		g.calls += run.ToolCalls
		g.errors += run.ToolErrors
		g.shell += run.ShellReads
		g.in += run.Input
		g.out += run.Output
	}
	for _, run := range runs {
		set := "tune"
		if run.Holdout {
			set = "holdout"
		}
		add(run.Arm+"/"+set, run)
		add(run.Arm+"/all", run)
	}
	var keys []string
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := groups[key]
		t.Logf("SUMMARY %s %-26s runs=%d pass=%d calls=%d toolErrors=%d shellReads=%d input=%d output=%d", label, key, g.n, g.pass, g.calls, g.errors, g.shell, g.in, g.out)
	}
	if out := os.Getenv("LIVE_EVAL_OUT"); out != "" {
		_ = os.MkdirAll(out, 0755)
		encoded, _ := json.MarshalIndent(runs, "", "  ")
		if err := os.WriteFile(filepath.Join(out, "tools-"+label+".json"), encoded, 0644); err != nil {
			t.Error(err)
		}
	}
}
