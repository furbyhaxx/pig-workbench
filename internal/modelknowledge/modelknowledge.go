// Package modelknowledge resolves a model's training-data cutoff from attributed data only.
package modelknowledge

import (
	"fmt"
	"strings"
	"time"
)

// Cutoff is a model's knowledge cutoff with its attribution.
type Cutoff struct {
	// Date is YYYY-MM-DD or YYYY-MM.
	Date string
	// Basis is "explicit" for a stated cutoff and "estimated" for one derived from a release date.
	Basis string
	// Source names where the value came from.
	Source string
}

// Lookup returns the bundled models.dev cutoff for a model, or false when the model is unknown or
// providers disagree about it. A provider alias such as a renamed "claude" provider falls back to
// the bare model id.
func Lookup(provider, id string) (Cutoff, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, key := range []string{provider + "/" + id, id} {
		if date, ok := snapshot[key]; ok {
			return Cutoff{Date: date, Basis: "explicit", Source: "models.dev snapshot " + snapshotDate}, true
		}
	}
	return Cutoff{}, false
}

// Estimate derives a cutoff of one year before the model's earliest listed release date, for models
// models.dev has no explicit cutoff for. Callers use it only after every explicit source came up empty.
func Estimate(provider, id string) (Cutoff, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	provider = strings.ToLower(strings.TrimSpace(provider))
	for _, key := range []string{provider + "/" + id, id} {
		released, ok := releases[key]
		if !ok {
			continue
		}
		date, ok := oneYearBefore(released)
		if !ok {
			continue
		}
		return Cutoff{Date: date, Basis: "estimated", Source: "release date " + released + " minus one year (models.dev snapshot " + snapshotDate + ")"}, true
	}
	return Cutoff{}, false
}

// oneYearBefore subtracts one calendar year from YYYY-MM-DD or YYYY-MM, keeping the precision.
func oneYearBefore(date string) (string, bool) {
	if day, err := time.Parse("2006-01-02", date); err == nil {
		if day.Month() == time.February && day.Day() == 29 {
			day = day.AddDate(0, 0, -1)
		}
		return day.AddDate(-1, 0, 0).Format("2006-01-02"), true
	}
	if month, err := time.Parse("2006-01", date); err == nil {
		return month.AddDate(-1, 0, 0).Format("2006-01"), true
	}
	return "", false
}

// ParseSetting parses `<model-id>=<YYYY-MM[-DD]>` entries (the model id may carry a `provider/`
// prefix) and returns the cutoff configured for the model, matching the provider-qualified id first.
func ParseSetting(entries []string, provider, id string) (Cutoff, bool, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	provider = strings.ToLower(strings.TrimSpace(provider))
	var qualified, bare *Cutoff
	for _, entry := range entries {
		name, date, ok := strings.Cut(entry, "=")
		name, date = strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(date)
		if !ok || name == "" || !ValidDate(date) {
			return Cutoff{}, false, fmt.Errorf("knowledge cutoff entry %q must look like <model-id>=<YYYY-MM[-DD]>", entry)
		}
		cutoff := &Cutoff{Date: date, Basis: "explicit", Source: "Workbench settings"}
		switch name {
		case provider + "/" + id:
			qualified = cutoff
		case id:
			bare = cutoff
		}
	}
	switch {
	case qualified != nil:
		return *qualified, true, nil
	case bare != nil:
		return *bare, true, nil
	}
	return Cutoff{}, false, nil
}

// ValidDate reports whether date is a real YYYY-MM-DD or YYYY-MM calendar date.
func ValidDate(date string) bool {
	if _, err := time.Parse("2006-01-02", date); err == nil {
		return true
	}
	_, err := time.Parse("2006-01", date)
	return err == nil
}

// Guidance is the fixed explanation a model gets next to its cutoff. It is deliberately independent
// of today's date so the delivered frame stays stable between days.
const Guidance = "Your training data stops at the cutoff above (earlier than you may assume if none is stated; one labelled estimated is derived from the model's release date and is approximate); compare it with today's date in the date frame. " +
	"Libraries, APIs, flags, model names, prices and events may have changed since, so remembered specifics are unverified. " +
	"Before relying on version-sensitive facts, check evidence in this session: the installed code, lockfiles, docs:// pages, command --help or a web search if available. " +
	"Prefer that evidence over memory, and say when you could not verify something."
