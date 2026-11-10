//go:build integration

package host

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var liveJobID = regexp.MustCompile(`Job ([0-9a-f]{32}):`)

func bashStep(args map[string]any) scriptStep {
	return func([]string) scriptedCall { return scriptedCall{"bash", args} }
}

// jobStep calls jobs on the job that result index from named.
func jobStep(from int, args map[string]any) scriptStep {
	return func(results []string) scriptedCall {
		if match := liveJobID.FindStringSubmatch(results[from]); match != nil {
			args["id"] = match[1]
		}
		return scriptedCall{"jobs", args}
	}
}

// TestLiveYieldAfterSettingPromotesForegroundCommands pins jobs.yield-after-ms and its per-call override.
func TestLiveYieldAfterSettingPromotesForegroundCommands(t *testing.T) {
	results := runScriptedConfig(t, t.TempDir(), "bash,jobs", "[jobs]\nyield-after-ms = 300\n",
		bashStep(map[string]any{"command": "sleep 30"}),
		jobStep(0, map[string]any{"action": "kill"}),
		bashStep(map[string]any{"command": "sleep 0.8; echo waited", "yield_seconds": 0}),
	)
	if !strings.Contains(results[0], ": running") {
		t.Errorf("the configured delay did not promote the command: %q", results[0])
	}
	if !strings.Contains(results[2], "waited") || !strings.Contains(results[2], ": exited") {
		t.Errorf("yield_seconds 0 must wait for the exit: %q", results[2])
	}
	if plain := runScripted(t, t.TempDir(), "bash", bashStep(map[string]any{"command": "sleep 0.8; echo waited"})); !strings.Contains(plain[0], "waited") {
		t.Errorf("without the setting a foreground command is never promoted: %q", plain[0])
	}
}

// TestLiveTerminationGraceSettingBoundsAStubbornJob pins jobs.termination-grace-ms.
func TestLiveTerminationGraceSettingBoundsAStubbornJob(t *testing.T) {
	stubborn := map[string]any{"command": "trap '' TERM; echo ready; while :; do sleep 0.05; done", "background": true}
	measure := func(config string) time.Duration {
		var asked, answered time.Time
		runScriptedConfig(t, t.TempDir(), "bash,jobs", config,
			bashStep(stubborn),
			jobStep(0, map[string]any{"action": "wait", "until": "output", "timeout_seconds": 10}),
			func(results []string) scriptedCall {
				asked = time.Now()
				return jobStep(0, map[string]any{"action": "kill"})(results)
			},
			func([]string) scriptedCall {
				answered = time.Now()
				return scriptedCall{"jobs", map[string]any{"action": "list"}}
			},
		)
		return answered.Sub(asked)
	}
	short, long := measure("[jobs]\ntermination-grace-ms = 100\n"), measure("[jobs]\ntermination-grace-ms = 1500\n")
	if short >= 1200*time.Millisecond || long < 1500*time.Millisecond {
		t.Fatalf("kill took %v with 100 ms of grace and %v with 1500 ms", short, long)
	}
}

// TestLiveMaxFinishedSettingBoundsRetainedJobs pins jobs.max-finished.
func TestLiveMaxFinishedSettingBoundsRetainedJobs(t *testing.T) {
	steps := []scriptStep{}
	for i := range 4 {
		steps = append(steps, bashStep(map[string]any{"command": "true", "background": true}), jobStep(2*i, map[string]any{"action": "wait", "timeout_seconds": 10}))
	}
	steps = append(steps, bashStep(map[string]any{"command": "true"}), func([]string) scriptedCall { return scriptedCall{"jobs", map[string]any{"action": "list"}} })
	results := runScriptedConfig(t, t.TempDir(), "bash,jobs", "[jobs]\nmax-finished = 2\n", steps...)
	list := results[len(results)-1]
	if strings.Count(list, `"id":`) != 3 || !strings.Contains(list, `"omitted":2`) {
		t.Fatalf("want the 2 newest background jobs and 1 foreground command, 2 omitted: %s", list)
	}
}

// TestLiveLogByteSettingsBoundRetainedOutput pins jobs.log-bytes and jobs.session-log-bytes.
func TestLiveLogByteSettingsBoundRetainedOutput(t *testing.T) {
	noisy := map[string]any{"command": "head -c 5000 /dev/zero | tr '\\0' x; echo; echo tail-marker"}
	results := runScriptedConfig(t, t.TempDir(), "bash,jobs", "[jobs]\nlog-bytes = 2048\nsession-log-bytes = 3072\n",
		bashStep(noisy),
		bashStep(map[string]any{"command": "head -c 2000 /dev/zero | tr '\\0' y; echo second"}),
		jobStep(0, map[string]any{"action": "output"}),
	)
	if kept := strings.Count(results[0], "x"); kept > 2048 || kept < 1024 || !strings.Contains(results[0], "tail-marker") {
		t.Errorf("the per-job cap kept %d of 5000 bytes: %.120q", kept, results[0])
	}
	if !strings.Contains(results[1], "second") || strings.Count(results[2], "x") > 1100 {
		t.Errorf("the session cap did not evict the older log: %d bytes of the first job remain", strings.Count(results[2], "x"))
	}
}

// TestLiveRawLogsSettingKeepsTerminalSequences pins jobs.raw-logs.
func TestLiveRawLogsSettingKeepsTerminalSequences(t *testing.T) {
	coloured := map[string]any{"command": `printf '\033[31mred\033[0m\n10%%\r100%%\n'`}
	cleaned := runScripted(t, t.TempDir(), "bash", bashStep(coloured))
	if strings.Contains(cleaned[0], "\x1b") || strings.Contains(cleaned[0], "[31m") || !strings.Contains(cleaned[0], "red\n100%") {
		t.Errorf("default results must be cleaned of terminal sequences: %q", cleaned[0])
	}
	raw := runScriptedConfig(t, t.TempDir(), "bash", "[jobs]\nraw-logs = true\n", bashStep(coloured))
	if !strings.Contains(raw[0], "[31mred") || !strings.Contains(raw[0], "10%") {
		t.Errorf("raw-logs must return the bytes as written: %q", raw[0])
	}
}
