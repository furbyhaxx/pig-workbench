// Package config owns scoped, validated, revisioned Workbench configuration.
package config

import (
	"fmt"
	"math"
	"slices"
)

// Scope identifies the source of an override.
type Scope string

const (
	// Defaults identifies built-in values.
	Defaults Scope = "defaults"
	// User identifies the agent-owned Workbench.toml.
	User Scope = "user"
	// Project identifies the trusted host-cwd Workbench.toml.
	Project Scope = "project"
	// Runtime identifies session-only overrides.
	Runtime Scope = "runtime"
)

// Kind is a setting's validated value type.
type Kind string

const (
	// Boolean accepts true or false.
	Boolean Kind = "boolean"
	// Integer accepts exact integral values within the declared bounds.
	Integer Kind = "integer"
	// String accepts a string, optionally restricted to choices.
	String Kind = "string"
	// Strings accepts a whole replacement array of strings.
	Strings Kind = "string-array"
)

// ProjectPolicy constrains an otherwise trusted project's overrides.
type ProjectPolicy string

const (
	// DenyProject reserves a setting to the user and runtime.
	DenyProject ProjectPolicy = "user-runtime-only"
	// AllowProject permits ordinary project overrides.
	AllowProject ProjectPolicy = "allowed"
	// ReduceProject permits only reducing a lower-layer numeric ceiling.
	ReduceProject ProjectPolicy = "reduce-only"
	// RequireTrueProject permits requiring, but never disabling, a safety guard.
	RequireTrueProject ProjectPolicy = "require-true"
)

// Definition is the single source of default, validation and scope metadata.
type Definition struct {
	Key         string        `json:"key"`
	Kind        Kind          `json:"type"`
	Default     any           `json:"default"`
	Min         int64         `json:"min,omitempty"`
	Max         int64         `json:"max,omitempty"`
	Choices     []string      `json:"choices,omitempty"`
	Project     ProjectPolicy `json:"project"`
	Application string        `json:"application"`
	Description string        `json:"description"`
}

