package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/messaging"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

const maxInboxWait = 300 * time.Second

// agentProvider exposes live sibling sessions: listing, status, history and the own inbox.
type agentProvider struct{ source func() (*session, bool) }

func (agentProvider) Scheme() string { return "agent" }

func splitAgent(uri string) ([]string, url.Values, error) {
	rest, ok := cutScheme(uri, "agent")
	if !ok {
		return nil, nil, errors.New("agent provider requires an agent:// URI")
	}
	rest, rawQuery, _ := strings.Cut(rest, "?")
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, nil, errors.New("invalid agent:// query")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return nil, query, nil
	}
	parts := strings.Split(rest, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, nil, errors.New("agent:// segment must not be empty, '.' or '..'")
		}
	}
	return parts, query, nil
}

func (p agentProvider) endpoint() (*sessionEndpoint, error) {
	s, ok := p.source()
	if !ok || s == nil || s.endpoint == nil {
		return nil, errNoEndpoint
	}
	return s.endpoint, nil
}

func (p agentProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	parts, _, err := splitAgent(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	if _, err := p.endpoint(); err != nil {
		return resource.StatResult{}, err
	}
	switch {
	case len(parts) <= 1:
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	case len(parts) == 2 && (parts[1] == "status" || parts[1] == "messages" || parts[1] == "inbox"):
		return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
	}
	return resource.StatResult{}, resource.ErrNotFound
}

func (p agentProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	parts, _, err := splitAgent(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	e, err := p.endpoint()
	if err != nil {
		return resource.Listing{}, err
	}
	if len(parts) == 2 || len(parts) > 2 {
		return resource.Listing{}, resource.ErrNotFound
	}
	if len(parts) == 1 {
		entries := []resource.Entry{}
		for _, name := range []string{"inbox", "messages", "status"} {
			entries = append(entries, resource.Entry{Name: name, URI: "agent://" + parts[0] + "/" + name, Kind: resource.EntryFile})
		}
		return resource.Listing{Complete: true, Entries: entries}, nil
	}
	records, err := e.peers(ctx)
	if err != nil {
		return resource.Listing{}, err
	}
	e.mu.Lock()
	own := e.record.ProjectID
	e.mu.Unlock()
	entries := []resource.Entry{{Name: "self", URI: "agent://self/", Kind: resource.EntryDir}}
	for _, record := range records {
		if own != "" && record.ProjectID != own {
			continue
		}
		name := shortID(record.SessionID)
		entries = append(entries, resource.Entry{Name: name, URI: "agent://" + name + "/", Kind: resource.EntryDir, Detail: fmt.Sprintf("%s  %s  %s  %s", record.SessionName, record.ProjectSlug, record.State, record.Cwd)})
	}
	sort.SliceStable(entries[1:], func(i, j int) bool { return entries[1+i].Name < entries[1+j].Name })
	return resource.Listing{Complete: true, Entries: entries}, nil
}

func (p agentProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	parts, query, err := splitAgent(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	e, err := p.endpoint()
	if err != nil {
		return resource.ReadResult{}, err
	}
	if len(parts) != 2 {
		return resource.ReadResult{}, resource.ErrUnsupported
	}
	var body string
	switch parts[1] {
	case "status":
		body, err = e.statusText(ctx, parts[0])
	case "messages":
		body, err = e.messagesText(ctx, parts[0])
	case "inbox":
		if parts[0] != "self" {
			return resource.ReadResult{}, errors.New("agent://<id>/inbox is write-only; read agent://self/inbox")
		}
		body, err = e.inboxText(ctx, query)
	default:
		return resource.ReadResult{}, resource.ErrNotFound
	}
	if err != nil {
		return resource.ReadResult{}, err
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, []byte(body), request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}

func infoText(info messaging.Info) string {
	var out strings.Builder
	fmt.Fprintf(&out, "session: %s\nname: %s\nproject: %s\ncwd: %s\nstate: %s\nmodel: %s\n", info.SessionID, info.SessionName, info.ProjectSlug, info.Cwd, info.State, info.Model)
	if !info.StateSince.IsZero() {
		fmt.Fprintf(&out, "in state since: %s\n", info.StateSince.Format(time.RFC3339))
	}
	return out.String()
}

func (e *sessionEndpoint) statusText(ctx context.Context, target string) (string, error) {
	if target == "self" {
		e.mu.Lock()
		defer e.mu.Unlock()
		text := infoText(e.infoLocked())
		if len(e.pending) > 0 {
			text += "waiting for an answer in:\n"
			for conversation, peer := range e.pending {
				text += "- conversation " + shortID(conversation) + " with " + shortID(peer) + "\n"
			}
		}
		return text, nil
	}
	record, err := e.resolve(ctx, target)
	if err != nil {
		return "", err
	}
	dir, err := endpointDirectory()
	if err != nil {
		return "", err
	}
	reply, err := messaging.Call(filepath.Join(dir, record.Endpoint+".sock"), record.Token, e.id, messaging.Request{Type: "info"})
	if err != nil || reply.Info == nil {
		return "", fmt.Errorf("session %s is unreachable", shortID(record.SessionID))
	}
	text := infoText(*reply.Info)
	e.mu.Lock()
	text += fmt.Sprintf("blocked: %t\ntrusted: %t\n", e.blocked[record.SessionID], e.trusted[record.SessionID])
	e.mu.Unlock()
	return text, nil
}

func (e *sessionEndpoint) messagesText(ctx context.Context, target string) (string, error) {
	peer := ""
	if target != "self" {
		record, err := e.resolve(ctx, target)
		if err != nil {
			return "", err
		}
		peer = record.SessionID
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	var out strings.Builder
	for _, entry := range e.log {
		if peer != "" && entry.Peer != peer {
			continue
		}
		arrow := "from"
		if entry.Outbound {
			arrow = "to"
		}
		fmt.Fprintf(&out, "%s %s %s", entry.At.Format("15:04:05"), arrow, shortID(entry.Peer))
		if entry.Envelope.Subject != "" {
			fmt.Fprintf(&out, " [%s]", entry.Envelope.Subject)
		}
		if entry.Delivery != "" {
			fmt.Fprintf(&out, " (%s)", entry.Delivery)
		}
		out.WriteString(": " + entry.Envelope.Body + "\n")
	}
	if out.Len() == 0 {
		return "No messages.\n", nil
	}
	return out.String(), nil
}

// inboxText waits for the next peer message when ?wait=<seconds> is given. Messages that arrive
// outside a wait are delivered to the model as frames and are not repeated here.
func (e *sessionEndpoint) inboxText(ctx context.Context, query url.Values) (string, error) {
	wait := time.Duration(0)
	if raw := query.Get("wait"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 0 {
			return "", errors.New("wait must be a number of seconds")
		}
		wait = min(time.Duration(seconds)*time.Second, maxInboxWait)
	}
	from := query.Get("from")
	if from != "" {
		record, err := e.resolve(ctx, from)
		if err != nil {
			return "", err
		}
		from = record.SessionID
	}
	for {
		env, err := e.wait(ctx, from, wait)
		if err != nil {
			return "", err
		}
		if env == nil {
			return "No message arrived.\n", nil
		}
		if env.Progress {
			start := time.Now()
			text := "Progress from " + peerLabel(env.From) + ": " + env.Body + "\n"
			wait = max(wait-time.Since(start), 5*time.Second)
			next, err := e.inboxText(ctx, url.Values{"wait": {strconv.Itoa(int(wait / time.Second))}, "from": {env.From.SessionID}})
			return text + next, err
		}
		return renderMessage(*env) + "\n", nil
	}
}

// agentWrite sends the written content to agent://<id>/inbox.
func (a *app) agentWrite(ctx context.Context, s *session, uri, content string) (string, messaging.Receipt, error) {
	parts, _, err := splitAgent(uri)
	if err != nil {
		return "", messaging.Receipt{}, err
	}
	if len(parts) != 2 || parts[1] != "inbox" || parts[0] == "self" {
		return "", messaging.Receipt{}, errors.New("agent:// accepts writes only at agent://<id>/inbox")
	}
	if s.endpoint == nil {
		return "", messaging.Receipt{}, errNoEndpoint
	}
	message := outbound{Body: content}
	if trimmed := strings.TrimSpace(content); strings.HasPrefix(trimmed, "{") {
		var parsed outbound
		if json.Unmarshal([]byte(trimmed), &parsed) == nil && parsed.Body != "" {
			message = parsed
		}
	}
	receipt, err := s.endpoint.send(ctx, parts[0], message)
	if err != nil {
		return "", receipt, err
	}
	return receiptText(parts[0], receipt), receipt, nil
}

func receiptText(target string, receipt messaging.Receipt) string {
	if receipt.Status != messaging.Accepted {
		return "rejected by " + target + ": " + receipt.Reason
	}
	return fmt.Sprintf("accepted by %s (%s); message %s, conversation %s", target, receipt.Delivery, shortID(receipt.MessageID), shortID(receipt.ConversationID))
}
