//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/api"
)

func TestLiveResumeWithoutProducerRetainsUnavailableDisplaySchemaAndOverrides(t *testing.T) {
	root := t.TempDir()
	recorder := &requestRecorder{calls: []scriptedCall{{"web_search", map[string]any{"query": "retained result"}}}}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	first := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock")
	first.send("call", "prompt", "Run the reference tool.")
	first.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	first.command("saved", "/workbench settings tools set user reference-producer web_search density \"compact\"")
	first.close()
	id := storedSessionID(t, root)
	resumed := startRPCWithExtensions(t, root, true, nil, "--session", id)
	entry := settingsNotice(t, resumed, "dormant", "show tools.reference-producer.web_search.density")["entry"].(map[string]any)
	if entry["value"] != "compact" || entry["scope"] != "user" || entry["application"] != "unavailable" {
		t.Fatalf("dormant resume lost override or claimed a live consumer: %+v", entry)
	}
	resumed.send("group", "prompt", "/workbench settings tools reference-producer web_search")
	event := resumed.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && strings.Contains(fmt.Sprint(e["message"]), `"toolDisplay"`)
	})
	if !strings.Contains(fmt.Sprint(event["message"]), `"active": false`) || strings.Contains(fmt.Sprint(event["message"]), `"registrationId"`) {
		t.Fatal("retained descriptor restored a live registration")
	}
}

func TestLiveRetainedProducerResultIsReadableAfterProducerRemoval(t *testing.T) {
	root := t.TempDir()
	recorder := &requestRecorder{calls: []scriptedCall{{"web_search", map[string]any{"query": "retained result"}}}}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	producer := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock")
	producer.send("call", "prompt", "Run the reference tool.")
	producer.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	producer.close()
	id := storedSessionID(t, root)
	later, laterRecorder := startInRoot(t, root, []scriptedCall{{"read", map[string]any{"path": "session://by-id/" + id + "/transcript"}}})
	later.send("replay", "prompt", "Read the old transcript.")
	later.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	laterRecorder.mu.Lock()
	defer laterRecorder.mu.Unlock()
	results := toolResults(t, laterRecorder.bodies[len(laterRecorder.bodies)-1])
	if len(results) == 0 || !strings.Contains(results[0], "Fixture result") {
		t.Fatalf("retained producer result was unreadable without the producer: %v", results)
	}
}

func TestLivePresentationUpdatesRetainFailedFinalAndExposeConflict(t *testing.T) {
	results := runScriptedWith(t, t.TempDir(), "web_search,read", "", []string{"reference-producer"},
		func([]string) scriptedCall { return scriptedCall{"web_search", map[string]any{"query": "lifecycle"}} },
		func([]string) scriptedCall {
			return scriptedCall{"read", map[string]any{"path": "cfg://workbench/integrations/reference-producer"}}
		},
	)
	if len(results) != 2 || !strings.Contains(results[1], "presentation call scripted-0  settled/failed r3") || !strings.Contains(results[1], "conflicting final") {
		t.Fatalf("actual streamed lifecycle did not retain final/conflict state: %v", results)
	}
}

func TestLiveDisplaySettingsChangedCarriesOnlyBoundDisplayValues(t *testing.T) {
	t.Setenv("WORKBENCH_REFERENCE_OBSERVE_DISPLAY", "1")
	fixture := startRPCWithExtensions(t, t.TempDir(), true, []string{"reference-producer"})
	fixture.send("set", "prompt", "/workbench settings tools set runtime reference-producer web_search density \"compact\"")
	event := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), `"displayChanged"`)
	})
	var output struct {
		DisplayChanged api.DisplaySettingsChanged `json:"displayChanged"`
		RegistrationID string                     `json:"registrationId"`
	}
	if err := json.Unmarshal([]byte(event["message"].(string)), &output); err != nil {
		t.Fatal(err)
	}
	if output.RegistrationID == "" || output.DisplayChanged.Owner != "reference-producer" || output.DisplayChanged.Key != "web_search" || output.DisplayChanged.ConfigRevision == 0 || output.DisplayChanged.Values["density"] != "compact" {
		t.Fatalf("display event not bound to saved value: %+v", output)
	}
	if _, exists := output.DisplayChanged.Values["secrets.enabled"]; exists {
		t.Fatal("execution setting exposed through display event")
	}
}

func TestLiveGroupedDisplaySettingsEditResetAndInspection(t *testing.T) {
	fixture := startRPCWithExtensions(t, t.TempDir(), true, []string{"reference-producer"})
	fixture.command("set", "/workbench settings tools set runtime reference-producer web_search density \"compact\"")
	fixture.send("show", "prompt", "/workbench settings tools reference-producer web_search")
	event := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), `"toolDisplay"`)
	})
	if !strings.Contains(fmt.Sprint(event["message"]), `"compact"`) || !strings.Contains(fmt.Sprint(event["message"]), `"runtime"`) {
		t.Fatalf("grouped settings lost value/source: %+v", event)
	}
	fixture.command("reset", "/workbench settings tools reset runtime reference-producer web_search density")
	entry := settingsNotice(t, fixture, "after-reset", "show tools.reference-producer.web_search.density")["entry"].(map[string]any)
	if entry["value"] != "normal" || entry["scope"] != "defaults" {
		t.Fatalf("scoped reset erased or hid producer default: %+v", entry)
	}
}