// Catalog returns independent metadata for the currently defined scalar settings.
func Catalog() []Definition {
	definitions := []Definition{
		choice("edit.mode", "hashline", AllowProject, "hashline", "replace", "auto"),
		boolean("edit.enforce-seen-lines", true, RequireTrueProject),
		boolean("edit.model-repair.enabled", false, DenyProject),
		integer("edit.snapshot.max-paths", 256, 1, 65536, DenyProject),
		integer("edit.snapshot.max-versions", 4, 1, 1024, DenyProject),
		integer("edit.snapshot.max-bytes", 67108864, 1024, 1<<40, DenyProject),
		integer("edit.snapshot.max-file-bytes", 4194304, 1024, 1<<30, DenyProject),
		integer("read.max-bytes", 51200, 1024, 51200, ReduceProject),
		integer("read.max-lines", 2000, 5, 2000, ReduceProject),
		integer("read.concurrency", 4, 1, 32, ReduceProject),
		boolean("read.summarize.enabled", true, AllowProject),
		integer("read.summarize.min-total-lines", 200, 1, 1<<30, AllowProject),
		integer("read.summarize.min-body-lines", 12, 1, 1<<30, AllowProject),
		integer("read.summarize.min-comment-lines", 8, 1, 1<<30, AllowProject),
		boolean("read.summarize.prose", false, AllowProject),
		integer("read.output-max-columns", 512, 1, 1<<20, ReduceProject),
		integer("read.query.default-rows", 100, 1, 1<<20, ReduceProject),
		integer("read.query.max-rows", 1000, 1, 1<<20, ReduceProject),
		integer("read.query.timeout-ms", 30000, 1, 300000, ReduceProject),
		integer("read.loop-hint-after", 3, 1, 10000, AllowProject),
		boolean("instructions.enabled", true, DenyProject),
		choice("instructions.boundary", "project", DenyProject, "project", "filesystem"),
		integer("instructions.watch.debounce-ms", 100, 1, 60000, DenyProject),
		choice("instructions.watch.remote", "auto", DenyProject, "auto", "native", "off"),
		integer("instructions.updates.context", 3, 0, 1000, AllowProject),
		choice("instructions.updates.self-edits", "notice", AllowProject, "notice", "diff", "silent"),
		boolean("instructions.agents-md-reminder", true, AllowProject),
		boolean("instructions.backlog.validate-pointers", true, AllowProject),
		choice("frames.role", "auto", DenyProject, "auto", "user", "developer", "system"),
		choice("frames.display.style", "rail", AllowProject, "row", "rail", "card"),
		choice("frames.display.default-visibility", "shown", AllowProject, "shown", "collapsed", "hidden"),
		integer("jobs.timeout-seconds", 120, 0, 86400, DenyProject),
		integer("jobs.yield-after-ms", 0, 0, 300000, DenyProject),
		integer("jobs.termination-grace-ms", 2000, 0, 60000, DenyProject),
		integer("jobs.max-finished", 20, 0, 10000, DenyProject),
		integer("jobs.log-bytes", 10485760, 1024, 1<<40, DenyProject),
		integer("jobs.session-log-bytes", 104857600, 1024, 1<<40, DenyProject),
		boolean("jobs.wake-on-completion", false, DenyProject),
		boolean("jobs.raw-logs", false, DenyProject),
		text("locations.worktree-parent", ".worktrees", AllowProject),
		text("locations.ssh-host-write", "include", DenyProject),
		boolean("ask.auto-submit", false, AllowProject),
		boolean("ask.confirm-dismiss-when-dirty", true, AllowProject),
		boolean("ask.present-single-as-multi", false, AllowProject),
		integer("ask.remote.timeout-ms", 300000, 1, 3600000, DenyProject),
		boolean("ask.history.enabled", true, DenyProject),
		integer("ask.history.retention-days", 90, 0, 36500, DenyProject),
		integer("answer.timeout-ms", 30000, 1, 3600000, DenyProject),
		integer("answer.retries", 1, 0, 10, DenyProject),
		integer("tldr.timeout-ms", 120000, 1, 3600000, DenyProject),
		integer("tldr.concurrency", 2, 1, 32, DenyProject),
		integer("storage.finished-retention-days", 7, 0, 36500, DenyProject),
		integer("storage.out-retention-days", 7, 0, 36500, DenyProject),
		choice("statusline.mode", "segments", DenyProject, "segments", "footer", "off"),
		choice("statusline.path-style", "fish", DenyProject, "fish", "full", "relative", "basename"),
		boolean("bash.route-to-tools", true, AllowProject),
		boolean("sessions.enabled", true, DenyProject),
		boolean("sessions.discoverable", true, DenyProject),
		choice("sessions.inbound", "all", DenyProject, "all", "sameProject", "allowlist", "off"),
		choice("sessions.wake", "always", DenyProject, "always", "trustedPeers", "never"),
		integer("sessions.wake-budget", 10, 0, 10000, DenyProject),
		integer("sessions.max-hops", 8, 1, 128, DenyProject),
		integer("sessions.rate-limit", 30, 1, 10000, DenyProject),
		choice("sessions.broadcast", "project", DenyProject, "project", "all", "off"),
		stringList("coordinator.classes", []string{"observe", "message"}, DenyProject),
		integer("coordinator.lease-hours", 8, 0, 8760, DenyProject),
		choice("coordinator.target-consent", "first-action", DenyProject, "first-action", "never", "always"),
		boolean("coordinator.single", true, DenyProject),
		boolean("coordinator.confirm-destructive", true, DenyProject),
		boolean("coordinator.mark-targets", true, DenyProject),
		boolean("coordinator.frames", true, DenyProject),
		boolean("coordinator.audit", true, DenyProject),
		boolean("coordinator.answer-attribution", true, DenyProject),
		boolean("projects.offer-registration", true, DenyProject),
		boolean("projects.storage-links", true, DenyProject),
		boolean("guides.enabled", true, AllowProject),
		integer("guides.max-bytes", 2048, 0, 65536, ReduceProject),
		stringList("guides.disabled", []string{}, AllowProject),
		integer("docs.source-versions-kept", 3, 1, 100, DenyProject),
		stringList("docs.source-paths", []string{"docs", "extensions/sdk", "cmd", "internal"}, DenyProject),
		boolean("secrets.enabled", true, DenyProject),
		boolean("secrets.env-injection", true, DenyProject),
		stringList("secrets.redact-owners", []string{}, DenyProject),
		stringList("trust.rules", []string{}, DenyProject),
		choice("projects.session-storage", "link", DenyProject, "link", "off"),
		boolean("activity.enabled", true, DenyProject),
		choice("activity.placement", "above-editor", DenyProject, "above-editor", "below-editor"),
		text("activity.shortcut", "alt+w", DenyProject),
		boolean("activity.replace-working-indicator", false, DenyProject),
		boolean("prompt.enabled", true, DenyProject),
		text("prompt.template", "builtin/workbench", AllowProject),
		choice("prompt.delivery", "auto", DenyProject, "auto", "force", "sections"),
		text("context.timezone", "system", DenyProject),
		choice("skills.listing", "prompt", AllowProject, "prompt", "frame"),
		integer("skills.listing-budget", 4096, 0, 65536, ReduceProject),
		boolean("skills.conditions", true, AllowProject),
	}
	for i := range definitions {
		d := &definitions[i]
		d.Application = "unavailable"
		switch d.Key {
		case "read.max-bytes":
			d.Application = "live"
			d.Description = "Maximum complete read-result bytes, including framing; 1024–51200."
		case "read.max-lines":
			d.Application = "live"
			d.Description = "Maximum complete read-result lines, including framing; 5–2000."
		case "activity.enabled", "activity.placement":
			d.Application = "live"
			d.Description = "The activity line: counts of running and failed jobs, a pending ask, live sessions, loaded skills and contributed items, as one row above or below the editor. Applied at session start."
		case "projects.session-storage":
			d.Application = "live"
			d.Description = "`link` shares one session directory per recognised project through symlinked buckets at session start (SPEC-0012 §3.5); `off` never touches session storage. Undo with /workbench projects unlink-storage."
		case "trust.rules":
			d.Application = "live"
			d.Description = "Workbench source trust rules, each `allow <absolute dir>` or `deny <absolute dir>`; the nearest rule decides whether instructions from a nested repository load. Manage with /workbench trust."
		default:
			d.Description = "Validated setting for its owning subsystem; inspect application status before relying on it."
		}
	}
	return definitions
}

