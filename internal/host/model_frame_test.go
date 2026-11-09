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
	facts := modelContextFacts(known, modelSettings(t, ""), nil)
	if cutoff := cutoffOf(t, facts); cutoff["date"] != "2026-06" || cutoff["basis"] != "explicit" || !strings.Contains(cutoff["source"].(string), "models.dev") {
		t.Fatalf("bundled cutoff = %v", cutoff)
	}
	if facts["training_data_notice"] != modelknowledge.Guidance {
		t.Fatal("guidance missing")
	}

	unknown := &sdk.ModelInfo{Provider: "x", ID: "no-such-model-9"}
	facts = modelContextFacts(unknown, modelSettings(t, ""), nil)
	if _, ok := facts["knowledge_cutoff"]; ok {
		t.Fatalf("a cutoff was invented for an unknown model: %v", facts)
	}
	if facts["training_data_notice"] == nil {
		t.Fatal("an unknown cutoff must still carry the guidance")
	}
}

func TestModelFrameCutoffPrecedenceIsSettingThenContributedThenSnapshot(t *testing.T) {
	model := &sdk.ModelInfo{Provider: "claude", ID: "claude-sonnet-5-5"}
	contributed := []contributedFields{{owner: "web", fields: map[string]any{"knowledge_cutoff": map[string]any{"date": "2026-03-01", "basis": "explicit", "source": "web"}}}}
	facts := modelContextFacts(model, modelSettings(t, ""), contributed)
	if cutoffOf(t, facts)["date"] != "2026-03-01" {
		t.Fatalf("contributed must beat the snapshot: %v", facts)
	}
	facts = modelContextFacts(model, modelSettings(t, "[models]\nknowledge-cutoffs = [\"claude-sonnet-5-5=2026-04-15\"]\n"), contributed)
	if cutoff := cutoffOf(t, facts); cutoff["date"] != "2026-04-15" || cutoff["source"] != "Workbench settings" {
		t.Fatalf("setting must beat everything: %v", cutoff)
	}
	facts = modelContextFacts(model, modelSettings(t, "[models]\nknowledge-cutoffs = [\"claude-sonnet-5-5=soon\"]\n"), nil)
	if msg, _ := facts["knowledge_cutoff_setting_error"].(string); !strings.Contains(msg, "claude-sonnet-5-5=soon") || cutoffOf(t, facts)["date"] != "2026-06" {
		t.Fatalf("a malformed entry must be named and must not block or replace the snapshot value: %v", facts)
	}
}
