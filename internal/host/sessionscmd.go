package host

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// sessionsCommand implements /workbench sessions [ls|send|trust|untrust|block|unblock|hide|show].
func (a *app) sessionsCommand(ctx sdk.Context, args string) error {
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
	switch verb {
	case "", "ls":
		records, err := e.peers(work)
		if err != nil {
			return err
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		var out strings.Builder
		fmt.Fprintf(&out, "This session %s is %s.\n", shortID(s.id), map[bool]string{true: "hidden", false: "discoverable"}[e.hidden])
		for _, record := range records {
			flags := ""
			if e.trusted[record.SessionID] {
				flags += " trusted"
			}
			if e.blocked[record.SessionID] {
				flags += " blocked"
			}
			fmt.Fprintf(&out, "%s  %-24s %-12s %-14s %s%s\n", shortID(record.SessionID), noticeText(record.SessionName, 24), record.ProjectSlug, record.State, record.Cwd, flags)
		}
		if len(records) == 0 {
			out.WriteString("No other live session is discoverable.\n")
		}
		a.notify(ctx, out.String())
		return nil
	case "send":
		target, text, _ := strings.Cut(rest, " ")
		if target == "" || strings.TrimSpace(text) == "" {
			return errors.New("usage: /workbench sessions send <session> <text>")
		}
		receipt, err := e.send(work, target, outbound{Body: strings.TrimSpace(text), user: true})
		if err != nil {
			return err
		}
		a.notify(ctx, receiptText(target, receipt))
		return nil
	case "trust", "untrust", "block", "unblock":
		if rest == "" {
			return fmt.Errorf("usage: /workbench sessions %s <session>", verb)
		}
		id, err := e.peerKey(work, rest)
		if err != nil {
			return err
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
		a.notify(ctx, fmt.Sprintf("%s: %s", verb, shortID(id)))
		return nil
	case "hide", "show":
		e.mu.Lock()
		e.hidden = verb == "hide"
		e.refreshLocked(ctx)
		e.publishLocked(ctx)
		e.mu.Unlock()
		a.notify(ctx, "This session is now "+map[bool]string{true: "hidden from discovery (existing conversations keep working)", false: "discoverable"}[verb == "hide"]+".")
		return nil
	}
	return errors.New("usage: /workbench sessions [ls | send <session> <text> | trust|untrust|block|unblock <session> | hide | show]")
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
