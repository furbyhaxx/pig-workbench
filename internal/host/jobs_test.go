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
