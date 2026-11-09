//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/config"
)

func settingsNotice(t *testing.T, f *rpcFixture, id, args string) map[string]any {
	t.Helper()
	f.send(id, "prompt", "/workbench settings "+args)
	event := f.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), `"settings"`)
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(event["message"].(string)), &result); err != nil {
		t.Fatal(err)
	}
	f.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == id })
	return result
}

func TestLiveCatalogSettingsHaveBehavioralConsumers(t *testing.T) {
	checks := []struct {
		name   string
		keys   []string
		verify func(*testing.T)
	}{
		{"read-repetition", []string{"read.loop-hint-after"}, TestLiveReadRepetitionThresholdControlsHints},
		{"read-budgets", []string{"read.max-bytes", "read.max-lines"}, TestLiveSummaryRespectsReadBudget},
		{"summaries", []string{"read.summarize.enabled", "read.summarize.min-total-lines"}, TestSummaryViewHonoursSettings},
		{"edit-mode", []string{"edit.mode"}, TestLiveEditModeRestrictsAcceptedSyntax},
		{"edit-seen", []string{"edit.enforce-seen-lines"}, TestLiveEditSeenGuardCanBeDisabledByUser},
		{"instructions", []string{"instructions.enabled", "instructions.backlog.validate-pointers"}, TestLiveInstructionSettingsControlFramesAndPointerValidation},
		{"instruction-context", []string{"instructions.updates.context"}, TestInstructionUpdateContextSettingControlsDiff},
		{"job-timeout", []string{"jobs.timeout-seconds"}, TestLiveConfiguredJobTimeoutApplies},
		{"ask-timeout", []string{"ask.remote.timeout-ms"}, TestLiveAskDialogTimeoutReportsTimedOutNeverADefaultAnswer},
		{"endpoint", []string{"sessions.enabled", "sessions.discoverable"}, TestLiveSessionEndpointSettingsAtStartup},
		{"messaging", []string{"sessions.inbound", "sessions.rate-limit", "sessions.wake", "sessions.wake-budget"}, TestSessionMessageSettingsAffectDelivery},
		{"job-wake", []string{"jobs.wake-on-completion"}, func(t *testing.T) {
			t.Run("on", TestLivePromotedJobWakesTheModelOnceWhenItEnds)
			t.Run("off", TestLiveJobCompletionStaysQuietWhenWakeIsDisabled)
		}},
		{"frame-wake", []string{"frames.wake-budget"}, TestLiveFrameWakeBudgetDeliversWithoutATurn},
		{"hops", []string{"sessions.max-hops"}, TestLiveMessageBeyondMaxHopsIsQueuedWithoutWaking},
		{"secrets", []string{"secrets.enabled"}, TestSecretsBoundaryHonoursTheEnabledSetting},
		{"secrets-grants", []string{"secrets.redact-owners"}, TestSecretsRedactIsGrantedPerOwnerAndNeverEchoesInputOnFailure},
		{"trust", []string{"trust.rules"}, TestLiveNestedRepositoryInstructionsLoadOnlyAfterATrustRule},
		{"storage", []string{"projects.session-storage"}, TestLiveSessionStorageCanBeDisabled},
		{"activity", []string{"activity.enabled", "activity.placement"}, TestLiveActivitySettingsControlWidget},
		{"prompt", []string{"prompt.enabled", "prompt.delivery"}, TestLivePromptSettingsControlTakeover},
		{"fetch-backends", []string{"fetch.disabled-backends"}, TestLiveDisabledFetchBackendFallsBackToTheBuiltinReader},
		{"skills", []string{"skills.listing-budget"}, TestLiveSkillsListingBudgetChangesTheCommandOutput},
	}
	var covered, live []string
	for _, check := range checks {
		covered = append(covered, check.keys...)
	}
	for _, definition := range config.Catalog() {
		if definition.Application == "live" {
			live = append(live, definition.Key)
		}
	}
	slices.Sort(covered)
	slices.Sort(live)
	if !slices.Equal(covered, live) {
		t.Fatalf("behavioral consumer checks do not match live catalog: checked=%v live=%v", covered, live)
	}
	for _, check := range checks {
		t.Run(check.name, check.verify)
	}
}

func TestLiveSkillsListingBudgetChangesTheCommandOutput(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "alpha")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	description := strings.Repeat("descriptionword ", 12)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: alpha\ndescription: "+description+"\n---\nbody\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil)
	listing := func(id string) string {
		t.Helper()
		fixture.send(id, "prompt", "/workbench skills")
		notice := fixture.wait(func(event map[string]any) bool {
			return event["type"] == "extension_ui_request" && event["method"] == "notify"
		})
		message := fmt.Sprint(notice["message"])
		if !strings.Contains(message, "alpha") {
			t.Fatalf("skill missing from listing: %s", message)
		}
		fixture.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == id })
		return message
	}
	settingsNotice(t, fixture, "unlimited", "set runtime skills.listing-budget 0")
	if got := listing("full"); !strings.Contains(got, strings.TrimSpace(description)) {
		t.Fatalf("unlimited listing lost description: %s", got)
	}
	settingsNotice(t, fixture, "limited", "set runtime skills.listing-budget 10")
	if got := listing("short"); strings.Contains(got, "descriptionword") {
		t.Fatalf("limited listing retained description: %s", got)
	}
}

func TestLiveSettingsScopesAndReload(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	project := filepath.Join(root, ".pig")
	for _, dir := range []string{agent, project, filepath.Join(root, "next")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), []byte("[read]\nmax-lines = 100\nmax-bytes = 4096\n"), 0600); err != nil {
		t.Fatal(err)
	}
	projectPath := filepath.Join(project, "Workbench.toml")
	if err := os.WriteFile(projectPath, []byte("[read]\nmax-lines = 40\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := startRPC(t, root, "--approve")
	check := func(f *rpcFixture, id string, lines float64, scope string) {
		t.Helper()
		output := settingsNotice(t, f, id, "show read.max-lines")
		entry := output["entry"].(map[string]any)
		if entry["value"] != lines || entry["scope"] != scope {
			t.Fatalf("wrong effective value/source: %+v", output)
		}
	}
	check(f, "initial", 40, "project")
	saved := settingsNotice(t, f, "runtime", "set runtime read.max-lines 10")["result"].(map[string]any)
	if saved["saved"] != true || saved["published"] != true {
		t.Fatalf("runtime change did not publish: %+v", saved)
	}
	check(f, "runtime-show", 10, "runtime")
	settingsNotice(t, f, "reset", "reset runtime read.max-lines")
	check(f, "inherited", 40, "project")
	settingsNotice(t, f, "user-save", "set user read.max-bytes 2048")
	f.command("cd", "/cd next")
	check(f, "after-cd", 40, "project")
	if err := os.WriteFile(projectPath, []byte("[read]\nmax-lines = 30\n"), 0600); err != nil {
		t.Fatal(err)
	}
	settingsNotice(t, f, "reload", "reload")
	check(f, "reloaded", 30, "project")
	settingsNotice(t, f, "temporary", "set runtime read.max-lines 7")
	f.close()
	untrusted := startRPC(t, root, "--no-approve")
	check(untrusted, "ignored-project", 100, "user")
	value := settingsNotice(t, untrusted, "persisted-user", "show read.max-bytes")["entry"].(map[string]any)
	if value["value"] != float64(2048) || value["scope"] != "user" {
		t.Fatalf("user save was not persisted: %+v", value)
	}
}
