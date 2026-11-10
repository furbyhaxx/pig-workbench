package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// sessionProvider projects the running session as read-only resources.
type sessionProvider struct {
	ctx    func() (sdk.Context, bool)
	source func() (*session, bool)
}

func (sessionProvider) Scheme() string { return "session" }

func (p sessionProvider) current() (sdk.Context, error) {
	ctx, ok := p.ctx()
	if !ok {
		return sdk.Context{}, errors.New("session resources are unavailable outside an active session")
	}
	return ctx, nil
}

func splitSession(uri string) ([]string, error) {
	rest, ok := cutScheme(uri, "session")
	if !ok {
		return nil, errors.New("session provider requires a session:// URI")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil, nil
	}
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, errors.New("session:// segment must not be empty, '.' or '..'")
		}
	}
	return parts, nil
}

var sessionFiles = []string{"meta", "transcript", "entries", "title", "system-prompt"}
var storedSessionFiles = []string{"meta", "transcript", "entries", "title"}

// sessionTarget is a parsed session:// URI: the session it names and the file below it.
type sessionTarget struct {
	root bool
	byID bool
	id   string
	file string
	// frame is the entry name below frames/ or instructions/.
	frame string
	depth int
}

func parseSessionTarget(parts []string) (sessionTarget, error) {
	var t sessionTarget
	if len(parts) == 0 {
		t.root = true
		return t, nil
	}
	if parts[0] == "by-id" {
		t.byID = true
		parts = parts[1:]
		if len(parts) == 0 {
			return t, nil
		}
	}
	if len(parts) > 3 || len(parts) == 3 && parts[1] != "frames" && parts[1] != "instructions" {
		return t, resource.ErrNotFound
	}
	t.id, t.depth = parts[0], len(parts)
	if len(parts) >= 2 {
		t.file = parts[1]
	}
	if len(parts) == 3 {
		t.frame = parts[2]
	}
	return t, nil
}

