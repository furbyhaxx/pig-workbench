package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
)

type jobMetadata struct {
	Command, Cwd     string
	LocationRevision uint64
	Started          time.Time
	TimedOut         bool
}
type jobMetadataStore struct {
	mu     sync.Mutex
	values map[string]jobMetadata
}

func (a *app) registerJobs(ext *sdk.Extension) {
	ext.RegisterTool(sdk.ToolDefinition{Name: "bash", Label: "bash", Description: "Run a local shell command in the captured Workbench location. Defaults: foreground, 120-second hard deadline, stdin closed. background returns a job ID immediately; yield_seconds promotes a still-running command after that delay. timeout_seconds=0 disables the deadline. interactive keeps stdin open for jobs write/eof. Cancellation terminates the owned process group. Output is bounded and reports its byte cursor and any eviction gap.", PromptSnippet: "Run shell commands with owned background jobs and bounded output.", Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"command"}, "properties": map[string]any{"command": map[string]any{"type": "string", "minLength": 1}, "background": map[string]any{"type": "boolean"}, "interactive": map[string]any{"type": "boolean"}, "yield_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 600}, "timeout_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 86400}}}, Execute: a.bash})
	ext.RegisterTool(sdk.ToolDefinition{Name: "jobs", Label: "jobs", Description: "Inspect or control a session-owned shell job. action=list needs no id; status/output/wait/kill/write/eof require id. output uses a monotonic byte cursor; evicted bytes are reported. wait is cancellable without killing a background job; kill terminates its process group. write sends exact text and eof closes stdin.", PromptSnippet: "Inspect, await, stop or send input to owned jobs.", Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"action"}, "properties": map[string]any{"action": map[string]any{"type": "string", "enum": []string{"list", "status", "output", "wait", "kill", "write", "eof"}}, "id": map[string]any{"type": "string"}, "text": map[string]any{"type": "string"}, "cursor": map[string]any{"type": "integer", "minimum": 0}, "timeout_seconds": map[string]any{"type": "number", "minimum": 0, "maximum": 3600}}}, Execute: a.jobsTool})
}

type bashParams struct {
	Command     string   `json:"command"`
	Background  bool     `json:"background"`
	Interactive bool     `json:"interactive"`
	Yield       float64  `json:"yield_seconds"`
	Timeout     *float64 `json:"timeout_seconds"`
}

func (a *app) bash(ctx sdk.Context, params map[string]any) (any, error) {
	var input bashParams
	if err := decode(params, &input); err != nil {
		return nil, err
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
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, err
	}
	timeoutEntry, _ := configuration.Get("jobs.timeout-seconds")
	timeout := float64(timeoutEntry.Value.(int64))
	if input.Timeout != nil {
		timeout = *input.Timeout
	}
	if handled, result, err := a.dispatchSchemeCommand(ctx, work, s, input.Command); handled {
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
	id, err := s.jobs.Start(work, jobs.Spec{Command: []string{shell, "-c", input.Command}, Dir: location.Cwd, Env: environment, Stdin: input.Interactive})
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
	s.jobMeta.values[id] = jobMetadata{Command: input.Command, Cwd: location.Cwd, LocationRevision: location.Revision, Started: time.Now()}
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
	return s.jobResult(id, 0)
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
}

func (a *app) jobsTool(ctx sdk.Context, params map[string]any) (any, error) {
	var input jobParams
	if err := decode(params, &input); err != nil {
		return nil, err
	}
	if input.Cursor < 0 || input.Timeout < 0 || input.Timeout > 3600 {
		return nil, errors.New("invalid output cursor or wait timeout")
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
		encoded, _ := json.Marshal(states)
		return sdk.ToolResult{Content: string(encoded), Details: states}, nil
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
		if _, err := s.jobs.Wait(wait, input.ID); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
	case "kill":
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
	return s.jobResult(input.ID, input.Cursor)
}

func (s *session) jobResult(id string, cursor int64) (sdk.ToolResult, error) {
	state, err := s.jobs.Status(id)
	if err != nil {
		return sdk.ToolResult{}, err
	}
	output, next, truncated, gap, err := s.jobs.Output(id, cursor, 48<<10)
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
	s.jobMeta.mu.Unlock()
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
	content := fmt.Sprintf("Job %s: %s\n%s", id, label, output)
	details := map[string]any{"id": id, "state": label, "running": state.Running, "exit": state.Exit, "error": state.Error, "cursor": next, "truncated": truncated, "gapFrom": gap, "command": metadata.Command, "cwd": metadata.Cwd, "locationRevision": metadata.LocationRevision}
	failed := state.Error != "" || state.Exit != nil && state.Exit.Code != 0 || metadata.TimedOut
	return sdk.ToolResult{Content: content, Details: details, IsError: failed}, nil
}
