package host

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

const buildPresentationDescriptor = `{"toolNames":["build_run"],"label":"Build","presentationVersion":1,"preset":"process","defaultDisplay":{"density":"normal","previewLines":4},"metricDefinitions":[{"id":"durationMs","label":"duration","unit":"ms"}]}`

func presentationSession(t *testing.T) *session {
	t.Helper()
	registry, err := contribution.New("generation", contribution.Options{Validator: validateContributionDescriptor})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	settings, err := config.New(config.Paths{User: filepath.Join(root, "user.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	return &session{id: "session", generation: "generation", contributions: registry, presentations: newPresentationCatalog(), settings: settings, router: resource.NewRouter()}
}

func presentationTestContext() context.Context {
	return context.WithValue(context.Background(), presentationToolsKey{}, []sdk.ToolInfo{{Name: "build_run", Source: "build-tools"}, {Name: "web_search", Source: "pig-web"}})
}

func registerPresentation(t *testing.T, a *app, s *session, owner, instance, key, descriptor string) map[string]any {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"kind": "toolPresentation", "key": key, "descriptor": json.RawMessage(descriptor)})
	result, failure := a.registerContribution(presentationTestContext(), s, api.Request{Method: "register", Generation: "generation", From: api.Identity{Owner: owner, Instance: instance}, Params: params})
	if failure != nil {
		t.Fatalf("register: %v", failure)
	}
	return result.(map[string]any)
}

func TestToolPresentationRegistrationDrivesTheResolver(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	receipt := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	if receipt["registrationId"] == "" || receipt["descriptorRevision"] == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	entry, ok := s.presentations.lookup("build_run")
	if !ok || entry.owner != "build-tools" || entry.descriptor.Label != "Build" {
		t.Fatalf("catalog entry = %#v, present %v", entry, ok)
	}
	a.session = s
	nextCalls := 0
	next := func() *sdk.ToolRenderers {
		nextCalls++
		return &sdk.ToolRenderers{}
	}
	renderers := a.resolveToolRenderer("build_run", next)
	if renderers == nil || renderers.Call == nil || renderers.Result == nil || nextCalls != 0 {
		t.Fatalf("registered renderer not selected: %#v", renderers)
	}
	if other := a.resolveToolRenderer("web_search", next); other == nil || nextCalls != 1 {
		t.Fatal("unregistered tool did not fall through to next")
	}
	call, err := renderers.Call(sdk.Context{}, map[string]any{"target": "packages/app"}, sdk.ToolRenderContext{}, 140)
	if err != nil || len(call) != 1 || !strings.Contains(call[0], "Build") {
		t.Fatalf("call render = %#v, %v", call, err)
	}
	details := map[string]any{"workbenchPresentation": map[string]any{
		"version": 1, "revision": 2, "phase": "settled", "outcome": "failed",
		"title": "Build app", "target": map[string]any{"kind": "path", "value": "packages/app"},
		"summary": "Compiler rejected two files", "complete": false, "effects": "partial",
		"metrics": []any{map[string]any{"id": "durationMs", "value": 1280, "unit": "ms"}},
		"blocks":  []any{map[string]any{"kind": "text", "text": "src/main.go:42: undefined: serve", "role": "error"}},
		"error":   map[string]any{"code": "compileFailed", "message": "Fix the compiler errors and retry."},
	}}
	result, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 140)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(result, "\n")
	for _, want := range []string{"Build app", "packages/app", "failed", "incomplete", "partial effects", "duration 1.28s", "undefined: serve", "compileFailed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("result render lacks %q:\n%s", want, joined)
		}
	}
	for _, width := range []int{36, 57, 80, 140} {
		lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, width)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range lines {
			if tui.VisibleWidth(line) > width {
				t.Fatalf("width %d overflows: %q", width, line)
			}
		}
	}
	// A host error cannot be turned into producer success.
	lines, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Details: details}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{IsError: true}, 80)
	if err != nil || strings.Contains(strings.Join(lines, "\n"), "· succeeded") {
		t.Fatalf("host error was presented as success: %#v", lines)
	}
	// Missing or malformed details keeps a usable generic result.
	fallback, err := renderers.Result(sdk.Context{}, sdk.ToolRenderResult{Content: []map[string]any{{"type": "text", "text": "raw output"}}}, sdk.ToolRenderResultOptions{}, sdk.ToolRenderContext{}, 80)
	if err != nil || !strings.Contains(strings.Join(fallback, "\n"), "raw output") {
		t.Fatalf("fallback = %#v, %v", fallback, err)
	}
}

