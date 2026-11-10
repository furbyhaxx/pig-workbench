package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/modelknowledge"
)

func modelSettings(t *testing.T, toml string) *config.Snapshot {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Workbench.toml")
	if err := os.WriteFile(path, []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := config.New(config.Paths{User: path, Project: filepath.Join(t.TempDir(), "p.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func cutoffOf(t *testing.T, facts map[string]any) map[string]any {
	t.Helper()
	cutoff, _ := facts["knowledge_cutoff"].(map[string]any)
	return cutoff
}

func TestModelFrameCarriesTheCutoffAndAlwaysTheStalenessGuidance(t *testing.T) {
	known := &sdk.ModelInfo{Provider: "claude", ID: "claude-sonnet-5-5", Name: "Sonnet"}
	facts := modelContextFacts(known, "", modelSettings(t, ""), nil)
	if cutoff := cutoffOf(t, facts); cutoff["date"] != "2026-06" || cutoff["basis"] != "explicit" || !strings.Contains(cutoff["source"].(string), "models.dev") {
		t.Fatalf("bundled cutoff = %v", cutoff)
	}
	if facts["training_data_notice"] != modelknowledge.Guidance {
		t.Fatal("guidance missing")
	}

	unknown := &sdk.ModelInfo{Provider: "x", ID: "no-such-model-9"}
	facts = modelContextFacts(unknown, "", modelSettings(t, ""), nil)
	if _, ok := facts["knowledge_cutoff"]; ok {
		t.Fatalf("a cutoff was invented for a model with neither a cutoff nor a release date: %v", facts)
	}
	if facts["training_data_notice"] == nil {
		t.Fatal("an unknown cutoff must still carry the guidance")
	}
}

func TestModelFrameCutoffPrecedenceIsSettingThenContributedThenSnapshot(t *testing.T) {
	model := &sdk.ModelInfo{Provider: "claude", ID: "claude-sonnet-5-5"}
	contributed := []contributedFields{{owner: "web", fields: map[string]any{"knowledge_cutoff": map[string]any{"date": "2026-03-01", "basis": "explicit", "source": "web"}}}}
	facts := modelContextFacts(model, "", modelSettings(t, ""), contributed)
	if cutoffOf(t, facts)["date"] != "2026-03-01" {
		t.Fatalf("contributed must beat the snapshot: %v", facts)
	}
	facts = modelContextFacts(model, "", modelSettings(t, "[models]\nknowledge-cutoffs = [\"claude-sonnet-5-5=2026-04-15\"]\n"), contributed)
	if cutoff := cutoffOf(t, facts); cutoff["date"] != "2026-04-15" || cutoff["source"] != "Workbench settings" {
		t.Fatalf("setting must beat everything: %v", cutoff)
	}
	facts = modelContextFacts(model, "", modelSettings(t, "[models]\nknowledge-cutoffs = [\"claude-sonnet-5-5=soon\"]\n"), nil)
	if msg, _ := facts["knowledge_cutoff_setting_error"].(string); !strings.Contains(msg, "claude-sonnet-5-5=soon") || cutoffOf(t, facts)["date"] != "2026-06" {
		t.Fatalf("a malformed entry must be named and must not block or replace the snapshot value: %v", facts)
	}
}

func TestModelFrameFallsBackToAnEstimateFromTheReleaseDateOnlyWhenNothingElseApplies(t *testing.T) {
	model := &sdk.ModelInfo{Provider: "z", ID: "glm-4.5-airx"}
	facts := modelContextFacts(model, "", modelSettings(t, ""), nil)
	if cutoff := cutoffOf(t, facts); cutoff["date"] != "2024-07-28" || cutoff["basis"] != "estimated" {
		t.Fatalf("estimate = %v", cutoff)
	}
	contributed := []contributedFields{{owner: "web", fields: map[string]any{"knowledge_cutoff": map[string]any{"date": "2025-01-01", "basis": "explicit", "source": "web"}}}}
	if cutoff := cutoffOf(t, modelContextFacts(model, "", modelSettings(t, ""), contributed)); cutoff["date"] != "2025-01-01" {
		t.Fatalf("a contributed value must beat the estimate: %v", cutoff)
	}
	if cutoff := cutoffOf(t, modelContextFacts(model, "", modelSettings(t, "[models]\nknowledge-cutoffs = [\"glm-4.5-airx=2025-03\"]\n"), nil)); cutoff["date"] != "2025-03" || cutoff["basis"] != "explicit" {
		t.Fatalf("a setting must beat the estimate: %v", cutoff)
	}
}

func TestModelFrameStatesTheThinkingLevelOnlyForAReasoningModel(t *testing.T) {
	reasoning := &sdk.ModelInfo{Provider: "claude", ID: "claude-sonnet-5-5", Reasoning: true}
	facts := modelContextFacts(reasoning, "high", modelSettings(t, ""), nil)
	if _, stale := facts["reasoning"]; stale || facts["thinking"] != "high" {
		t.Fatalf("facts = %v", facts)
	}
	plain := &sdk.ModelInfo{Provider: "claude", ID: "claude-sonnet-5-5"}
	if _, present := modelContextFacts(plain, "high", modelSettings(t, ""), nil)["thinking"]; present {
		t.Fatal("a model without reasoning has no thinking level")
	}
}

func TestAChangedFactIsSentAsItsChangedFieldsOnly(t *testing.T) {
	before := `{"id":"m","thinking":"medium","contextWindow":425000,"tools":["read","bash"],"training_data_notice":"long text"}`
	after := `{"id":"m","thinking":"high","contextWindow":425000,"tools":["read","grep"],"training_data_notice":"long text"}`
	want := "Changed in session.model; every other field is unchanged.\nthinking: medium -> high\ntools: +grep, -bash"
	if got := factChange("session.model", before, after); got != want {
		t.Fatalf("change = %q", got)
	}
	if got := factChange("session.model", before, before); got != "" {
		t.Fatalf("unchanged facts produced %q", got)
	}
}
