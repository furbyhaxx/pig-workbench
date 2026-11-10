package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type jobMetadata struct {
	Title            string
	Command, Cwd     string
	LocationRevision uint64
	Started          time.Time
	TimedOut         bool
	// Waiters counts jobs-wait calls in flight; Observed records that a tool result
	// already showed the model the job's terminal state.
	Waiters  int
	Observed bool
}

// describe names the job for people: its title when it has one, with the command behind it.
func (m jobMetadata) describe() string {
	if m.Title == "" {
		return m.Command
	}
	return m.Title + ": " + m.Command
}

type jobMetadataStore struct {
	mu     sync.Mutex
	values map[string]jobMetadata
}

func (a *app) registerJobs(ext *sdk.Extension) {
	ext.RegisterTool(sdk.ToolDefinition{Name: "bash", Label: "bash", Description: "Run a local shell command in the captured Workbench location. Defaults: foreground, 120-second hard deadline, stdin closed. cwd runs this one command in another directory without changing the location; title names the job in listings. background returns a job ID immediately; yield_seconds promotes a still-running command after that delay. timeout_seconds=0 disables the deadline. interactive keeps stdin open for jobs write/eof. Cancellation terminates the owned process group. Output is bounded and reports its byte cursor and any eviction gap. Scheme paths work as file arguments: a standalone ls, cat, rm, mkdir, mv, cp or `word > path` acts on the resource itself, and other commands read a copy (for example `cat job://<id>/log | wc -l`); writes need a standalone form.", PromptSnippet: "Run shell commands with owned background jobs and bounded output.", Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"command"}, "properties": map[string]any{"command": map[string]any{"type": "string", "minLength": 1}, "background": map[string]any{"type": "boolean"}, "interactive": map[string]any{"type": "boolean"}, "yield_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 600}, "timeout_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 86400}, "title": map[string]any{"type": "string", "maxLength": 80, "description": "Short one-line name shown in job listings."}, "cwd": map[string]any{"type": "string", "description": "Directory for this command only; absolute or relative to the Workbench location."}}}, Execute: a.bash})
	ext.RegisterTool(sdk.ToolDefinition{Name: "jobs", Label: "jobs", Description: "Inspect or control a session-owned shell job. action=list needs no id; status/output/wait/kill/write/eof require id. output uses a monotonic byte cursor; evicted bytes are reported. wait returns at exit by default; until=output returns when new bytes pass the cursor. wait is cancellable without killing a background job. kill terminates its process group; signal=INT|HUP|QUIT sends that signal instead and reports the observed state. stream=stdout|stderr reads one stream; its cursor counts only that stream's bytes. write sends exact text and eof closes stdin.", PromptSnippet: "Inspect, await, stop or send input to owned jobs.", Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"action"}, "properties": map[string]any{"action": map[string]any{"type": "string", "enum": []string{"list", "status", "output", "wait", "kill", "write", "eof"}}, "id": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}, "cursor": map[string]any{"type": "integer", "minimum": 0}, "timeout_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 3600}, "until": map[string]any{"type": "string", "enum": []string{"exit", "output"}}, "stream": map[string]any{"type": "string", "enum": []string{"combined", "stdout", "stderr"}}, "signal": map[string]any{"type": "string", "enum": []string{"TERM", "INT", "HUP", "QUIT", "KILL"}}}}, Execute: a.jobsTool})
}

type bashParams struct {
	Command     string   `json:"command"`
	Background  bool     `json:"background"`
	Interactive bool     `json:"interactive"`
	Yield       float64  `json:"yield_seconds"`
	Timeout     *float64 `json:"timeout_seconds"`
	Title       string   `json:"title"`
	Cwd         string   `json:"cwd"`
}

