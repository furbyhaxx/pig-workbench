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
		integer("read.loop-hint-after", 3, 1, 10000, AllowProject),
		boolean("instructions.enabled", true, DenyProject),
		text("instructions.system-dir", "/etc/agents", DenyProject),
		choice("instructions.boundary", "project", DenyProject, "project", "filesystem"),
		integer("instructions.updates.context", 3, 0, 1000, AllowProject),
		choice("instructions.updates.self-edits", "notice", AllowProject, "notice", "diff", "silent"),
		boolean("instructions.backlog.validate-pointers", true, AllowProject),
		integer("frames.wake-budget", 10, 0, 1000, DenyProject),
		choice("frames.display.style", "rail", AllowProject, "row", "rail", "card"),
		choice("frames.display.default-visibility", "collapsed", AllowProject, "shown", "collapsed", "hidden"),
		stringList("frames.display.sources", []string{}, AllowProject),
		integer("jobs.timeout-seconds", 120, 0, 86400, DenyProject),
		integer("jobs.yield-after-ms", 0, 0, 300000, DenyProject),
		integer("jobs.termination-grace-ms", 2000, 0, 60000, DenyProject),
		integer("jobs.max-finished", 20, 0, 10000, DenyProject),
		integer("jobs.log-bytes", 10485760, 1024, 1<<40, DenyProject),
		integer("jobs.session-log-bytes", 104857600, 1024, 1<<40, DenyProject),
		boolean("jobs.wake-on-completion", true, DenyProject),
		boolean("jobs.raw-logs", false, DenyProject),
		integer("ask.remote.timeout-ms", 300000, 1, 3600000, DenyProject),
		integer("storage.finished-retention-days", 7, 0, 36500, DenyProject),
		boolean("bash.route-to-tools", true, AllowProject),
		boolean("sessions.enabled", true, DenyProject),
		boolean("sessions.discoverable", true, DenyProject),
		choice("sessions.inbound", "all", DenyProject, "all", "sameProject", "allowlist", "off"),
		choice("sessions.wake", "always", DenyProject, "always", "trustedPeers", "never"),
		integer("sessions.wake-budget", 10, 0, 10000, DenyProject),
		integer("sessions.max-hops", 8, 1, 128, DenyProject),
		integer("sessions.rate-limit", 30, 1, 10000, DenyProject),
		boolean("projects.offer-registration", true, DenyProject),
		boolean("guides.enabled", true, AllowProject),
		integer("guides.max-bytes", 2048, 0, 65536, ReduceProject),
		stringList("guides.disabled", []string{}, AllowProject),
		stringList("fetch.disabled-backends", []string{}, DenyProject),
		boolean("secrets.enabled", true, DenyProject),
		boolean("secrets.env-injection", true, DenyProject),
		stringList("secrets.redact-owners", []string{}, DenyProject),
		stringList("trust.rules", []string{}, DenyProject),
		choice("projects.session-storage", "link", DenyProject, "link", "off"),
		boolean("activity.enabled", true, DenyProject),
		choice("activity.placement", "above-editor", DenyProject, "above-editor", "below-editor"),
		text("activity.shortcut", "alt+w", DenyProject),
		boolean("prompt.enabled", true, DenyProject),
		text("prompt.template", "builtin/workbench", AllowProject),
		choice("prompt.delivery", "auto", DenyProject, "auto", "force", "sections"),
		text("context.timezone", "system", DenyProject),
		stringList("models.knowledge-cutoffs", []string{}, DenyProject),
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
		case "read.output-max-columns":
			d.Application = "live"
			d.Description = "Characters shown per line in a read; a longer line is cut with its omitted byte count, cannot be edited until read whole, and a :raw selector shows it uncut."
		case "read.concurrency":
			d.Application = "live"
			d.Description = "How many targets of one multi-read are fetched at the same time; results keep input order and share one budget. 1 reads serially."
		case "read.summarize.enabled":
			d.Application = "live"
			d.Description = "Enable declaration summaries for bare reads of large local code files."
		case "read.summarize.min-total-lines":
			d.Application = "live"
			d.Description = "Minimum source lines for a declaration summary on a bare local code read."
		case "read.summarize.min-body-lines":
			d.Application = "live"
			d.Description = "A declaration body of at least this many lines is folded in a summary; shorter bodies are shown."
		case "read.summarize.min-comment-lines":
			d.Application = "live"
			d.Description = "A comment block of at least this many lines is folded in a summary to its first and last line."
		case "read.summarize.prose":
			d.Application = "live"
			d.Description = "Summarize large Markdown files to their headings; off reads them as exact text."
		case "edit.mode":
			d.Application = "live"
			d.Description = "Select hashline, exact-text replace, or auto edit input; captured at the next turn boundary."
		case "bash.route-to-tools":
			d.Application = "live"
			d.Description = "Run standalone ls, cat, rm, mv, cp, mkdir and `text > path` on scheme paths in-process, and pass readable scheme paths to other commands as read-only copies; applies to bash and ! commands."
		case "edit.enforce-seen-lines":
			d.Application = "live"
			d.Description = "Require fully emitted source lines before hashline edits; projects may require but not disable the guard."
		case "edit.snapshot.max-paths":
			d.Application = "live"
			d.Description = "How many files keep a source snapshot for tagged edits; the least recently used file is dropped first and must be read again before a tagged edit."
		case "edit.snapshot.max-versions":
			d.Application = "live"
			d.Description = "How many versions of one file stay available to tags from earlier reads."
		case "edit.snapshot.max-bytes":
			d.Application = "live"
			d.Description = "Memory bound for all source snapshots; each retained source counts twice its size."
		case "edit.snapshot.max-file-bytes":
			d.Application = "live"
			d.Description = "Largest file that gets a snapshot; a larger file is read without a tag and edited with exact text."
		case "guides.enabled":
			d.Application = "live"
			d.Description = "Add a contributed scheme's guide to the first read of that scheme in a session."
		case "guides.max-bytes":
			d.Application = "live"
			d.Description = "A guide longer than this is replaced by a pointer to docs://workbench/schemes/<scheme>; 0 always sends the pointer."
		case "guides.disabled":
			d.Application = "live"
			d.Description = "Schemes whose guide is never added to a read."
		case "storage.finished-retention-days":
			d.Application = "live"
			d.Description = "Days a finished session's local:// scratch directory is kept before a later session start removes it; a directory in use is never removed, and 0 removes unused ones at once."
		case "projects.offer-registration":
			d.Application = "live"
			d.Description = "Tell the user once per repository that no project recognises it and how to create or join one."
		case "context.timezone":
			d.Application = "live"
			d.Description = "Time zone of the session.date frame: `system` or an IANA name such as Europe/Zurich. An unknown zone delivers `Date is unavailable.` instead of a guessed date."
		case "secrets.env-injection":
			d.Application = "live"
			d.Description = "Set vault secrets that have an environment name as real values in the environment of bash calls and jobs; false starts them without those variables."
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
		case "instructions.boundary":
			d.Application = "live"
			d.Description = "How far up instruction discovery ascends: project stops at the project root; filesystem also applies instruction files in the directories above it, up to the filesystem root, for trusted projects."
		case "instructions.updates.self-edits":
			d.Application = "live"
			d.Description = "How a change the agent made itself to a delivered instruction file reaches it: a one-line notice, the same diff as an outside change, or nothing. Bytes the agent did not write always arrive as a diff."
		case "jobs.timeout-seconds":
			d.Application = "live"
			d.Description = "Default bash hard deadline in seconds; zero disables it. A tool-call timeout overrides this value."
		case "jobs.wake-on-completion":
			d.Application = "live"
			d.Description = "Start a turn when a background or promoted job finishes while the session is idle; a running session receives the notice at its next boundary."
		case "fetch.disabled-backends":
			d.Application = "live"
			d.Description = "Fetch backends (owner/key, as listed by /workbench integrations) that read https:// skips; the builtin reader serves those URLs. The backend's own tools keep working."
		case "frames.display.style":
			d.Application = "live"
			d.Description = "How an opened frame body is framed in the transcript: rail (line at the left), row (indent only) or card (rail with a closing rule). Transcript only; delivery is unchanged."
		case "frames.display.default-visibility":
			d.Application = "live"
			d.Description = "collapsed shows one row per context frame, shown opens its body, hidden omits the row. Transcript only; the model receives every frame regardless."
		case "frames.display.sources":
			d.Application = "live"
			d.Description = "Transcript visibility per frame source, each `shown|collapsed|hidden <source prefix>`; the longest matching prefix wins over the default. Manage with /workbench frames show|collapse|hide <source>."
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
			d.Description = "Serve the prompt.template render in place of PiG's prompt when delivery is auto or force and instructions are enabled; false or a rendering failure retains PiG's prompt."
		case "prompt.delivery":
			d.Application = "live"
			d.Description = "auto resolves to force on stock PiG and replaces PiG's prompt with builtin/workbench; sections needs the fork host and retains PiG's prompt here. Disable takeover with prompt.enabled = false."
		case "prompt.template":
			d.Application = "live"
			d.Description = "Entry template of the system prompt: builtin/workbench, or a file such as system.md in the agent directory's system-prompts/ or, in a trusted project, the project's. A template that fails to render keeps the last good render, else PiG's prompt, and is reported."
		case "models.knowledge-cutoffs":
			d.Application = "live"
			d.Description = "Knowledge cutoffs for the session.model frame, each `<model-id>=<YYYY-MM[-DD]>` with an optional `provider/` prefix; overrides contributed, bundled and estimated values (an estimate is the release date minus one year). Captured when the frame is built."
		case "skills.listing-budget":
			d.Application = "live"
			d.Description = "Target byte budget for the session.skills frame and the /workbench skills listing; over it descriptions are shortened, then dropped, and every skill stays listed by name. Zero disables the ladder."
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
