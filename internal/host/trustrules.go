package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

// trustRule is one `allow <dir>` or `deny <dir>` entry of trust.rules.
type trustRule struct {
	allow bool
	dir   string
}

func parseTrustRule(text string) (trustRule, error) {
	verb, dir, _ := strings.Cut(strings.TrimSpace(text), " ")
	dir = strings.TrimSpace(dir)
	if (verb != "allow" && verb != "deny") || dir == "" {
		return trustRule{}, errors.New("a trust rule is `allow <absolute directory>` or `deny <absolute directory>`")
	}
	if !filepath.IsAbs(dir) {
		return trustRule{}, fmt.Errorf("trust rule directory %q must be absolute", dir)
	}
	return trustRule{allow: verb == "allow", dir: filepath.Clean(dir)}, nil
}

func (r trustRule) String() string {
	verb := "deny"
	if r.allow {
		verb = "allow"
	}
	return verb + " " + r.dir
}

func within(dir, root string) bool {
	rel, err := filepath.Rel(root, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// decideTrust returns whether dir may be crossed: the most specific rule wins, a deny beats an allow at the same depth, and no match is not an allow.
func decideTrust(rules []trustRule, dir string) (allowed, matched bool) {
	dir = filepath.Clean(dir)
	best := -1
	for _, rule := range rules {
		if !within(dir, rule.dir) {
			continue
		}
		depth := len(rule.dir)
		switch {
		case depth > best:
			best, allowed, matched = depth, rule.allow, true
		case depth == best && !rule.allow:
			allowed = false
		}
	}
	return allowed, matched
}

func trustRulesFrom(snapshot *config.Snapshot) ([]trustRule, []string) {
	entry, ok := snapshot.Get("trust.rules")
	if !ok {
		return nil, nil
	}
	list, _ := entry.Value.([]string)
	var rules []trustRule
	var invalid []string
	for _, text := range list {
		rule, err := parseTrustRule(text)
		if err != nil {
			invalid = append(invalid, text)
			continue
		}
		rules = append(rules, rule)
	}
	return rules, invalid
}

// trustAllows reports whether a user rule lets instructions of the nested repository at dir load.
func trustAllows(settings *config.Manager, dir string) bool {
	snapshot, err := settings.Current()
	if err != nil {
		return false
	}
	rules, _ := trustRulesFrom(snapshot)
	allowed, _ := decideTrust(rules, dir)
	return allowed
}

// trustCommand implements /workbench trust [ls|add allow|deny <dir>|rm <dir>].
func (a *app) trustCommand(ctx sdk.Context, args string) error {
	if strings.TrimSpace(args) == "" && panelsAvailable(ctx) {
		return a.trustPanel(ctx)
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
	rest = strings.TrimSpace(rest)
	baseline, err := s.settings.Current()
	if err != nil {
		return err
	}
	rules, invalid := trustRulesFrom(baseline)
	switch verb {
	case "", "ls":
		var out strings.Builder
		out.WriteString("Workbench trust rules (not PiG's /trust; the nearest rule wins, deny beats allow at equal depth):\n")
		for _, rule := range rules {
			fmt.Fprintf(&out, "- %s\n", rule)
		}
		if len(rules) == 0 {
			out.WriteString(emptyState("trust rules", "Nested repositories stay a boundary.") + "\n")
		}
		for _, text := range invalid {
			fmt.Fprintf(&out, "! ignored malformed rule: %s\n", noticeText(text, 80))
		}
		a.notify(ctx, out.String())
		return nil
	case "add", "rm":
	default:
		return errors.New("usage: /workbench trust [ls | add allow|deny <absolute dir> | rm <absolute dir>]")
	}
	count, err := a.editTrustRules(ctx, s, work, baseline, verb, rest)
	if err != nil {
		return err
	}
	a.notify(ctx, fmt.Sprintf("Trust rules saved to user settings (%d rule(s)). This does not change PiG's own /trust store.", count))
	return nil
}

// editTrustRules applies one add or rm to the user-scope rules and returns the rule count saved.
func (a *app) editTrustRules(ctx sdk.Context, s *session, work context.Context, baseline *config.Snapshot, verb, rest string) (int, error) {
	var current []string
	if entry, ok := baseline.Get("trust.rules"); ok {
		current, _ = entry.Value.([]string)
	}
	updated := slices.Clone(current)
	switch verb {
	case "add":
		rule, err := parseTrustRule(rest)
		if err != nil {
			return 0, err
		}
		updated = slices.DeleteFunc(updated, func(text string) bool {
			existing, err := parseTrustRule(text)
			return err == nil && existing.dir == rule.dir
		})
		updated = append(updated, rule.String())
	case "rm":
		dir := filepath.Clean(rest)
		if rest == "" || !filepath.IsAbs(dir) {
			return 0, errors.New("usage: /workbench trust rm <absolute dir>")
		}
		before := len(updated)
		updated = slices.DeleteFunc(updated, func(text string) bool {
			existing, err := parseTrustRule(text)
			return err == nil && existing.dir == dir
		})
		if len(updated) == before {
			return 0, fmt.Errorf("no trust rule for %s", dir)
		}
	}
	result, saveErr := s.settings.Save(work, config.User, baseline, []config.Edit{{Key: "trust.rules", Value: updated}}, ctx.IsProjectTrusted)
	if result.Published {
		a.settingsChanged(ctx, s, result.Revision, result.Changed)
	}
	if saveErr != nil {
		return 0, saveErr
	}
	return len(updated), nil
}

// trustPanel lists the user's trust rules; each add or remove admits and saves like the command.
func (a *app) trustPanel(ctx sdk.Context) error {
	title, _ := a.hubTitle.Load().(string)
	if title == "" {
		title = "Workbench › Trust"
	}
	rules := func() ([]string, []string) {
		s, err := a.current()
		if err != nil {
			return nil, nil
		}
		snapshot, err := s.settings.Current()
		if err != nil {
			return nil, nil
		}
		parsed, invalid := trustRulesFrom(snapshot)
		texts := make([]string, len(parsed))
		for i, rule := range parsed {
			texts[i] = rule.String()
		}
		return texts, invalid
	}
	edit := func(verb, rest string) error {
		s, work, release, err := a.admit(ctx)
		if err != nil {
			return err
		}
		defer release()
		baseline, err := s.settings.Current()
		if err != nil {
			return err
		}
		_, err = a.editTrustRules(ctx, s, work, baseline, verb, rest)
		return err
	}
	_, err := ctx.Custom(&trustPanel{title: title, rules: rules, edit: edit, height: ctx.Height}, nil)
	return err
}