func (a *app) bash(ctx sdk.Context, params map[string]any) (any, error) {
	var input bashParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	if len(input.Title) > 80 || strings.ContainsAny(input.Title, "\r\n") {
		return nil, errors.New("title is one line of at most 80 characters")
	}
	if strings.TrimSpace(input.Command) == "" || input.Yield < 0 || input.Yield > 600 || input.Timeout != nil && (*input.Timeout < 0 || *input.Timeout > 86400) {
		return nil, errors.New("invalid command or timeout")
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	location, done, err := s.location.Acquire(ctx.ToolCallID(), false)
	if err != nil {
		return nil, err
	}
	defer done()
	if input.Cwd != "" {
		// The override runs this one command elsewhere; the session location is untouched.
		if location.Cwd, err = commandDir(location.Cwd, input.Cwd); err != nil {
			return nil, err
		}
	}
	work = withCapturedLocation(work, location)
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, err
	}
	timeoutEntry, _ := configuration.Get("jobs.timeout-seconds")
	timeout := float64(timeoutEntry.Value.(int64))
	if input.Timeout != nil {
		timeout = *input.Timeout
	}
	handled, result, command, err := a.prepareShellCommand(ctx, work, s, input.Command, ctx.ToolCallID())
	if handled || err != nil {
		return result, err
	}
	shell := "/bin/sh"
	if _, err := os.Stat("/bin/bash"); err == nil {
		shell = "/bin/bash"
	}
	environment, err := s.jobEnvironment(work)
	if err != nil {
		s.secrets.withhold(ctx, err)
		return nil, errors.New("the secrets boundary is unavailable; the command was not run")
	}
	id, err := s.jobs.Start(work, jobs.Spec{Command: []string{shell, "-c", command}, Dir: location.Cwd, Env: environment, Stdin: input.Interactive})
	if err != nil {
		return nil, err
	}
	if s.jobRegistry != nil {
		if pid, identity, ok := s.jobs.ProcessID(id); ok {
			if err := s.jobRegistry.Add(jobs.Record{JobID: id, SessionID: s.id, Generation: s.generation, PID: pid, Identity: identity}); err != nil {
				ctx.Notify(noticeText("Job registry record failed: "+err.Error(), 0), "warning")
			}
		}
	}
	retained := s.jobs.List()
	keep := make(map[string]bool, len(retained))
	for _, state := range retained {
		keep[state.ID] = true
	}
	s.jobMeta.mu.Lock()
	for old := range s.jobMeta.values {
		if !keep[old] {
			delete(s.jobMeta.values, old)
		}
	}
	s.jobMeta.values[id] = jobMetadata{Title: input.Title, Command: input.Command, Cwd: location.Cwd, LocationRevision: location.Revision, Started: time.Now()}
	s.jobMeta.mu.Unlock()
	if timeout > 0 {
		err := s.supervisor.Start(func(lifetime context.Context) {
			deadline, cancel := context.WithTimeout(lifetime, time.Duration(timeout*float64(time.Second)))
			defer cancel()
			if _, err := s.jobs.Wait(deadline, id); errors.Is(err, context.DeadlineExceeded) {
				s.jobMeta.mu.Lock()
				meta := s.jobMeta.values[id]
				meta.TimedOut = true
				s.jobMeta.values[id] = meta
				s.jobMeta.mu.Unlock()
				_ = s.jobs.Cancel(id)
			}
		})
		if err != nil {
			_ = s.jobs.Cancel(id)
			return nil, err
		}
	}
	if !input.Background {
		wait := work
		cancel := func() {}
		if input.Yield > 0 {
			wait, cancel = context.WithTimeout(work, time.Duration(input.Yield*float64(time.Second)))
		}
		_, waitErr := s.jobs.Wait(wait, id)
		cancel()
		if work.Err() != nil {
			_ = s.jobs.Cancel(id)
			return nil, work.Err()
		}
		if waitErr != nil && !errors.Is(waitErr, context.DeadlineExceeded) {
			return nil, waitErr
		}
		if waitErr == nil {
			a.forgetJobRecord(s, id)
		}
	}
	if state, err := s.jobs.Status(id); err == nil && state.Running {
		a.watchJobCompletion(s, id)
	}
	return s.jobResult(id, jobs.StreamCombined, 0)
}

