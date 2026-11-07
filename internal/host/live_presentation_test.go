//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/api"
)

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
