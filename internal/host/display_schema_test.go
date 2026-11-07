package host

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"

	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

func TestPreviewCountsRenderedBodyLinesInsteadOfBlocks(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	a.session = s
	registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	renderers := a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return nil })
	snapshot, _ := s.settings.Current()
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: "tools.build-tools.build_run.preview-lines", Value: int64(1)}}, nil); err != nil {
		t.Fatal(err)
	}
	details := map[string]any{"workbenchPresentation": map[string]any{"version": 1, "revision": 1, "phase": "settled", "outcome": "succeeded", "title": "Build", "blocks": []any{map[string]any{"kind": "text", "text": "first body line\nsecond body line\nthird body line"}}}}
	lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 140)
	text := strings.Join(lines, "\n")
	if err != nil || !strings.Contains(text, "first body line") || strings.Contains(text, "second body line") || !strings.Contains(text, "2 more presentation lines") {
		t.Fatalf("preview budget counted blocks rather than rendered lines: %v %v", lines, err)
	}
}

func TestDisabledPresentationDelegatesAtFirstResolutionAndReportsLaterEnable(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	a.session = s
	receipt := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	snapshot, _ := s.settings.Current()
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: "tools.build-tools.build_run.enabled", Value: false}}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = s.settings.Current()
	s.refreshPresentationDisplay(snapshot)
	next := &sdk.ToolRenderers{}
	if got := a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return next }); got != next {
		t.Fatal("disabled presentation did not delegate without changing execution")
	}
	snapshot, _ = s.settings.Current()
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: "tools.build-tools.build_run.enabled", Value: true}}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = s.settings.Current()
	s.refreshPresentationDisplay(snapshot)
	if !s.presentations.appliesAfterReload(receipt["registrationId"].(string)) {
		t.Fatal("enabling after fallback resolution hid the host cache limitation")
	}
}

func TestSavedDisplayValuesAffectCachedRendererWithoutProducerExecution(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	a.session = s
	registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	renderers := a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return nil })
	details := map[string]any{"workbenchPresentation": map[string]any{"version": 1, "revision": 1, "phase": "settled", "outcome": "succeeded", "title": "Build", "complete": true, "metrics": []any{map[string]any{"id": "durationMs", "value": 1280, "unit": "ms"}}, "blocks": []any{map[string]any{"kind": "text", "text": "visible producer block"}}}}
	before, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 140)
	if err != nil || !strings.Contains(strings.Join(before, "\n"), "visible producer block") {
		t.Fatalf("initial renderer: %v %v", before, err)
	}
	snapshot, _ := s.settings.Current()
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: "tools.build-tools.build_run.preview-lines", Value: int64(0)}, {Key: "tools.build-tools.build_run.metrics", Value: []string{}}}, nil); err != nil {
		t.Fatal(err)
	}
	after, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 140)
	if err != nil || strings.Contains(strings.Join(after, "\n"), "visible producer block") || strings.Contains(strings.Join(after, "\n"), "duration") {
		t.Fatalf("cached renderer ignored effective display values: %v %v", after, err)
	}
	snapshot, _ = s.settings.Current()
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: "tools.build-tools.build_run.enabled", Value: false}}, nil); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = s.settings.Current()
	s.refreshPresentationDisplay(snapshot)
	disabled, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details, Content: []map[string]any{{"type": "text", "text": "retained raw output"}}}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 140)
	if err != nil || !strings.Contains(strings.Join(disabled, "\n"), "retained raw output") || !strings.Contains(strings.Join(disabled, "\n"), "failed (host)") || !strings.Contains(strings.Join(disabled, "\n"), "appliesAfterReload") {
		t.Fatalf("disabled cached renderer lost host/raw truth: %v %v", disabled, err)
	}
	if !s.presentations.appliesAfterReload(s.presentations.list()[0].registrationID) {
		t.Fatal("disabled cached claim did not report reload delegation")
	}
}