func TestLatePresentationClaimsAndWithdrawalsReportReloadLimits(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	a.session = s
	if got := a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return nil }); got != nil {
		t.Fatal("unregistered tool was claimed")
	}
	receipt := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	if receipt["appliesAfterReload"] != true {
		t.Fatalf("late claim hid resolver cache limitation: %+v", receipt)
	}
	text, ok := s.integrationsText("build-tools")
	if !ok || !strings.Contains(text, "appliesAfterReload") {
		t.Fatalf("inspection hid pending claim: %q", text)
	}
	other := presentationSession(t)
	a.session = other
	early := registerPresentation(t, a, other, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	if early["appliesAfterReload"] == true {
		t.Fatal("early claim was incorrectly delayed")
	}
	if a.resolveToolRenderer("build_run", func() *sdk.ToolRenderers { return nil }) == nil {
		t.Fatal("early claim was not resolved")
	}
	changedDescriptor := strings.Replace(buildPresentationDescriptor, `"label":"Build"`, `"label":"Changed"`, 1)
	params, _ := json.Marshal(registrationParams{Kind: "toolPresentation", Key: "build_run", Descriptor: json.RawMessage(changedDescriptor), PreviousRevision: early["descriptorRevision"].(string)})
	changed, failure := a.registerContribution(presentationTestContext(), other, api.Request{Method: "register", RegistrationID: early["registrationId"].(string), Generation: "generation", From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: params})
	if failure != nil || changed.(map[string]any)["appliesAfterReload"] != true {
		t.Fatalf("changed cached descriptor was not marked: %+v %v", changed, failure)
	}
	params, _ = json.Marshal(registrationParams{Kind: "toolPresentation", Key: "build_run", Descriptor: json.RawMessage(buildPresentationDescriptor), PreviousRevision: changed.(map[string]any)["descriptorRevision"].(string)})
	restored, failure := a.registerContribution(presentationTestContext(), other, api.Request{Method: "register", RegistrationID: early["registrationId"].(string), Generation: "generation", From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: params})
	if failure != nil || restored.(map[string]any)["appliesAfterReload"] == true {
		t.Fatalf("restoring cached descriptor retained a false pending claim: %+v %v", restored, failure)
	}
	request := api.Request{Method: "unregister", Generation: "generation", RegistrationID: early["registrationId"].(string), From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: json.RawMessage(`{}`)}
	withdrawn, failure := a.unregisterContribution(context.Background(), other, request)
	if failure != nil || withdrawn.(map[string]any)["appliesAfterReload"] != true {
		t.Fatalf("withdrawal hid cached renderer: %+v %v", withdrawn, failure)
	}
	owners := other.integrationOwners()
	if len(owners) != 1 || owners[0] != "build-tools" {
		t.Fatalf("pending owner disappeared from inspection/completion: %v", owners)
	}
	text, ok = other.integrationsText("build-tools")
	if !ok || !strings.Contains(text, "withdrawn") || !strings.Contains(text, "appliesAfterReload") {
		t.Fatalf("inspection erased pending withdrawal: %q", text)
	}
}

func TestToolPresentationRegistrationIsIdempotentAndConflictSafe(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	first := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	again := registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	if first["registrationId"] != again["registrationId"] || again["idempotent"] != true {
		t.Fatalf("idempotent register = %#v, then %#v", first, again)
	}
	params, _ := json.Marshal(map[string]any{"kind": "toolPresentation", "key": "other", "descriptor": json.RawMessage(buildPresentationDescriptor)})
	_, failure := a.registerContribution(presentationTestContext(), s, api.Request{Method: "register", Generation: "generation", From: api.Identity{Owner: "other-tools", Instance: "i-2"}, Params: params})
	if failure == nil || failure.Code != "permissionDenied" {
		t.Fatalf("tool ownership conflict = %v", failure)
	}
	if _, ok := s.presentations.lookup("build_run"); !ok {
		t.Fatal("conflicting registration rolled back the live owner")
	}
	all := s.contributions.List()
	if len(all) != 1 {
		t.Fatalf("conflicting registration leaked into the registry: %#v", all)
	}
	request := api.Request{Method: "unregister", Generation: "generation", RegistrationID: first["registrationId"].(string), From: api.Identity{Owner: "build-tools", Instance: "i-1"}, Params: json.RawMessage(`{}`)}
	if _, failure := a.unregisterContribution(context.Background(), s, request); failure != nil {
		t.Fatalf("unregister: %v", failure)
	}
	if _, ok := s.presentations.lookup("build_run"); ok || len(s.contributions.List()) != 0 {
		t.Fatal("unregister left a live presentation")
	}
}

