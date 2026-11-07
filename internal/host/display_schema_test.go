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
	if value, _ := snapshot.Get(key); value.Value != "compact" || value.Application != "registered" {
		t.Fatalf("stable-key re-registration lost preference: %+v", value)
	}
}