func TestLiveDisplaySettingsGetReportsOnlyEffectiveDisplayValues(t *testing.T) {
	fixture := startRPCWithExtensions(t, t.TempDir(), true, []string{"reference-producer"})
	key := "tools.reference-producer.web_search.density"
	settingsNotice(t, fixture, "set", `set runtime `+key+` "compact"`)
	fixture.send("get", "prompt", "/reference display-settings")
	event := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), `"displaySettings"`)
	})
	var output struct {
		DisplaySettings api.DisplaySettings `json:"displaySettings"`
	}
	if err := json.Unmarshal([]byte(event["message"].(string)), &output); err != nil {
		t.Fatal(err)
	}
	field := output.DisplaySettings.Fields["density"]
	if output.DisplaySettings.Owner != "reference-producer" || output.DisplaySettings.Key != "web_search" || field.Value != "compact" || field.Scope != "runtime" || field.Application != "live" {
		t.Fatalf("effective display query: %+v", output)
	}
	if _, present := output.DisplaySettings.Fields["secrets.enabled"]; present {
		t.Fatal("execution policy appeared in display query")
	}
	fixture.send("catalog", "prompt", "/workbench settings catalog tools.reference-producer.web_search")
	catalog := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), `"catalog"`)
	})
	if !strings.Contains(fmt.Sprint(catalog["message"]), key) {
		t.Fatalf("dynamic schema missing from catalog: %+v", catalog)
	}
}

func TestLivePresentationDefaultsAndDormantOverridesFollowStableKeys(t *testing.T) {
	fixture := startRPCWithExtensions(t, t.TempDir(), true, []string{"reference-producer"})
	key := "tools.reference-producer.web_search.density"
	entry := func(id string) map[string]any {
		return settingsNotice(t, fixture, id, "show "+key)["entry"].(map[string]any)
	}
	if value := entry("default"); value["value"] != "normal" || value["application"] != "live" {
		t.Fatalf("producer default not active: %+v", value)
	}
	settingsNotice(t, fixture, "override", `set runtime `+key+` "compact"`)
	fixture.command("withdraw", "/reference withdraw-presentation")
	if value := entry("dormant"); value["value"] != "compact" || value["application"] != "unavailable" {
		t.Fatalf("withdrawal erased override or stayed active: %+v", value)
	}
	fixture.command("restore", "/reference register-presentation")
	if value := entry("restored"); value["value"] != "compact" || value["application"] != "live" {
		t.Fatalf("stable-key activation lost override: %+v", value)
	}
}

func TestLivePresentationOwnerCannotClaimAnotherExtensionsTool(t *testing.T) {
	fixture := startRPCWithExtensions(t, t.TempDir(), true, []string{"reference-producer"})
	fixture.send("claim", "prompt", "/reference claim-other")
	event := fixture.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), "ownershipProbe")
	})
	if !strings.Contains(fmt.Sprint(event["message"]), `"ownershipProbe":"denied"`) {
		t.Fatalf("foreign tool was claimed without verified ownership: %+v", event)
	}
}

func TestLiveFrameDeliveryChangedReachesTheProducerOnce(t *testing.T) {
	t.Setenv("WORKBENCH_REFERENCE_OBSERVE_DISPLAY", "1")
	root := t.TempDir()
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("p", "prompt", "say done")
	seen := map[string]api.FrameDeliveryChanged{}
	fixture.wait(func(e map[string]any) bool {
		if e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), `"frameDelivery"`) {
			var output struct {
				FrameDelivery  api.FrameDeliveryChanged `json:"frameDelivery"`
				RegistrationID string                   `json:"registrationId"`
			}
			if err := json.Unmarshal([]byte(e["message"].(string)), &output); err != nil {
				t.Fatal(err)
			}
			if output.RegistrationID == "" || output.FrameDelivery.State != "delivered" || output.FrameDelivery.Source != "reference.research" {
				t.Fatalf("unexpected delivery event: %+v", output)
			}
			if _, twice := seen[output.FrameDelivery.FrameID]; twice {
				t.Fatalf("frame %s reported delivered twice", output.FrameDelivery.FrameID)
			}
			seen[output.FrameDelivery.FrameID] = output.FrameDelivery
		}
		return e["type"] == "agent_end"
	})
	if len(seen) != 2 {
		t.Fatalf("want a delivery event for the notice and the claim, have %v", seen)
	}
}