func TestSettingsGetReturnsOnlyDeclaredOwnerDisplayFields(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	receipt := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	request := api.Request{Method: "settings.get", Generation: "generation", RegistrationID: receipt["registrationId"].(string), From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: json.RawMessage(`{}`)}
	result, failure := a.dispatchService(t.Context(), sdk.Context{}, s, request)
	if failure != nil {
		t.Fatal(failure)
	}
	settings := result.(api.DisplaySettings)
	if settings.Owner != "build-tools" || settings.Key != "build_run" || settings.Fields["density"].Value != "normal" || settings.ConfigRevision == 0 {
		t.Fatalf("display settings response: %+v", settings)
	}
	if _, exists := settings.Fields["secrets.enabled"]; exists {
		t.Fatal("execution policy leaked through display settings")
	}
	a.session = s
	items, err := a.workbenchCompletions("settings show tools.build-tools.build_run.den")
	if err != nil || len(items) != 1 || items[0].Value != "settings show tools.build-tools.build_run.density" {
		t.Fatalf("dynamic completion missing: %+v %v", items, err)
	}
	request.From.Owner = "foreign"
	if _, failure := a.dispatchService(t.Context(), sdk.Context{}, s, request); failure == nil || failure.Code != "notFound" {
		t.Fatalf("foreign owner read another producer's settings: %v", failure)
	}
}

func TestPresentationSchemaReplacementDoesNotReinterpretSavedTypes(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	original := strings.TrimSuffix(buildPresentationDescriptor, "}") + `,"settings":[{"key":"extra","label":"Extra","type":"boolean","default":true}]}`
	first := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", original)
	snapshot, err := s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	key := "tools.build-tools.build_run.extra"
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: key, Value: false}}, nil); err != nil {
		t.Fatal(err)
	}
	changed := strings.TrimSuffix(buildPresentationDescriptor, "}") + `,"settings":[{"key":"extra","label":"Extra","type":"integer","minimum":0,"maximum":1,"default":1}]}`
	params, _ := json.Marshal(registrationParams{Kind: "toolPresentation", Key: "build_run", Descriptor: json.RawMessage(changed), PreviousRevision: first["descriptorRevision"].(string)})
	if _, failure := a.registerContribution(presentationTestContext(), s, api.Request{Method: "register", RegistrationID: first["registrationId"].(string), Generation: "generation", From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: params}); failure == nil {
		t.Fatal("descriptor changed the meaning of a saved boolean")
	}
	snapshot, err = s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := snapshot.Get(key); value.Value != false {
		t.Fatalf("failed replacement rewrote preference: %+v", value)
	}
	entry, ok := s.presentations.lookup("build_run")
	if !ok || entry.descriptor.Settings[0].Type != "boolean" {
		t.Fatalf("failed schema replacement lost prior presentation: %+v", entry)
	}
}

func TestPresentationSchemasUseStableDefaultsAndKeepWithdrawnOverrides(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	first := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	snapshot, err := s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	key := "tools.build-tools.build_run.density"
	if value, ok := snapshot.Get(key); !ok || value.Value != "normal" {
		t.Fatalf("producer defaults not activated: %+v %v", value, ok)
	}
	if _, err := s.settings.Save(t.Context(), config.Runtime, snapshot, []config.Edit{{Key: key, Value: "compact"}}, nil); err != nil {
		t.Fatal(err)
	}
	request := api.Request{Method: "unregister", Generation: "generation", RegistrationID: first["registrationId"].(string), From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: json.RawMessage(`{}`)}
	if _, failure := a.unregisterContribution(context.Background(), s, request); failure != nil {
		t.Fatal(failure)
	}
	snapshot, err = s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	if value, ok := snapshot.Get(key); !ok || value.Value != "compact" || value.Application != "unavailable" {
		t.Fatalf("withdrawal erased active override or schema status: %+v %v", value, ok)
	}
	second := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	if first["registrationId"] == second["registrationId"] {
		t.Fatal("fixture did not rotate the runtime registration")
	}
	snapshot, err = s.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := snapshot.Get(key); value.Value != "compact" || value.Application != "live" {
		t.Fatalf("stable-key re-registration lost preference: %+v", value)
	}
}
