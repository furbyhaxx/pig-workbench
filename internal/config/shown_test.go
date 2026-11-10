package config

import (
	"slices"
	"testing"
)

// pendingConsumers names shown keys that still wait for their consumer; it only shrinks and is empty at the Slice 1 exit.
var pendingConsumers = []string{
	"activity.replace-working-indicator", "answer.retries", "answer.timeout-ms", "ask.auto-submit", "ask.confirm-dismiss-when-dirty",
	"ask.history.enabled", "ask.history.retention-days", "ask.present-single-as-multi", "context.timezone", "coordinator.answer-attribution",
	"coordinator.audit", "coordinator.classes", "coordinator.confirm-destructive", "coordinator.frames", "coordinator.lease-hours",
	"coordinator.mark-targets", "coordinator.single", "coordinator.target-consent", "docs.source-paths", "docs.source-versions-kept",
	"edit.model-repair.enabled", "edit.snapshot.max-bytes", "edit.snapshot.max-file-bytes", "edit.snapshot.max-paths", "edit.snapshot.max-versions",
	"frames.role", "guides.disabled", "guides.enabled", "guides.max-bytes", "instructions.agents-md-reminder", "jobs.log-bytes",
	"jobs.max-finished", "jobs.raw-logs", "jobs.session-log-bytes", "jobs.termination-grace-ms", "jobs.yield-after-ms",
	"locations.ssh-host-write", "locations.worktree-parent", "projects.offer-registration", "projects.storage-links", "prompt.template",
	"read.concurrency", "read.output-max-columns", "read.query.default-rows", "read.query.max-rows", "read.query.timeout-ms",
	"read.summarize.min-body-lines", "read.summarize.min-comment-lines", "read.summarize.prose", "secrets.env-injection",
	"sessions.broadcast", "skills.conditions", "statusline.mode", "statusline.path-style", "storage.finished-retention-days",
	"storage.out-retention-days", "tldr.concurrency", "tldr.timeout-ms",
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
