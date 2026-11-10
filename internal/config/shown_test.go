package config

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

// pendingConsumers names shown keys that still wait for their consumer; it only shrinks and is empty at the Slice 1 exit.
var pendingConsumers = []string{
	"activity.replace-working-indicator",
	"context.timezone",
	"jobs.log-bytes",
	"jobs.max-finished",
	"jobs.raw-logs",
	"jobs.session-log-bytes",
	"jobs.termination-grace-ms",
	"jobs.yield-after-ms",
	"projects.offer-registration",
	"projects.storage-links",
	"prompt.template",
	"secrets.env-injection",
	"skills.conditions",
	"storage.finished-retention-days",
}

func TestEveryShownSettingHasAConsumer(t *testing.T) {
	var inert []string
	for _, definition := range Catalog() {
		if definition.Application != "live" {
			inert = append(inert, definition.Key)
		}
	}
	slices.Sort(inert)
	pending := slices.Clone(pendingConsumers)
	slices.Sort(pending)
	if !slices.Equal(inert, pending) {
		t.Fatalf("shown settings without a consumer = %v, want exactly the pending list %v; wire the key or remove it from the catalog", inert, pending)
	}
}

func TestAValueOfARemovedSettingSurvivesASaveAndIsIgnored(t *testing.T) {
	m, paths := setup(t, "[coordinator]\nsingle = false # mine\n\n[read]\nmax-lines = 100\n", "")
	if err := m.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	snapshot := current(t, m)
	if _, shown := snapshot.Get("coordinator.single"); shown {
		t.Fatal("a setting without a consumer is shown")
	}
	if _, err := m.Save(context.Background(), User, snapshot, []Edit{{Key: "coordinator.single", Value: true}}, nil); err == nil {
		t.Fatal("a setting without a consumer was saved")
	}
	if _, err := m.Save(context.Background(), User, snapshot, []Edit{{Key: "read.max-lines", Value: int64(200)}}, nil); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(paths.User)
	if err != nil || !strings.Contains(string(saved), "single = false") || !strings.Contains(string(saved), "# mine") || !strings.Contains(string(saved), "max-lines = 200") {
		t.Fatalf("saved file lost the retained value: %v\n%s", err, saved)
	}
}
