package host

import (
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

func TestGroupedDisplayPlainViewFitsRequiredWidthsAndKeepsScope(t *testing.T) {
	groups := []displayGroupView{{Owner: "reference-producer", Key: "web_search", Active: true, Fields: []displayFieldView{{Definition: config.Definition{Key: "tools.reference-producer.web_search.density"}, Effective: config.Entry{Value: strings.Repeat("long", 100), Scope: config.Runtime, Application: "registered"}}}}}
	for _, width := range []int{36, 57, 80, 140} {
		lines := displayGroupLines(groups, width)
		for _, line := range lines {
			if tui.VisibleWidth(line) > width || strings.ContainsRune(line, '\x1b') {
				t.Fatalf("width %d unsafe line: %q", width, line)
			}
		}
		if !strings.Contains(strings.Join(lines, "\n"), "[runtime]") || !strings.Contains(strings.Join(lines, "\n"), "reference-producer / web_search") {
			t.Fatalf("width %d lost identity/provenance: %v", width, lines)
		}
	}
}

func TestDisplayCommandIdentifiersPreserveQuotedSegmentsAndValueTail(t *testing.T) {
	fields, tail, err := commandFields(`runtime "owner with space" 'key.dot' metrics ["count", "bytes"]`, 4)
	if err != nil || len(fields) != 4 || fields[1] != "owner with space" || fields[2] != "key.dot" || tail != `["count", "bytes"]` {
		t.Fatalf("quoted identifiers/value tail: %v %q %v", fields, tail, err)
	}
	for _, bad := range []string{`"unclosed`, `"joined"tail`} {
		if _, _, err := commandFields(bad, 1); err == nil {
			t.Fatalf("ambiguous identifier accepted: %q", bad)
		}
	}
}

func TestDisplayChangeEventsContainOnlyRegistrationDisplayValues(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	receipt := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	snapshot, err := s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	key := "tools.build-tools.build_run.density"
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: key, Value: "compact"}}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	events, failure := a.displayChangeEvents(s, snapshot.Revision(), []string{key, "secrets.enabled"})
	if failure != nil || len(events) != 1 {
		t.Fatalf("display events: %+v %v", events, failure)
	}
	event := events[0]
	payload := event.Data.(api.DisplaySettingsChanged)
	if event.RegistrationID != receipt["registrationId"] || payload.Owner != "build-tools" || payload.Key != "build_run" || payload.ConfigRevision != snapshot.Revision() || payload.Values["density"] != "compact" {
		t.Fatalf("display identity/value not bound: %+v", event)
	}
	if _, exists := payload.Values["secrets.enabled"]; exists {
		t.Fatal("execution policy included in display event")
	}
	if events, failure := a.displayChangeEvents(s, snapshot.Revision(), []string{"secrets.enabled"}); failure != nil || len(events) != 0 {
		t.Fatalf("unrelated settings emitted a display update: %+v %v", events, failure)
	}
}