// watchJobCompletion sends one notice when a job that outlived its tool call
// ends, waking an idle model unless jobs.wake-on-completion is off. The notice
// carries no output, so the secrets boundary stays on the read path; a job
// killed on request is already known to the model.
func (a *app) watchJobCompletion(s *session, id string) {
	_ = s.supervisor.Start(func(lifetime context.Context) {
		if _, err := s.jobs.Wait(lifetime, id); err != nil {
			return
		}
		state, err := s.jobs.Status(id)
		if err != nil {
			return
		}
		if !s.settleJobObservers(lifetime, id) {
			return
		}
		s.jobMeta.mu.Lock()
		metadata := s.jobMeta.values[id]
		s.jobMeta.mu.Unlock()
		if state.Cancelled && !metadata.TimedOut || metadata.Observed {
			return
		}
		label := jobLabel(state, metadata)
		if state.Exit != nil {
			label += fmt.Sprintf(" (exit %d)", state.Exit.Code)
		}
		body := fmt.Sprintf("Background job %s finished: %s\nCommand: %s\nRead job://%s/log or use the jobs tool for its output.", id, label, noticeText(metadata.Command, 200), id)
		wake := s.settingBool("jobs.wake-on-completion", true)
		receipt, frame, failure := a.admitFrame(s.host, s, "workbench", "data", true, frames.Publish{EventKey: "job/" + id + "/exit", Kind: "notice", Source: "workbench.jobs", Title: "Job " + id + " finished", Body: body, SubjectKey: "job://" + id, Resource: "job://" + id, Delivery: "nextBoundary", Wake: wake, DataTrust: "trusted"})
		if failure != nil || receipt.Duplicate {
			return
		}
		// A failed send leaves the frame pending for the next boundary hook.
		_, _ = a.deliverFrame(s.host, s, frame, wake)
		s.host.Notify(noticeText("Background job "+id+" finished: "+label, 0), "info")
	})
}

// adjustJobWaiters tracks jobs-wait calls in flight so the completion watcher does not
// announce an exit that call is about to report.
func (s *session) adjustJobWaiters(id string, delta int) {
	s.jobMeta.mu.Lock()
	defer s.jobMeta.mu.Unlock()
	if metadata, ok := s.jobMeta.values[id]; ok {
		metadata.Waiters += delta
		s.jobMeta.values[id] = metadata
	}
}

