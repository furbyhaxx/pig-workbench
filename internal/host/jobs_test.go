package host

import (
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/jobs"
)

// TestJobLabelNamesEverySpecState pins the SPEC-0008 §6 state names shown in rows.
func TestJobLabelNamesEverySpecState(t *testing.T) {
	exit := &jobs.Exit{}
	for want, state := range map[string]jobs.JobState{
		"failedToStart": {FailedToStart: true, Error: "no such directory"},
		"unknown":       {Running: true, Unknown: true, Cancelled: true},
		"stopping":      {Running: true, Stopping: true},
		"running":       {Running: true},
		"cancelled":     {Cancelled: true, Exit: exit},
		"exited":        {Exit: exit},
	} {
		facts := newJobFacts(state, jobMetadata{})
		if facts.State != want {
			t.Errorf("state %+v is labelled %q, want %q", state, facts.State, want)
		}
		if facts.Running != (want == "running" || want == "stopping") {
			t.Errorf("%s: running = %v", want, facts.Running)
		}
	}
	if got := jobLabel(jobs.JobState{Cancelled: true, Exit: exit}, jobMetadata{TimedOut: true}); got != "timedOut" {
		t.Errorf("a deadline kill is labelled %q", got)
	}
}

// TestTerminalTextKeepsWhatATerminalWouldShow pins the default cleaning of command output.
func TestTerminalTextKeepsWhatATerminalWouldShow(t *testing.T) {
	for input, want := range map[string]string{
		"plain\n":                                   "plain\n",
		"\x1b[1;31mred\x1b[0m text\n":               "red text\n",
		"crlf\r\nnext\r\n":                          "crlf\nnext\n",
		"10%\r50%\r100%\ndone\n":                    "100%\ndone\n",
		"\x1b]0;window title\x07shown\n":            "shown\n",
		"\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\": "link",
		"tab\there\x00\x08\x7f\n":                   "tab\there\n",
		"héllo ✓\n":                                 "héllo ✓\n",
		"cut \x1b[3":                                "cut ",
		"spinner\r\r\n":                             "spinner\n",
	} {
		if got := string(terminalText([]byte(input))); got != want {
			t.Errorf("terminalText(%q) = %q, want %q", input, got, want)
		}
	}
}
