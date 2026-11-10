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

// TestJobSignalNamesAreSharedByToolAndCtl pins the one signal table and its one refusal text.
func TestJobSignalNamesAreSharedByToolAndCtl(t *testing.T) {
	for _, name := range []string{"TERM", "term", "SIGTERM", " sigint ", "HUP", "QUIT", "KILL"} {
		if _, err := jobSignal(name); err != nil {
			t.Errorf("jobSignal(%q): %v", name, err)
		}
	}
	const refusal = `unsupported signal "USR1"; supported: TERM INT HUP QUIT KILL`
	if _, err := jobSignal("USR1"); err == nil || err.Error() != refusal {
		t.Fatalf("lookup refusal = %v", err)
	}
	s := newJobTestSession(t)
	id, err := s.jobs.Start(t.Context(), jobs.Spec{Command: []string{"sleep", "30"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := (jobProvider{}).jobAction(s, "job://"+id+"/ctl", "signal USR1"); err == nil || err.Error() != refusal {
		t.Fatalf("ctl refusal = %v", err)
	}
}