func TestDescribeContributionsIsFilteredAndPaged(t *testing.T) {
	a := &app{}
	s := presentationSession(t)
	registerPresentation(t, a, s, "build-tools", "i-1", "build_run", buildPresentationDescriptor)
	params, _ := json.Marshal(describeParams{Kind: "toolPresentation"})
	result, failure := a.dispatchService(context.Background(), sdk.Context{}, s, api.Request{Method: "describe", Generation: "generation", Params: params})
	if failure != nil {
		t.Fatal(failure)
	}
	described := result.(map[string]any)
	if len(described["registrations"].([]map[string]any)) != 1 || described["nextCursor"] != "" {
		t.Fatalf("describe = %#v", described)
	}
}

// TestPresentationHeaderShortensTargetsToWidth proves a long recorded target is
// rendered through the path-display rules at every required width.
func TestPresentationHeaderShortensTargetsToWidth(t *testing.T) {
	snapshot := presentation.Snapshot{Title: "Read", Phase: presentation.PhaseSettled, Target: &presentation.Target{Kind: "path", Value: "/srv/projects/pi-workbench/internal/host/presentation_render.go"}}
	descriptor := presentation.Descriptor{Label: "Read"}
	for _, width := range []int{36, 57, 80, 140} {
		header := presentationHeader(descriptor, snapshot, false, width, "", nil)
		if got := tui.VisibleWidth(header); got > width/2+24 {
			t.Errorf("width %d: header %q is %d cells", width, header, got)
		}
		if !strings.Contains(header, "presentation_render.go") && width >= 57 {
			t.Errorf("width %d: header dropped the basename: %q", width, header)
		}
	}
}

// TestFirstTouchGuideIsInjectedOncePerRevision proves a contributed scheme's
// guide reaches the model on first touch only, and again after re-registration.
func TestFirstTouchGuideIsInjectedOncePerRevision(t *testing.T) {
	provider := &contributedProvider{
		scheme:         "kb",
		canonical:      "kb",
		registrationID: "reg-1",
		owner:          api.Identity{Owner: "kb-tools", Instance: "i-1"},
		descriptor:     resource.ProviderDescriptor{Schemes: []string{"kb"}, Summary: "Knowledge base", Guide: "Use kb://<id>/"},
	}
	router := resource.NewRouter()
	if err := router.RegisterContributed([]resource.Provider{provider}); err != nil {
		t.Fatal(err)
	}
	s := &session{router: router, guideInjected: make(map[string]string)}
	if _, ok := s.firstTouchGuide("file"); ok {
		t.Fatal("a builtin scheme must not inject a contributed guide")
	}
	guide, ok := s.firstTouchGuide("kb")
	if !ok || !strings.Contains(guide, "Use kb://<id>/") {
		t.Fatalf("first touch did not inject the guide: %q %v", guide, ok)
	}
	if _, again := s.firstTouchGuide("kb"); again {
		t.Fatal("the guide was injected twice in one session revision")
	}
	provider.descriptor.GuideRevision = "changed-guide"
	provider.descriptor.Guide = "Updated guide at the same registration"
	if guide, ok := s.firstTouchGuide("kb"); !ok || !strings.Contains(guide, "Updated guide") {
		t.Fatalf("guide revision did not refresh delivery: %q %v", guide, ok)
	}
	provider.registrationID = "reg-2"
	if _, ok := s.firstTouchGuide("kb"); !ok {
		t.Fatal("re-registration did not re-inject the guide")
	}
}
