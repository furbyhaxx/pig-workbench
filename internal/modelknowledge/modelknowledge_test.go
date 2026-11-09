package modelknowledge

import (
	"strings"
	"testing"
)

func TestLookupFindsABareIDUnderAProviderAliasAndRefusesUnknownModels(t *testing.T) {
	got, ok := Lookup("my-alias", "DeepSeek-Flash")
	if !ok || got.Date != "2025-05" || got.Basis != "explicit" || got.Source == "" {
		t.Fatalf("alias lookup = %+v, %v", got, ok)
	}
	if got, ok := Lookup("claude", "claude-sonnet-5-5"); !ok || got.Date != "2026-06" {
		t.Fatalf("the model from the user's session = %+v, %v", got, ok)
	}
	if _, ok := Lookup("claude", "no-such-model-9"); ok {
		t.Fatal("a model absent from the snapshot must have no cutoff")
	}
}

func TestSettingPrefersTheProviderQualifiedEntryAndRejectsBadEntries(t *testing.T) {
	entries := []string{"claude-sonnet-5-5=2026-01", "claude/claude-sonnet-5-5=2026-02-15"}
	got, ok, err := ParseSetting(entries, "claude", "claude-sonnet-5-5")
	if err != nil || !ok || got.Date != "2026-02-15" || got.Source != "Workbench settings" {
		t.Fatalf("qualified = %+v %v %v", got, ok, err)
	}
	got, ok, err = ParseSetting(entries, "other", "claude-sonnet-5-5")
	if err != nil || !ok || got.Date != "2026-01" {
		t.Fatalf("bare = %+v %v %v", got, ok, err)
	}
	if _, ok, _ := ParseSetting(entries, "x", "other-model"); ok {
		t.Fatal("unrelated model matched")
	}
	for _, bad := range []string{"no-equals", "=2026-01", "m=2026-13", "m=yesterday", "m=2026-02-30"} {
		if _, _, err := ParseSetting([]string{bad}, "p", "m"); err == nil {
			t.Fatalf("entry %q accepted", bad)
		}
	}
}

func TestEstimateIsReleaseDateMinusAYearAndNeverReplacesAnExplicitCutoff(t *testing.T) {
	got, ok := Estimate("any", "GLM-4.5-AirX")
	if !ok || got.Date != "2024-07-28" || got.Basis != "estimated" || !strings.Contains(got.Source, "2025-07-28") {
		t.Fatalf("estimate = %+v, %v", got, ok)
	}
	if _, ok := Estimate("claude", "claude-sonnet-5-5"); ok {
		t.Fatal("a model with an explicit cutoff must not also carry a release-date estimate")
	}
	if _, ok := Estimate("x", "no-such-model-9"); ok {
		t.Fatal("an unknown model has no release date")
	}
}

func TestOneYearBeforeKeepsPrecisionAndSurvivesLeapDay(t *testing.T) {
	for in, want := range map[string]string{"2025-07-28": "2024-07-28", "2025-07": "2024-07", "2024-02-29": "2023-02-28"} {
		if got, ok := oneYearBefore(in); !ok || got != want {
			t.Fatalf("oneYearBefore(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := oneYearBefore("soon"); ok {
		t.Fatal("an unparsable release date produced an estimate")
	}
}