// resolveStored finds a stored session by full ID or an unambiguous prefix of at least 6 characters.
func (p sessionProvider) resolveStored(host sdk.Context, id string) (sdk.SessionInfo, error) {
	all, err := sdk.SessionManagerListAll(host, customSessionDir(host), nil)
	if err != nil {
		return sdk.SessionInfo{}, fmt.Errorf("list sessions: %w", err)
	}
	var matches []sdk.SessionInfo
	for _, info := range dedupeByID(all) {
		if info.ID == id {
			return info, nil
		}
		if idMatches(info.ID, id) {
			matches = append(matches, info)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return sdk.SessionInfo{}, resource.ErrNotFound
	}
	return sdk.SessionInfo{}, fmt.Errorf("%q matches %d sessions; use a longer ID", id, len(matches))
}

func (p sessionProvider) isSelf(host sdk.Context, id string) bool {
	if id == "self" {
		return true
	}
	own, err := host.GetSessionID()
	return err == nil && own == id
}

func containsName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

func (p sessionProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	parts, err := splitSession(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	host, err := p.current()
	if err != nil {
		return resource.StatResult{}, err
	}
	t, err := parseSessionTarget(parts)
	if err != nil {
		return resource.StatResult{}, err
	}
	if t.root || (t.byID && t.id == "") {
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	}
	files := storedSessionFiles
	if p.isSelf(host, t.id) {
		files = sessionFiles
	} else if _, err := p.resolveStored(host, t.id); err != nil {
		return resource.StatResult{}, err
	}
	if t.depth == 1 {
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	}
	if t.file == "frames" {
		records, err := p.frameRecords(host, t.id)
		if err != nil {
			return resource.StatResult{}, err
		}
		if t.depth == 2 {
			return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
		}
		if _, ok := findFrame(records, t.frame); ok {
			return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
		}
		return resource.StatResult{}, resource.ErrNotFound
	}
	if t.file == "instructions" {
		entries, err := p.instructionEntries(host, t.id)
		if err != nil {
			return resource.StatResult{}, err
		}
		if t.depth == 2 {
			return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
		}
		for _, entry := range entries {
			if entry.Name == t.frame {
				return resource.StatResult{URI: uri, Kind: entry.Kind}, nil
			}
		}
		return resource.StatResult{}, resource.ErrNotFound
	}
	if containsName(files, t.file) {
		return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
	}
	return resource.StatResult{}, resource.ErrNotFound
}

func sessionDetail(info sdk.SessionInfo) string {
	title := "(untitled)"
	if info.Name != nil && *info.Name != "" {
		title = *info.Name
	}
	return fmt.Sprintf("%s  %d messages  %s  %s", title, info.MessageCount, info.Modified.Local().Format("2006-01-02 15:04"), info.Cwd)
}

func (p sessionProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	parts, err := splitSession(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	host, err := p.current()
	if err != nil {
		return resource.Listing{}, err
	}
	t, err := parseSessionTarget(parts)
	if err != nil {
		return resource.Listing{}, err
	}
	switch {
	case t.root:
		entries := []resource.Entry{{Name: "self", URI: "session://self/", Kind: resource.EntryDir}, {Name: "by-id", URI: "session://by-id/", Kind: resource.EntryDir}}
		stored, err := sdk.SessionManagerList(host, host.Cwd(), customSessionDir(host), nil)
		if err != nil {
			return resource.Listing{}, fmt.Errorf("list sessions: %w", err)
		}
		for _, info := range stored {
			entries = append(entries, resource.Entry{Name: info.ID, URI: "session://" + info.ID + "/", Kind: resource.EntryDir, Detail: sessionDetail(info)})
		}
		return resource.PageEntries(entries, cursor, limit)
	case t.byID && t.id == "":
		all, err := sdk.SessionManagerListAll(host, customSessionDir(host), nil)
		if err != nil {
			return resource.Listing{}, fmt.Errorf("list sessions: %w", err)
		}
		entries := make([]resource.Entry, 0, len(all))
		for _, info := range dedupeByID(all) {
			entries = append(entries, resource.Entry{Name: info.ID, URI: "session://by-id/" + info.ID, Kind: resource.EntryLink, Target: "session://" + info.ID + "/", Detail: sessionDetail(info)})
		}
		return resource.PageEntries(entries, cursor, limit)
	}
	if t.depth == 2 && t.file == "frames" {
		records, err := p.frameRecords(host, t.id)
		if err != nil {
			return resource.Listing{}, err
		}
		entries := make([]resource.Entry, 0, len(records))
		for _, record := range records {
			entries = append(entries, resource.Entry{Name: record.Frame.FrameID, URI: "session://self/frames/" + record.Frame.FrameID, Kind: resource.EntryFile, Detail: frameReceiptLine(record)})
		}
		return resource.PageEntries(entries, cursor, limit)
	}
	if t.depth == 2 && t.file == "instructions" {
		entries, err := p.instructionEntries(host, t.id)
		if err != nil {
			return resource.Listing{}, err
		}
		return resource.PageEntries(entries, cursor, limit)
	}
	if t.depth != 1 {
		return resource.Listing{}, resource.ErrNotFound
	}
	files, base := storedSessionFiles, "session://"+t.id+"/"
	if p.isSelf(host, t.id) {
		files, base = sessionFiles, "session://self/"
	} else if _, err := p.resolveStored(host, t.id); err != nil {
		return resource.Listing{}, err
	}
	entries := make([]resource.Entry, 0, len(files))
	for _, name := range files {
		entries = append(entries, resource.Entry{Name: name, URI: base + name, Kind: resource.EntryFile})
	}
	if p.isSelf(host, t.id) {
		entries = append(entries, resource.Entry{Name: "frames", URI: base + "frames/", Kind: resource.EntryDir}, resource.Entry{Name: "instructions", URI: base + "instructions/", Kind: resource.EntryDir})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return resource.PageEntries(entries, cursor, limit)
}

func (p sessionProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, err := splitSession(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	session, err := p.current()
	if err != nil {
		return resource.ReadResult{}, err
	}
	t, err := parseSessionTarget(parts)
	if err != nil {
		return resource.ReadResult{}, err
	}
	if t.depth < 2 {
		return resource.ReadResult{}, resource.ErrUnsupported
	}
	var body []byte
	if t.file == "instructions" {
		return resource.ReadResult{}, fmt.Errorf("%w: list session://self/instructions/ and read the linked file; write pin or unpin to its ctl", resource.ErrUnsupported)
	}
	if t.file == "frames" {
		body, err = p.renderFrames(session, t)
	} else if p.isSelf(session, t.id) {
		body, err = p.render(ctx, session, t.file)
	} else {
		var info sdk.SessionInfo
		if info, err = p.resolveStored(session, t.id); err == nil {
			body, err = renderStored(ctx, session, info, t.file)
		}
	}
	if err != nil {
		return resource.ReadResult{}, err
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, body, request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}

func (p sessionProvider) render(ctx context.Context, session sdk.Context, name string) ([]byte, error) {
	switch name {
	case "meta":
		id, err := session.GetSessionID()
		if err != nil {
			return nil, err
		}
		entries, err := session.GetEntries()
		if err != nil {
			return nil, err
		}
		branch, err := session.GetBranch()
		if err != nil {
			return nil, err
		}
		leaf, err := session.GetLeafID()
		if err != nil {
			return nil, err
		}
		name, err := session.GetSessionName()
		if err != nil {
			return nil, err
		}
		meta := map[string]any{
			"sessionId":     id,
			"name":          name,
			"cwd":           session.Cwd(),
			"mode":          session.Mode(),
			"entries":       len(entries),
			"branchEntries": len(branch),
			"leafId":        leaf,
		}
		file, err := session.GetSessionFile()
		if err != nil {
			return nil, err
		}
		if file != nil {
			meta["file"] = *file
		} else {
			meta["durable"] = false
		}
		return json.MarshalIndent(meta, "", "  ")
	case "title":
		title, err := session.GetSessionName()
		if err != nil {
			return nil, err
		}
		if title == nil {
			return []byte("\n"), nil
		}
		return []byte(*title + "\n"), nil
	case "system-prompt":
		s, ok := p.source()
		if !ok {
			return nil, errors.New("session state is unavailable")
		}
		return servedPrompt(session, s)
	case "entries":
		entries, err := session.GetEntries()
		if err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, raw := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			out.WriteString(string(raw))
			out.WriteByte('\n')
		}
		return []byte(out.String()), nil
	case "transcript":
		branch, err := session.GetBranch()
		if err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, entry := range branch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			switch entry.Role {
			case "user":
				fmt.Fprintf(&out, "## user\n\n%s\n\n", strings.TrimRight(entry.Content, "\n"))
			case "assistant":
				fmt.Fprintf(&out, "## assistant\n\n%s\n\n", strings.TrimRight(entry.Content, "\n"))
			case "toolResult":
				fmt.Fprintf(&out, "## tool %s\n\n%s\n\n", entry.ToolName, strings.TrimRight(entry.Content, "\n"))
			default:
				if entry.Content != "" {
					role := entry.Role
					if role == "" {
						role = "context"
					}
					fmt.Fprintf(&out, "## %s\n\n%s\n\n", role, strings.TrimRight(entry.Content, "\n"))
				}
			}
		}
		return []byte(out.String()), nil
	}
	return nil, resource.ErrNotFound
}

// dedupeByID keeps the first (most recent) entry of each session: a shared project directory is reachable through one bucket per checkout.
func dedupeByID(all []sdk.SessionInfo) []sdk.SessionInfo {
	seen := make(map[string]bool, len(all))
	unique := make([]sdk.SessionInfo, 0, len(all))
	for _, info := range all {
		if !seen[info.ID] {
			seen[info.ID] = true
			unique = append(unique, info)
		}
	}
	return unique
}

// frameRecords returns this session's frame ledger, oldest first; only the own session has one.
func (p sessionProvider) frameRecords(host sdk.Context, id string) ([]frameRecord, error) {
	if !p.isSelf(host, id) || p.source == nil {
		return nil, resource.ErrNotFound
	}
	s, ok := p.source()
	if !ok {
		return nil, resource.ErrNotFound
	}
	var records []frameRecord
	for _, raw := range s.contextFrames.ledger.Snapshot() {
		var record frameRecord
		if json.Unmarshal(raw, &record) == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

func findFrame(records []frameRecord, id string) (frameRecord, bool) {
	for _, record := range records {
		if record.Frame.FrameID == id {
			return record, true
		}
	}
	return frameRecord{}, false
}

func frameReceiptLine(record frameRecord) string {
	line := fmt.Sprintf("%-10s %-8s %s", record.State, record.Frame.Kind, record.Frame.Source)
	if record.Frame.Title != "" {
		line += "  " + record.Frame.Title
	}
	return line
}

// renderFrames renders the receipt list, or one frame with its provenance and body.
func (p sessionProvider) renderFrames(host sdk.Context, t sessionTarget) ([]byte, error) {
	records, err := p.frameRecords(host, t.id)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	if t.depth == 2 {
		if len(records) == 0 {
			return []byte("no frames in this session\n"), nil
		}
		for _, record := range records {
			fmt.Fprintf(&out, "%s  %s\n", record.Frame.FrameID, frameReceiptLine(record))
		}
		return []byte(out.String()), nil
	}
	record, ok := findFrame(records, t.frame)
	if !ok {
		return nil, resource.ErrNotFound
	}
	frame := record.Frame
	fmt.Fprintf(&out, "frame %s\nstate %s\nsource %s\nkind %s\ndelivery %s\ntrust %s\nauthority %s\ncreated %s\ndigest %s\n", frame.FrameID, record.State, frame.Source, frame.Kind, frame.Delivery, frame.Trust, frame.Authority, frame.CreatedAt.Local().Format(time.RFC3339), frame.ContentDigest)
	if frame.Title != "" {
		fmt.Fprintf(&out, "title %s\n", frame.Title)
	}
	if record.BodyDropped {
		out.WriteString("\nbody no longer retained in memory; the session entry still holds it\n")
	} else {
		out.WriteString("\n" + frame.Body + "\n")
	}
	return []byte(out.String()), nil
}

// instructionEntries lists the instruction files this session delivered to the model, each linking to
// its file with class, delivery state and pin origin, followed by the ctl entry.
func (p sessionProvider) instructionEntries(host sdk.Context, id string) ([]resource.Entry, error) {
	records, err := p.frameRecords(host, id)
	if err != nil {
		return nil, err
	}
	s, _ := p.source()
	s.contextFrames.mu.Lock()
	pins := append([]instructionPin(nil), s.contextFrames.pins...)
	s.contextFrames.mu.Unlock()
	root := s.location.Current().ProjectRoot
	const base = "session://self/instructions/"
	index := make(map[string]int)
	var entries []resource.Entry
	for _, record := range records {
		frame := record.Frame
		class, isInstruction := strings.CutPrefix(frame.Source, "instructions.")
		if !isInstruction || frame.Kind == "notice" || frame.Resource == "" || record.State != "delivered" && record.State != "pending" {
			continue
		}
		detail := class + " · " + record.State
		for _, pin := range pins {
			if pin.Path == frame.Resource {
				detail += " · pinned by " + pin.Origin
			}
		}
		short := frame.Resource
		if rel, err := filepath.Rel(root, short); err == nil && filepath.IsLocal(rel) {
			short = rel
		}
		name := class + "-" + sanitizeSegment(strings.Trim(short, "/"))
		entry := resource.Entry{Name: name, URI: base + name, Kind: resource.EntryLink, Target: "file://" + frame.Resource, Detail: detail}
		if at, ok := index[name]; ok {
			entries[at] = entry
			continue
		}
		index[name] = len(entries)
		entries = append(entries, entry)
	}
	return append(entries, resource.Entry{Name: "ctl", URI: base + "ctl", Kind: resource.EntryControl, Detail: "write: pin <path> [--class <id>] | unpin <path>"}), nil
}