// settleJobObservers waits until no jobs-wait call is in flight for the job, so a tool
// result that reports the exit has been recorded before the watcher decides. It returns
// false when the session ended first.
func (s *session) settleJobObservers(ctx context.Context, id string) bool {
	for {
		s.jobMeta.mu.Lock()
		waiting := s.jobMeta.values[id].Waiters > 0
		s.jobMeta.mu.Unlock()
		if !waiting {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// forgetJobRecord drops a finished job's registry record; a job that outlives
// this runtime is reclaimed by the next start's sweep instead.
func (a *app) forgetJobRecord(s *session, id string) {
	if s == nil || s.jobRegistry == nil || id == "" {
		return
	}
	_ = s.jobRegistry.Remove(s.id, id)
}

type jobParams struct {
	Action  string  `json:"action"`
	ID      string  `json:"id"`
	Text    string  `json:"text"`
	Cursor  int64   `json:"cursor"`
	Timeout float64 `json:"timeout_seconds"`
	Until   string  `json:"until"`
	Stream  string  `json:"stream"`
	Signal  string  `json:"signal"`
}

func jobStream(name string) (jobs.Stream, error) {
	switch name {
	case "", "combined":
		return jobs.StreamCombined, nil
	case "stdout":
		return jobs.StreamStdout, nil
	case "stderr":
		return jobs.StreamStderr, nil
	}
	return 0, fmt.Errorf("unknown stream %q; use combined, stdout or stderr", name)
}

// waitOutput returns once the stream has bytes past cursor or the job has ended.
func (s *session) waitOutput(ctx context.Context, id string, stream jobs.Stream, cursor int64) error {
	for {
		data, _, _, _, err := s.jobs.Stream(id, stream, cursor, 1)
		if err != nil && !errors.Is(err, jobs.ErrLogEvicted) {
			return err
		}
		state, err := s.jobs.Status(id)
		if err != nil {
			return err
		}
		if len(data) > 0 || !state.Running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (a *app) jobsTool(ctx sdk.Context, params map[string]any) (any, error) {
	var input jobParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	if input.Cursor < 0 || input.Timeout < 0 || input.Timeout > 3600 {
		return nil, errors.New("invalid output cursor or wait timeout")
	}
	stream, err := jobStream(input.Stream)
	if err != nil {
		return nil, err
	}
	if input.Until != "" && input.Until != "exit" && input.Until != "output" || input.Until != "" && input.Action != "wait" {
		return nil, errors.New("until applies to wait and takes exit or output")
	}
	if input.Signal != "" && input.Action != "kill" {
		return nil, errors.New("signal applies to kill")
	}
	signal, forceSignal := jobSignals[input.Signal]
	if input.Signal != "" && !forceSignal {
		return nil, fmt.Errorf("unsupported signal %q; supported: TERM INT HUP QUIT KILL", input.Signal)
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if input.Action == "list" {
		states := s.jobs.List()
		live := make(map[string]bool, len(states))
		for _, state := range states {
			live[state.ID] = true
		}
		s.jobMeta.mu.Lock()
		for id := range s.jobMeta.values {
			if !live[id] {
				delete(s.jobMeta.values, id)
			}
		}
		s.jobMeta.mu.Unlock()
		list := jobList{Jobs: make([]jobFacts, 0, len(states)), Omitted: s.jobs.Retired()}
		s.jobMeta.mu.Lock()
		for _, state := range states {
			list.Jobs = append(list.Jobs, newJobFacts(state, s.jobMeta.values[state.ID]))
		}
		s.jobMeta.mu.Unlock()
		encoded, err := json.Marshal(list)
		if err != nil {
			return nil, err
		}
		return sdk.ToolResult{Content: string(encoded), Details: list}, nil
	}
	if input.ID == "" {
		return nil, errors.New("job id is required")
	}
	switch input.Action {
	case "status", "output":
	case "wait":
		wait := work
		cancel := func() {}
		if input.Timeout > 0 {
			wait, cancel = context.WithTimeout(work, time.Duration(input.Timeout*float64(time.Second)))
		}
		defer cancel()
		s.adjustJobWaiters(input.ID, 1)
		defer s.adjustJobWaiters(input.ID, -1)
		var waitErr error
		if input.Until == "output" {
			waitErr = s.waitOutput(wait, input.ID, stream, input.Cursor)
		} else {
			_, waitErr = s.jobs.Wait(wait, input.ID)
		}
		if waitErr != nil && !errors.Is(waitErr, context.DeadlineExceeded) {
			return nil, waitErr
		}
	case "kill":
		if forceSignal && signal != syscall.SIGTERM && signal != syscall.SIGKILL {
			if err := s.jobs.Signal(input.ID, signal); err != nil {
				return nil, err
			}
			settle, cancel := context.WithTimeout(work, 500*time.Millisecond)
			_, _ = s.jobs.Wait(settle, input.ID)
			cancel()
			break
		}
		if err := s.jobs.Cancel(input.ID); err != nil {
			return nil, err
		}
		a.forgetJobRecord(s, input.ID)
	case "write":
		if _, err := s.jobs.Write(input.ID, []byte(input.Text)); err != nil {
			return nil, err
		}
	case "eof":
		if err := s.jobs.EOF(input.ID); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unknown jobs action")
	}
	return s.jobResult(input.ID, stream, input.Cursor)
}

// jobFacts is one job as the jobs tool reports it (SPEC-0008 §6). Absent facts are omitted, never zero.
type jobFacts struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Command   string `json:"command"`
	Cwd       string `json:"cwd"`
	StartedAt string `json:"startedAt,omitempty"`
	State     string `json:"state"`
	Running   bool   `json:"running"`
	ExitCode  *int   `json:"exitCode,omitempty"`
	Signal    string `json:"signal,omitempty"`
	// DurationMs is the run time in milliseconds, present once the job exited.
	DurationMs *int64 `json:"durationMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// jobList is the list result: every active job, the retained finished ones and how many older
// finished jobs were dropped.
type jobList struct {
	Jobs    []jobFacts `json:"jobs"`
	Omitted int        `json:"omitted"`
}

func newJobFacts(state jobs.JobState, metadata jobMetadata) jobFacts {
	facts := jobFacts{ID: state.ID, Title: metadata.Title, Command: metadata.Command, Cwd: metadata.Cwd, State: jobLabel(state, metadata), Running: state.Running, Error: state.Error}
	if !metadata.Started.IsZero() {
		facts.StartedAt = metadata.Started.Format(time.RFC3339)
	}
	if state.Exit != nil {
		code, duration := state.Exit.Code, state.Exit.Duration.Milliseconds()
		facts.DurationMs = &duration
		if state.Exit.Signaled {
			facts.Signal = state.Exit.Signal
		} else {
			facts.ExitCode = &code
		}
	}
	return facts
}

// commandDir resolves a bash cwd override against base to an existing local directory.
func commandDir(base, name string) (string, error) {
	target, err := resource.ParsePath(base, name, "")
	if err != nil {
		return "", err
	}
	if target.Scheme != "file" {
		return "", fmt.Errorf("cwd must be a local directory, not a %s:// resource", target.Scheme)
	}
	dir, err := filepath.EvalSymlinks(target.Path)
	if err != nil {
		return "", fmt.Errorf("cwd %s: %w", name, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("cwd %s is not a directory", name)
	}
	return dir, nil
}

func jobLabel(state jobs.JobState, metadata jobMetadata) string {
	label := "running"
	if !state.Running {
		label = "exited"
	}
	if state.Cancelled {
		label = "cancelled"
	}
	if metadata.TimedOut {
		label = "timedOut"
	}
	return label
}

// jobRows lists the session's jobs for people and panels.
func (s *session) jobRows() []jobRow {
	states := s.jobs.List()
	rows := make([]jobRow, 0, len(states))
	for _, state := range states {
		s.jobMeta.mu.Lock()
		metadata := s.jobMeta.values[state.ID]
		s.jobMeta.mu.Unlock()
		label := jobLabel(state, metadata)
		if state.Exit != nil && !state.Running {
			label += fmt.Sprintf(" (exit %d)", state.Exit.Code)
		}
		rows = append(rows, jobRow{id: state.ID, label: label, command: noticeText(metadata.describe(), 0)})
	}
	return rows
}

// jobListText renders the session's jobs for people; the jobs tool keeps its JSON for models.
func (s *session) jobListText() string {
	rows := s.jobRows()
	if len(rows) == 0 {
		return "No jobs in this session."
	}
	var b strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&b, "%s  %s  %s\n", row.id, row.label, row.command)
	}
	return b.String()
}

// jobPage is the Details of a single-job result: the job's facts plus the output page it carries.
type jobPage struct {
	jobFacts
	Cursor           int64  `json:"cursor"`
	Truncated        bool   `json:"truncated"`
	GapFrom          int64  `json:"gapFrom"`
	LocationRevision uint64 `json:"locationRevision"`
}

func (s *session) jobResult(id string, stream jobs.Stream, cursor int64) (sdk.ToolResult, error) {
	state, err := s.jobs.Status(id)
	if err != nil {
		return sdk.ToolResult{}, err
	}
	output, next, truncated, gap, err := s.jobs.Stream(id, stream, cursor, 48<<10)
	if err != nil && !errors.Is(err, jobs.ErrLogEvicted) {
		return sdk.ToolResult{}, err
	}
	start := next - int64(len(output))
	lines := 0
	for i, value := range output {
		if value == '\n' {
			lines++
			if lines >= 1997 && i+1 < len(output) {
				output = output[:i+1]
				next = start + int64(len(output))
				truncated = true
				break
			}
		}
	}
	s.jobMeta.mu.Lock()
	metadata := s.jobMeta.values[id]
	if _, ok := s.jobMeta.values[id]; ok && !state.Running {
		observed := metadata
		observed.Observed = true
		s.jobMeta.values[id] = observed
	}
	s.jobMeta.mu.Unlock()
	label := jobLabel(state, metadata)
	content := fmt.Sprintf("Job %s: %s\n%s", id, label, output)
	details := jobPage{jobFacts: newJobFacts(state, metadata), Cursor: next, Truncated: truncated, GapFrom: gap, LocationRevision: metadata.LocationRevision}
	failed := state.Error != "" || state.Exit != nil && state.Exit.Code != 0 || metadata.TimedOut
	return sdk.ToolResult{Content: content, Details: details, IsError: failed}, nil
}