func boolean(key string, value bool, policy ProjectPolicy) Definition {
	return Definition{Key: key, Kind: Boolean, Default: value, Project: policy}
}
func integer(key string, value, min, max int64, policy ProjectPolicy) Definition {
	return Definition{Key: key, Kind: Integer, Default: value, Min: min, Max: max, Project: policy}
}
func text(key, value string, policy ProjectPolicy) Definition {
	return Definition{Key: key, Kind: String, Default: value, Project: policy}
}
func choice(key, value string, policy ProjectPolicy, choices ...string) Definition {
	d := text(key, value, policy)
	d.Choices = choices
	return d
}
func stringList(key string, value []string, policy ProjectPolicy) Definition {
	return Definition{Key: key, Kind: Strings, Default: value, Project: policy}
}

func (d Definition) validate(value any) (any, error) {
	invalid := func() (any, error) {
		return nil, fmt.Errorf("expected %s within the documented bounds/choices; got %T", d.Kind, value)
	}
	switch d.Kind {
	case Boolean:
		if v, ok := value.(bool); ok {
			return v, nil
		}
	case String:
		if v, ok := value.(string); ok && (len(d.Choices) == 0 || slices.Contains(d.Choices, v)) {
			return v, nil
		}
	case Integer:
		var number int64
		switch v := value.(type) {
		case int:
			number = int64(v)
		case int64:
			number = v
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v > 1<<53-1 || v < -(1<<53-1) {
				return invalid()
			}
			number = int64(v)
		default:
			return invalid()
		}
		if number >= d.Min && number <= d.Max {
			return number, nil
		}
	case Strings:
		switch v := value.(type) {
		case []string:
			return slices.Clone(v), nil
		case []any:
			result := make([]string, len(v))
			for i, item := range v {
				s, ok := item.(string)
				if !ok {
					return invalid()
				}
				result[i] = s
			}
			return result, nil
		}
	}
	return invalid()
}
