package host

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
)

// integrationOwners lists live contributors and pending cached presentation owners.
func (s *session) integrationOwners() []string {
	seen := map[string]bool{}
	for _, registration := range s.contributions.List() {
		seen[registration.Owner] = true
	}
	for _, change := range s.presentations.changes() {
		seen[change.Owner] = true
	}
	owners := make([]string, 0, len(seen))
	for owner := range seen {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	return owners
}

func descriptorSummary(registration contribution.Registration) string {
	var descriptor struct {
		Summary string `json:"summary"`
		Label   string `json:"label"`
		Title   string `json:"title"`
	}
	_ = json.Unmarshal(registration.Descriptor, &descriptor)
	for _, text := range []string{descriptor.Summary, descriptor.Label, descriptor.Title} {
		if text != "" {
			return text
		}
	}
	return ""
}

// integrationsText renders every live registration, or one owner's with full descriptors.
func (s *session) integrationsText(owner string) (string, bool) {
	var out strings.Builder
	found := false
	for _, registration := range s.contributions.List() {
		if owner != "" && registration.Owner != owner {
			continue
		}
		found = true
		fmt.Fprintf(&out, "%s  %s  %s  rev %s", registration.Owner, registration.Kind, registration.Key, shortID(registration.DescriptorRevision))
		if registration.Kind == contribution.KindToolPresentation && s.presentations.appliesAfterReload(registration.RegistrationID) {
			out.WriteString("  appliesAfterReload")
		}
		if summary := descriptorSummary(registration); summary != "" {
			fmt.Fprintf(&out, "  %s", noticeText(summary, 120))
		}
		out.WriteByte('\n')
		if owner != "" {
			pretty, err := json.MarshalIndent(json.RawMessage(registration.Descriptor), "  ", "  ")
			if err == nil {
				fmt.Fprintf(&out, "  %s\n", pretty)
			}
		}
	}
	for _, change := range s.presentations.changes() {
		if change.Change == "withdrawn" && (owner == "" || owner == change.Owner) {
			fmt.Fprintf(&out, "%s  toolPresentation  %s  withdrawn  appliesAfterReload  %s\n", change.Owner, change.Key, change.Tool)
			found = true
		}
	}
	for _, call := range s.presentationData.summaries(owner) {
		state := call.Phase
		if call.Outcome != "" {
			state += "/" + call.Outcome
		}
		fmt.Fprintf(&out, "%s  presentation call %s  %s r%d  %s\n", call.Owner, call.CallID, state, call.Revision, call.Tool)
		if call.Diagnostic != "" {
			fmt.Fprintf(&out, "  %s\n", call.Diagnostic)
		}
		found = true
	}
	if !found {
		if owner != "" {
			return "", false
		}
		return "No extension has registered contributions with Workbench.\n", true
	}
	return out.String(), true
}

// resourcesText lists every scheme with its origin and, for contributed schemes, its capabilities.
func (s *session) resourcesText(scheme string) (string, bool) {
	contributed := map[string]string{}
	for _, registration := range s.contributions.List() {
		if registration.Kind != contribution.KindResourceProvider {
			continue
		}
		var descriptor struct {
			Schemes      []string `json:"schemes"`
			Capabilities []string `json:"capabilities"`
			Summary      string   `json:"summary"`
		}
		if json.Unmarshal(registration.Descriptor, &descriptor) != nil {
			continue
		}
		for _, name := range descriptor.Schemes {
			contributed[name] = fmt.Sprintf("contributed by %s  [%s]  %s", registration.Owner, strings.Join(descriptor.Capabilities, ","), noticeText(descriptor.Summary, 120))
		}
	}
	var out strings.Builder
	found := false
	for _, name := range s.router.Schemes() {
		if scheme != "" && name != strings.TrimSuffix(scheme, "://") {
			continue
		}
		found = true
		origin := "builtin"
		if detail, ok := contributed[name]; ok {
			origin = detail
		}
		fmt.Fprintf(&out, "%s://  %s\n", name, origin)
	}
	if !found {
		return "", false
	}
	return out.String(), true
}

func (a *app) integrationsCommand(ctx sdk.Context, args string) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	text, ok := s.integrationsText(strings.TrimSpace(args))
	if !ok {
		return fmt.Errorf("no live registrations for owner %q", strings.TrimSpace(args))
	}
	notify(ctx, text)
	return nil
}

func (a *app) resourcesCommand(ctx sdk.Context, args string) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	text, ok := s.resourcesText(strings.TrimSpace(args))
	if !ok {
		return fmt.Errorf("scheme %q is not registered", strings.TrimSpace(args))
	}
	notify(ctx, text)
	return nil
}
