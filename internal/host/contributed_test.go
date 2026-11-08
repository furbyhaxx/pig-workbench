package host

import (
	"encoding/json"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/api"
)

func TestFetchBackendSelectionPrefersMostSpecificRule(t *testing.T) {
	router := &fetchRouter{}
	for _, backend := range []fetchBackend{
		{key: "catch-all", descriptor: fetchDescriptor{Match: []fetchMatch{{Host: "*"}}}},
		{key: "suffix", descriptor: fetchDescriptor{Match: []fetchMatch{{Host: "*.example.test"}}}},
		{key: "exact", descriptor: fetchDescriptor{Match: []fetchMatch{{Host: "docs.example.test"}}}},
		{key: "exact-path", descriptor: fetchDescriptor{Match: []fetchMatch{{Host: "docs.example.test", PathPrefix: "/api/"}}}},
		{key: "late-exact", descriptor: fetchDescriptor{Match: []fetchMatch{{Host: "docs.example.test"}}}},
	} {
		router.add(backend)
	}
	cases := map[string]string{
		"https://docs.example.test/api/v1": "exact-path",
		"https://docs.example.test/guide":  "exact",
		"https://www.example.test/":        "suffix",
		"https://example.test/":            "catch-all",
		"https://elsewhere.test/":          "catch-all",
	}
	for url, want := range cases {
		got, ok := router.selectBackend(url)
		if !ok || got.key != want {
			t.Errorf("%s selected %q (%v); want %q", url, got.key, ok, want)
		}
	}
	router.remove("")
	empty := &fetchRouter{}
	if _, ok := empty.selectBackend("https://x.test/"); ok {
		t.Fatal("empty router matched")
	}
}

func TestFetchBackendDescriptorValidation(t *testing.T) {
	valid := `{"label":"Reader","match":[{"host":"*.example.test","pathPrefix":"/docs"}],"capabilities":["fetch","locate"],"egress":"thirdParty","retention":"user","limits":{"bodyBytes":1000}}`
	if _, err := parseFetchBackend([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string]string{
		"no fetch":   `{"label":"R","match":[{"host":"*"}],"capabilities":["locate"],"egress":"none","limits":{}}`,
		"bad egress": `{"label":"R","match":[{"host":"*"}],"capabilities":["fetch"],"egress":"maybe","limits":{}}`,
		"bad host":   `{"label":"R","match":[{"host":"a/b"}],"capabilities":["fetch"],"egress":"none","limits":{}}`,
		"no rules":   `{"label":"R","match":[],"capabilities":["fetch"],"egress":"none","limits":{}}`,
	} {
		if _, err := parseFetchBackend([]byte(payload)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestStatusSegmentRenderFitsAndNamesSeverity(t *testing.T) {
	count := 3
	segment := statusSegment{descriptor: statusDescriptor{Label: "web", MaxColumns: 24, Glyph: "◆", MinColumns: 30}, value: statusValue{Text: "searching a very long query that never ends here", Style: "warning"}}
	for _, width := range []int{36, 57, 80, 140} {
		if got := renderStatus(segment, width); tui.VisibleWidth(got) > min(24, width/3) {
			t.Errorf("width %d: %q overflows", width, got)
		}
	}
	if got := renderStatus(segment, 40); got != "◆" {
		t.Errorf("narrow terminal = %q; want glyph", got)
	}
	segment.value = statusValue{Count: &count, Severity: "error"}
	if got := renderStatus(segment, 140); got != "3 web (error)" {
		t.Errorf("count render = %q", got)
	}
	zero := 0
	segment.value = statusValue{Count: &zero}
	if got := renderStatus(segment, 140); got != "" {
		t.Errorf("zero count = %q; want hidden", got)
	}
}

func TestFrameProducerDescriptorRejectsInternalAndUnknownFields(t *testing.T) {
	if _, err := parseFrameProducer([]byte(`{"label":"R","kinds":["notice"],"contextFields":[{"source":"session.model","field":"knowledge_cutoff"}]}`)); err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string]string{
		"message kind":  `{"label":"R","kinds":["message"]}`,
		"unknown field": `{"label":"R","kinds":["notice"],"contextFields":[{"source":"session.model","field":"name"}]}`,
		"other source":  `{"label":"R","kinds":["notice"],"contextFields":[{"source":"session.date","field":"knowledge_cutoff"}]}`,
	} {
		if _, err := parseFrameProducer([]byte(payload)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestContributedFieldValuesRefuseInvalidClaims(t *testing.T) {
	producer := frameProducer{owner: api.Identity{Owner: "web"}, descriptor: frameProducerDescriptor{ContextFields: []contextFieldClaim{{Source: "session.model", Field: "knowledge_cutoff"}}}}
	field := func(payload string) map[string]json.RawMessage {
		return map[string]json.RawMessage{"knowledge_cutoff": json.RawMessage(payload)}
	}
	good := `{"date":"2025-06-01","basis":"estimated","source":"vendor card"}`
	values, failure := contributedFieldValues(producer, "session.model", field(good))
	if failure != nil {
		t.Fatalf("valid claim refused: %+v", failure)
	}
	cutoff := values["knowledge_cutoff"].(map[string]any)
	if cutoff["basis"] != "estimated" || cutoff["contributedBy"] != "web" {
		t.Fatalf("basis and owner must stay visible: %v", cutoff)
	}
	for name, test := range map[string]struct {
		source string
		fields map[string]json.RawMessage
		code   string
	}{
		"unclaimed field":   {"session.model", map[string]json.RawMessage{"name": json.RawMessage(`"x"`)}, "permissionDenied"},
		"wrong source":      {"session.date", field(good), "permissionDenied"},
		"impossible date":   {"session.model", field(`{"date":"2025-02-30","basis":"explicit","source":"s"}`), "invalidRequest"},
		"loose date":        {"session.model", field(`{"date":"2025-6-1","basis":"explicit","source":"s"}`), "invalidRequest"},
		"unknown basis":     {"session.model", field(`{"date":"2025-06-01","basis":"guess","source":"s"}`), "invalidRequest"},
		"control in source": {"session.model", field(`{"date":"2025-06-01","basis":"explicit","source":"a\u001b[31m"}`), "invalidRequest"},
	} {
		if _, failure := contributedFieldValues(producer, test.source, test.fields); failure == nil || failure.Code != test.code {
			t.Errorf("%s: failure = %+v, want %s", name, failure, test.code)
		}
	}
}

func TestContributedFieldsApplyOnlyToTheirModel(t *testing.T) {
	registry := newProducerRegistry()
	bound := modelBinding{Provider: "p", ID: "m1"}
	registry.fields["reg"] = contributedFields{owner: "web", registrationID: "reg", model: bound, fields: map[string]any{"knowledge_cutoff": 1}}
	if got := registry.modelFields(bound); len(got) != 1 {
		t.Fatalf("bound model lost its fields: %v", got)
	}
	if got := registry.modelFields(modelBinding{Provider: "p", ID: "m2"}); len(got) != 0 {
		t.Fatalf("a model change must not inherit stale data: %v", got)
	}
}
