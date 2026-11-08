package host

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// peerRow is one live peer with the policy flags this session set on it.
type peerRow struct{ id, key, name, project, state, cwd, flags string }

// peerRows lists the other live sessions and whether this one is hidden.
func (e *sessionEndpoint) peerRows(ctx context.Context) ([]peerRow, bool, error) {
	records, err := e.peers(ctx)
	if err != nil {
		return nil, false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	rows := make([]peerRow, 0, len(records))
	for _, record := range records {
		flags := ""
		if e.trusted[record.SessionID] {
			flags += " trusted"
		}
		if e.blocked[record.SessionID] {
			flags += " blocked"
		}
		rows = append(rows, peerRow{id: shortID(record.SessionID), key: record.SessionID, name: noticeText(record.SessionName, 24), project: record.ProjectSlug, state: record.State, cwd: record.Cwd, flags: strings.TrimSpace(flags)})
	}
	return rows, e.hidden, nil
}

// sessionsCommand implements /workbench sessions [ls|send|trust|untrust|block|unblock|hide|show].
func (a *app) sessionsCommand(ctx sdk.Context, args string) error {
	if strings.TrimSpace(args) == "" && panelsAvailable(ctx) {
		return a.sessionsPanel(ctx)
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	e := s.endpoint
	if e == nil {
		return errNoEndpoint
	}
	verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
	rest = strings.TrimSpace(rest)
	if verb == "" || verb == "ls" {
		rows, hidden, err := e.peerRows(work)
		if err != nil {
			return err
		}
		var out strings.Builder
		fmt.Fprintf(&out, "This session %s is %s.\n", shortID(s.id), map[bool]string{true: "hidden", false: "discoverable"}[hidden])
		for _, row := range rows {
			fmt.Fprintf(&out, "%s  %-24s %-12s %-14s %s %s\n", row.id, row.name, row.project, row.state, row.cwd, row.flags)
		}
		if len(rows) == 0 {
			out.WriteString("No other live session is discoverable.\n")
		}
		a.notify(ctx, out.String())
		return nil
	}
	text, err := a.sessionsAct(ctx, work, e, verb, rest)
	if err != nil {
		return err
	}
	a.notify(ctx, text)
	return nil
}

// sessionsAct runs one state-changing sessions verb and returns its confirmation text.
func (a *app) sessionsAct(ctx sdk.Context, work context.Context, e *sessionEndpoint, verb, rest string) (string, error) {
	switch verb {
	case "send":
		target, text, _ := strings.Cut(rest, " ")
		if target == "" || strings.TrimSpace(text) == "" {
			return "", errors.New("usage: /workbench sessions send <session> <text>")
		}
		receipt, err := e.send(work, target, outbound{Body: strings.TrimSpace(text), user: true})
		if err != nil {
			return "", err
		}
		return receiptText(target, receipt), nil
	case "trust", "untrust", "block", "unblock":
		if rest == "" {
			return "", fmt.Errorf("usage: /workbench sessions %s <session>", verb)
		}
		id, err := e.peerKey(work, rest)
		if err != nil {
			return "", err
		}
		e.mu.Lock()
		switch verb {
		case "trust":
			e.trusted[id] = true
		case "untrust":
			delete(e.trusted, id)
		case "block":
			e.blocked[id] = true
		case "unblock":
			delete(e.blocked, id)
		}
		e.mu.Unlock()
		return fmt.Sprintf("%s: %s", verb, shortID(id)), nil
	case "hide", "show":
		e.mu.Lock()
		e.hidden = verb == "hide"
		e.refreshLocked(ctx)
		e.publishLocked(ctx)
		e.mu.Unlock()
		return "This session is now " + map[bool]string{true: "hidden from discovery (existing conversations keep working)", false: "discoverable"}[verb == "hide"] + ".", nil
	}
	return "", errors.New("usage: /workbench sessions [ls | send <session> <text> | trust|untrust|block|unblock <session> | hide | show]")
}

// sessionsPanel opens the live-session list; each action admits and runs like the command.
func (a *app) sessionsPanel(ctx sdk.Context) error {
	title, _ := a.hubTitle.Load().(string)
	if title == "" {
		title = "Workbench › Sessions"
	}
	own, err := a.current()
	if err != nil {
		return err
	}
	if own.endpoint == nil {
		return errNoEndpoint
	}
	panel := &sessionsPanel{title: title, height: ctx.Height, self: shortID(own.id)}
	panel.rows = func() ([]peerRow, bool, error) {
		s, work, release, err := a.admit(ctx)
		if err != nil {
			return nil, false, err
		}
		defer release()
		return s.endpoint.peerRows(work)
	}
	panel.act = func(verb, rest string) (string, error) {
		s, work, release, err := a.admit(ctx)
		if err != nil {
			return "", err
		}
		defer release()
		return a.sessionsAct(ctx, work, s.endpoint, verb, rest)
	}
	_, err = ctx.Custom(panel, nil)
	return err
}

// peerKey resolves a live peer, or accepts a full session or project ID that is not live (to block it ahead of time).
func (e *sessionEndpoint) peerKey(ctx context.Context, name string) (string, error) {
	if record, err := e.resolve(ctx, name); err == nil {
		return record.SessionID, nil
	}
	if strings.HasPrefix(name, "prj-") || len(name) >= 32 {
		return name, nil
	}
	return "", fmt.Errorf("no live session matches %q", name)
}
