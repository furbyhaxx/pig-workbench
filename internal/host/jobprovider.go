package host

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"syscall"

	"github.com/furbyhaxx/pig-workbench/internal/jobs"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// jobProvider projects one session's jobs as the job:// tree.
// Writes to stdin and ctl are actions; reads page the bounded logs.
type jobProvider struct{ source func() (*session, bool) }

func (jobProvider) Scheme() string { return "job" }

var jobFiles = []string{"status", "cmdline", "cwd", "log", "stdout", "stderr", "stdin", "ctl"}

func splitJob(uri string) ([]string, error) {
	rest, ok := cutScheme(uri, "job")
	if !ok {
		return nil, errors.New("job provider requires a job:// URI")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil, nil
	}
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("job:// segment must not be empty, '.' or '..'")
		}
	}
	return parts, nil
}

func (p jobProvider) session() (*session, error) {
	s, ok := p.source()
	if !ok || s == nil || s.jobs == nil {
		return nil, errors.New("job resources are unavailable outside an active session")
	}
	return s, nil
}

func (p jobProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	parts, err := splitJob(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	s, err := p.session()
	if err != nil {
		return resource.StatResult{}, err
	}
	switch len(parts) {
	case 0:
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	case 1:
		if _, err := s.jobs.Status(parts[0]); err != nil {
			return resource.StatResult{}, resource.ErrNotFound
		}
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	case 2:
		if _, err := s.jobs.Status(parts[0]); err != nil {
			return resource.StatResult{}, resource.ErrNotFound
		}
		for _, name := range jobFiles {
			if name == parts[1] {
				kind := resource.EntryFile
				if name == "ctl" {
					kind = resource.EntryControl
				}
				return resource.StatResult{URI: uri, Kind: kind}, nil
			}
		}
	}
	return resource.StatResult{}, resource.ErrNotFound
}

func (p jobProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	parts, err := splitJob(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	s, err := p.session()
	if err != nil {
		return resource.Listing{}, err
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var names []string
	base := "job://"
	kind := resource.EntryDir
	switch len(parts) {
	case 0:
		for _, state := range s.jobs.List() {
			names = append(names, state.ID)
		}
	case 1:
		if _, err := s.jobs.Status(parts[0]); err != nil {
			return resource.Listing{}, resource.ErrNotFound
		}
		names, base, kind = jobFiles, "job://"+parts[0]+"/", resource.EntryFile
	default:
		return resource.Listing{}, errors.New("job:// lists only the catalog and one job")
	}
	start := 0
	for i, name := range names {
		if name == cursor {
			start = i
			break
		}
	}
	end := min(start+limit, len(names))
	entries := make([]resource.Entry, 0, end-start)
	for _, name := range names[start:end] {
		entryKind := kind
		if name == "ctl" {
			entryKind = resource.EntryControl
		}
		entries = append(entries, resource.Entry{Name: name, URI: base + name, Kind: entryKind})
	}
	listing := resource.Listing{Entries: entries, Complete: end == len(names)}
	if !listing.Complete {
		listing.NextCursor = names[end]
	}
	return listing, nil
}

func (p jobProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, err := splitJob(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	if len(parts) != 2 {
		return resource.ReadResult{}, errors.New("job:// reads a job file, not a directory; list it first")
	}
	s, err := p.session()
	if err != nil {
		return resource.ReadResult{}, err
	}
	id, name := parts[0], parts[1]
	data, err := jobFile(s, id, name)
	if err != nil {
		return resource.ReadResult{}, err
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, data, request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	page.ContentType = "text/plain"
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}

func jobFile(s *session, id, name string) ([]byte, error) {
	state, err := s.jobs.Status(id)
	if err != nil {
		return nil, resource.ErrNotFound
	}
	s.jobMeta.mu.Lock()
	meta := s.jobMeta.values[id]
	s.jobMeta.mu.Unlock()
	switch name {
	case "status":
		label := "running"
		if !state.Running {
			label = "exited"
		}
		if state.Cancelled {
			label = "cancelled"
		}
		if meta.TimedOut {
			label = "timedOut"
		}
		code := ""
		if state.Exit != nil {
			code = fmt.Sprintf(" exit=%d", state.Exit.Code)
		}
		runtime := time.Since(meta.Started).Truncate(time.Second)
		return []byte(fmt.Sprintf("id: %s\nstate: %s%s\nruntime: %s\nerror: %s\n", id, label, code, runtime, state.Error)), nil
	case "cmdline":
		return []byte(meta.Command + "\n"), nil
	case "cwd":
		return []byte(meta.Cwd + "\n"), nil
	case "log", "stdout", "stderr":
		stream := jobs.StreamCombined
		if name == "stdout" {
			stream = jobs.StreamStdout
		}
		if name == "stderr" {
			stream = jobs.StreamStderr
		}
		data, _, _, _, err := s.jobs.Stream(id, stream, 0, 1<<20)
		if err != nil && !errors.Is(err, jobs.ErrLogEvicted) {
			return nil, err
		}
		return data, nil
	case "stdin", "ctl":
		return nil, fmt.Errorf("%s is write-only; write the action instead of reading it", name)
	default:
		return nil, resource.ErrNotFound
	}
}

// jobAction applies a write to stdin or ctl. ctl text is one action: eof, kill or signal <name>.
func (p jobProvider) jobAction(s *session, uri, content string) error {
	parts, err := splitJob(uri)
	if err != nil {
		return err
	}
	if len(parts) != 2 {
		return errors.New("job:// writes target stdin or ctl of one job")
	}
	id, name := parts[0], parts[1]
	if _, err := s.jobs.Status(id); err != nil {
		return resource.ErrNotFound
	}
	switch name {
	case "stdin":
		_, err := s.jobs.Write(id, []byte(content))
		return err
	case "ctl":
		action := strings.TrimSpace(content)
		switch {
		case action == "eof":
			return s.jobs.EOF(id)
		case action == "kill":
			return s.jobs.Cancel(id)
		case strings.HasPrefix(action, "signal "):
			name := strings.TrimSpace(strings.TrimPrefix(action, "signal "))
			number, ok := jobSignals[strings.ToUpper(name)]
			if !ok {
				return fmt.Errorf("unsupported signal %q; supported: HUP INT QUIT TERM KILL", name)
			}
			return s.jobs.Signal(id, syscall.Signal(number))
		default:
			return fmt.Errorf("ctl accepts eof, kill or signal <name>; got %q", action)
		}
	default:
		return fmt.Errorf("job:// %s is read-only", name)
	}
}

// jobCatalog renders the job list for ls job://.
func jobCatalog(s *session) string {
	states := s.jobs.List()
	if len(states) == 0 {
		return "no jobs\n"
	}
	var b strings.Builder
	for _, state := range states {
		s.jobMeta.mu.Lock()
		meta := s.jobMeta.values[state.ID]
		s.jobMeta.mu.Unlock()
		label := "running"
		if !state.Running {
			label = "exited"
		}
		_, _ = fmt.Fprintf(&b, "%s\t%s\t%s\n", state.ID, label, meta.Command)
	}
	return b.String()
}

var jobSignals = map[string]syscall.Signal{"HUP": syscall.SIGHUP, "INT": syscall.SIGINT, "QUIT": syscall.SIGQUIT, "TERM": syscall.SIGTERM, "KILL": syscall.SIGKILL}
