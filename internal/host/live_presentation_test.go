//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/api"
)

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
	if output.DisplaySettings.Owner != "reference-producer" || output.DisplaySettings.Key != "web_search" || field.Value != "compact" || field.Scope != "runtime" || field.Application != "registered" {
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
	if value := entry("default"); value["value"] != "normal" || value["application"] != "registered" {
		t.Fatalf("producer default not active: %+v", value)
	}
	settingsNotice(t, fixture, "override", `set runtime `+key+` "compact"`)
	fixture.command("withdraw", "/reference withdraw-presentation")
	if value := entry("dormant"); value["value"] != "compact" || value["application"] != "unavailable" {
		t.Fatalf("withdrawal erased override or stayed active: %+v", value)
	}
	fixture.command("restore", "/reference register-presentation")
	if value := entry("restored"); value["value"] != "compact" || value["application"] != "registered" {
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
