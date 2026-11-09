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
		text("instructions.system-dir", "/etc/agents", DenyProject),
		choice("instructions.boundary", "project", DenyProject, "project", "filesystem"),
		integer("instructions.watch.debounce-ms", 100, 1, 60000, DenyProject),
		choice("instructions.watch.remote", "auto", DenyProject, "auto", "native", "off"),
		integer("instructions.updates.context", 3, 0, 1000, AllowProject),
		choice("instructions.updates.self-edits", "notice", AllowProject, "notice", "diff", "silent"),
		boolean("instructions.agents-md-reminder", true, AllowProject),
		boolean("instructions.backlog.validate-pointers", true, AllowProject),
		integer("frames.wake-budget", 10, 0, 1000, DenyProject),
		choice("frames.role", "auto", DenyProject, "auto", "user", "developer", "system"),
		choice("frames.display.style", "rail", AllowProject, "row", "rail", "card"),
		choice("frames.display.default-visibility", "shown", AllowProject, "shown", "collapsed", "hidden"),
		integer("jobs.timeout-seconds", 120, 0, 86400, DenyProject),
		integer("jobs.yield-after-ms", 0, 0, 300000, DenyProject),
		integer("jobs.termination-grace-ms", 2000, 0, 60000, DenyProject),
		integer("jobs.max-finished", 20, 0, 10000, DenyProject),
		integer("jobs.log-bytes", 10485760, 1024, 1<<40, DenyProject),
		integer("jobs.session-log-bytes", 104857600, 1024, 1<<40, DenyProject),
		boolean("jobs.wake-on-completion", true, DenyProject),
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
		stringList("fetch.disabled-backends", []string{}, DenyProject),
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
		stringList("models.knowledge-cutoffs", []string{}, DenyProject),
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
		case "read.loop-hint-after":
			d.Application = "live"
			d.Description = "Add a repetition hint after this many identical canonical/revision/view/selection reads; source remains present."
		case "read.concurrency":
			d.Description = "Validated but not applied; multi-read currently runs serially."
		case "read.summarize.enabled":
			d.Application = "live"
			d.Description = "Enable declaration summaries for bare reads of large local code files."
		case "read.summarize.min-total-lines":
			d.Application = "live"
			d.Description = "Minimum source lines for a declaration summary on a bare local code read."
		case "edit.mode":
			d.Application = "live"
			d.Description = "Select hashline, exact-text replace, or auto edit input; captured at the next turn boundary."
		case "edit.enforce-seen-lines":
			d.Application = "live"
			d.Description = "Require fully emitted source lines before hashline edits; projects may require but not disable the guard."
		case "instructions.enabled":
			d.Application = "live"
			d.Description = "Deliver Workbench instruction frames and suppress duplicate host context-file injection. Prompt takeover currently also requires this setting."
		case "instructions.system-dir":
			d.Application = "live"
			d.Description = "Directory holding the host-level AGENTS.md (steering above the user's) and HOST.md (always-on host facts); empty disables both. World-writable files are refused; group-writable ones load."
		case "instructions.backlog.validate-pointers":
			d.Application = "live"
			d.Description = "Validate BACKLOG pointer targets against the project root before instruction delivery."
		case "instructions.updates.context":
			d.Application = "live"
			d.Description = "Context lines around instruction updates; full views are sent when a reconstructible diff is not smaller."
		case "jobs.timeout-seconds":
			d.Application = "live"
			d.Description = "Default bash hard deadline in seconds; zero disables it. A tool-call timeout overrides this value."
		case "jobs.wake-on-completion":
			d.Application = "live"
			d.Description = "Start a turn when a background or promoted job finishes while the session is idle; a running session receives the notice at its next boundary."
		case "fetch.disabled-backends":
			d.Application = "live"
			d.Description = "Fetch backends (owner/key, as listed by /workbench integrations) that read https:// skips; the builtin reader serves those URLs. The backend's own tools keep working."
		case "frames.wake-budget":
			d.Application = "live"
			d.Description = "Maximum wakes per external frame producer per hour; over budget the frame is delivered without a turn. Zero disables external wake."
		case "ask.remote.timeout-ms":
			d.Application = "live"
			d.Description = "Timeout for native ask dialogs, including resumed pending asks."
		case "sessions.enabled":
			d.Application = "live"
			d.Description = "Start authenticated inter-session messaging at session start."
		case "sessions.discoverable":
			d.Application = "live"
			d.Description = "Advertise the messaging endpoint to discovery; captured at session start."
		case "sessions.inbound":
			d.Application = "live"
			d.Description = "Accept all, same-project, explicitly trusted allowlist, or no inbound peers."
		case "sessions.wake":
			d.Application = "live"
			d.Description = "Wake the receiver for accepted messages always, only for trusted peers, or never."
		case "sessions.wake-budget":
			d.Application = "live"
			d.Description = "Maximum message-triggered wakes per peer per hour."
		case "sessions.max-hops":
			d.Application = "live"
			d.Description = "Stop automatic wake when a message reaches the hop limit; busy sessions may still receive a steer."
		case "sessions.rate-limit":
			d.Application = "live"
			d.Description = "Maximum accepted messages per peer per minute."
		case "secrets.enabled":
			d.Application = "live"
			d.Description = "Enable the session secrets boundary; enabled but unavailable protection withholds model requests."
		case "secrets.redact-owners":
			d.Application = "live"
			d.Description = "Explicit trusted extension owners permitted to call secrets.redact."
		case "prompt.enabled":
			d.Application = "live"
			d.Description = "Serve builtin/workbench in place of PiG's prompt when delivery is auto or force and instructions are enabled; false or a rendering failure retains PiG's prompt."
		case "prompt.delivery":
			d.Application = "live"
			d.Description = "auto resolves to force on stock PiG and replaces PiG's prompt with builtin/workbench; sections needs the fork host and retains PiG's prompt here. Disable takeover with prompt.enabled = false."
		case "prompt.template":
			d.Description = "Validated and displayed only; rendering currently always uses builtin/workbench."
		case "skills.listing":
			d.Application = "live"
			d.Description = "prompt keeps PiG's <skills> section in the Workbench system prompt; frame leaves it out and delivers the budgeted skill:// listing in the session.capabilities frame. The system prompt is only Workbench's when prompt delivery takes it over."
		case "models.knowledge-cutoffs":
			d.Application = "live"
			d.Description = "Knowledge cutoffs for the session.model frame, each `<model-id>=<YYYY-MM[-DD]>` with an optional `provider/` prefix; overrides contributed, bundled and estimated values (an estimate is the release date minus one year). Captured when the frame is built."
		case "skills.listing-budget":
			d.Application = "live"
			d.Description = "Target byte budget for the /workbench skills listing and the frame listing; zero disables the ladder. It does not change PiG's prompt skill listing."
		case "activity.shortcut":
			d.Application = "live"
			d.Description = "Key that shows the activity line's inspector (for example alt+w or ctrl+shift+s). PiG registers shortcuts while the extension loads, so the user-layer value is read at load and a change applies after /reload; runtime and project values are not consulted."
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
			d.Description = "Validated and retained only; no runtime consumer is wired for this setting."
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
