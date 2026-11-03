---
document:
  type: "specification"
  id: "SPEC-0014"
  title: "System prompt templates, context frames and progressive skill disclosure"
  description: "Workbench renders PiG's system prompt from Jinja templates (partials, model/tool conditionals, model families), keeping PiG's prompt structure with Workbench content; session facts arrive as nugent-style context frames; skills are listed, loaded and unloaded by progressive disclosure"
  status: "draft"
  revision: 3
  created_at: "2026-11-03T12:26:45+01:00"
  updated_at: "2026-11-03T13:03:50+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User asked for templated system prompts that take over PiG's builtin, model-aware conditionals, nukii/nugent skill disclosure and context frames, and multiple instruction files per directory"
      changes: ["Host facts from PiG source and SDK", "Template engine, files, sections, inputs, model families", "Delivery modes force and sections, parity check, epochs", "Context frames", "Progressive skill disclosure", "Settings, probes PR1–PR6, host request H-P1, acceptance, open decisions"]
    - revision: 2
      reason: "User correction on OpenAI behaviour; host limits made explicit"
      changes: ["§6.3 example: OpenAI models over-apply security, not under-apply", "§3.3 raw sections only with force delivery", "§2 Anthropic OAuth identity block"]
    - revision: 3
      reason: "User: no verbatim copy of PiG's prompt; keep PiG's sections adapted to Workbench; host changes go to a personal PiG fork"
      changes: ["builtin/pig and the parity check removed; builtin/workbench is the default and keeps PiG's sections with adapted content (§7)", "<docs> as docs:// URIs, new <resources> section, <cwd> holds the stable project root", "H-P1 becomes fork patch F-P1; no upstream filing", "Acceptance, probes, settings and decisions updated"]
---

# 1. Purpose and scope

Workbench owns the system prompt. It renders it from Jinja templates and sends it in place of PiG's builtin. The builtin template, `builtin/workbench` (§7), keeps PiG's sections, order and markup and adapts their content: documentation as `docs://` URIs, skills as `skill://`, instructions from SPEC-0007, a new `<resources>` section. It does not copy PiG's text. Model-aware templates are built on top: the same instructions, worded for each model family, plus countersteering for a family's known weaknesses.

Three related mechanisms ship with it, because a cacheable prompt needs them:
- **Context frames** (nugent): date, environment, project, location, model and capabilities arrive as framed messages before the user's input, not in the system prompt.
- **Progressive skill disclosure** (nukii-revived): skills are listed cheaply, loaded on demand, revealed when nested, unloaded on request, conditional on the workspace.
- **Several instruction files per directory**: SPEC-0007 class field `select` (§9).

Not in scope: automatic session titles (PLAN-0001 later), subagent and agent-definition prompts (excluded from Workbench), personality presets. Users can write personalities as partials.

Prior art (read-only):
- nukii-revived `f03b820`: `host:/mnt/Projects/nukular/nukii-revived/.nuk/skills/nukii-system-prompts/SKILL.md` (prompt assembly rules), `crates/nukii-coding-agent/src/skills/` (conditions, load/unload, sub-skills, replay budget), `crates/nukii-agent-sdk/src/extensions/mod.rs` (keyed reminders, replay policies).
- nugent (nukular-ai `8a40f606`): `crates/nukular-nugent/docs/prompts.md` (template layout, blocks, sandboxing, reload as diff frames), `src/app/context_frames.rs` and `context_facts/date.rs` (frames), `crates/nukular-agent-sdk/src/hook/context_snapshot.rs` and `src/runtime/turn/context_frames.rs` (refresh policies, boundary commit).
- Harness prompting dossiers in progress under `.project/research/` (`antigravity-cli`, `claude-code-cli`, `codex-cli`, `deepseek-harness`; each has a system-prompt chapter). They are the input for family adapters (§6.3); nothing here depends on their conclusions.

# 2. Host facts

Source: PiG `main@9b4efdd6` (2026-11-02; the installed build is `0.5.3+1.4.0.74f46864`, whose commit is not on the public remote, so version drift is possible) and the staged SDK `sdk-899c7964784a0050`.

| Fact | Source |
|---|---|
| The builtin prompt is ordered sections: untagged `preamble`, then `<tools>`, `<rules>`, `<docs>`, `<addendum>`, `<project_context>`, `<skills>`, `<cwd>`. Empty optional sections are left out; `<cwd>` is always present | `internal/codingagent/prompts/coding.go` `BuildSystemPromptSections` |
| `customPrompt` (SYSTEM.md, `--system-prompt`) replaces the preamble and drops tools, rules and docs; `appendSystemPrompt` joins `addendum` | same, `prompts/tool_definitions.go` `FromExtensionOptions` |
| Context files: one per directory (first of `AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, `CLAUDE.md`, `CLAUDE.MD`), agent dir first, then root → cwd | `docs/config.md` |
| `before_agent_start` fires once per user prompt with `systemPromptOptions` (selected and hidden tools, snippets, guidelines, prompt guidelines, append, custom prompt, sections, cwd, context files, skills) and the host-rendered `systemPrompt`. Handlers may edit every option; later handlers see earlier edits | SDK `prompt_sections.go`, `extension.go`; PiG `before_agent_start_run.go` |
| Custom `sections` replace a base section by name in place or append; an empty value leaves the base unchanged; `preamble` is rejected | `prompts/custom_sections.go` |
| A returned `systemPrompt` (or `options.forceSystemPrompt`) is a **forced** prompt: projected onto every request of the run and not recorded in the transcript | `interactive_run_prompt.go` `projectRunPrompt` |
| Unforced prompts are recorded as a `system` message with all sections, then patch messages by section name (`null` removes). Models with `supportsMidConvoSystemMessages` receive patches as mid-conversation system messages; others get the transcript collapsed into one system prompt | `coding/session_transcript.go`, `ai/transcript.go`, `docs/session-format.md` |
| `SendMessage(steer)` from `before_agent_start` is persisted before the user prompt; a returned `message` lands right after it | probes P30 |
| `GetSystemPromptOptions` returns the base options in commands | SDK `context.go` |
| Anthropic OAuth requests get a fixed first system block (`You are Claude Code, Anthropic's official CLI for Claude.`) before the prompt; only `before_provider_request` sees it | `ai/anthropic_client.go`, `ai/anthropic.go` `anthSystemBlocks` |

# 3. Templates

## 3.1 Engine

[gonja v2](https://github.com/nikolalohinski/gonja) (`v2.9.1`, 2026-09-26): pure-Go Jinja2 with `extends`/`block`/`super()`, `include`, `import`/`macro`, `set`, `with`, `filter`, custom filters/tests/globals, pluggable loaders, `StrictUndefined`, `TrimBlocks`. minijinja (used by nukii and nugent) is Rust; its C binding would need cgo, which a Piglet binary avoids. Templates written for nugent mostly port unchanged; differences are documented in `docs://workbench/prompts`.

Configuration: strict undefined, `trim_blocks` and `lstrip_blocks` on, trailing newline kept, no autoescape. Limits: include/extends depth 16, 256 KiB per template file, 64 KiB of rendered prompt (exceeding it is a render error), render deadline 100 ms (render runs in a goroutine; on timeout the result is discarded).

## 3.2 Files and lookup

| Root | Location | Trust |
|---|---|---|
| builtin | embedded, namespace `builtin/` | always; cannot be shadowed, unknown `builtin/` names fail |
| user | `sdk.AgentDir()/system-prompts/` | user |
| project | `sdk.ConfigDirName()/system-prompts/` in the project | only while the host trusts the project (SPEC-0010 §5) |

- The entry template is the setting `prompt.template`: `builtin/workbench` (default, §7) or a file name such as `system.md` in the user or project root. A project file shadows the user file of the same name; partials fall back to the user root.
- Includes resolve only below these roots. Absolute paths, `..`, backslashes, NUL bytes and symlinks that leave the root are rejected after canonicalisation.
- PiG's own `SYSTEM.md`, `APPEND_SYSTEM.md`, `--system-prompt` and `--append-system-prompt` keep working: they reach the templates as `host.custom_prompt` and `host.append`, and `builtin/workbench` applies them where PiG does: the custom prompt replaces the preamble, the append fills `<addendum>`.

## 3.3 Sections

A template renders **sections**, the unit PiG records and patches:

```jinja
{% extends "builtin/workbench" %}
{% block rules %}
{{ super() }}
{% if model is family("openai") %}
- Do exactly the requested scope. Add security measures only for a concrete threat in this change: …
{% endif %}
{% endblock %}
```

- Text before the first section is the preamble. `{% section "name" %}…{% endsection %}` (custom tag) delimits a section; its body is wrapped as `<name>\n…\n</name>`, as PiG does. `{% section "name" raw %}` emits it untagged; only `force` delivery can send untagged sections (PiG wraps every custom section).
- Names follow PiG's rule `[a-z][a-z0-9_-]*`; duplicates are a render error. A section whose body is empty after trimming is left out.
- `builtin/workbench` wraps each section in a block of the same name (`preamble`, `tools`, `rules`, `resources`, `docs`, `addendum`, `project_context`, `skills`, `cwd`), so a child template changes one section without copying the rest.

## 3.4 Inputs

Only typed values that are fixed for the request. No clock, environment variables, git state or file reads from templates. Session facts are frames (§5).

| Variable | Content |
|---|---|
| `model` | `provider`, `id`, `name`, `api`, `family` (§3.5), `reasoning`, `context_window`, `max_output`, `thinking` (level) |
| `tools` | `selected` (ordered), `declared` (selected minus hidden), `hidden`, `snippets` (name → text), `guidelines` (name → list) |
| `host` | `custom_prompt`, `append`, `prompt_guidelines`, `docs` (whether PiG's docs bundle is present), `context_files` (path, content), `skills` (PiG's discovered list), `sections` (other extensions' custom sections, name → text), `mode` (`interactive`, `rpc`, `print`, `json`), `os`, `shell` |
| `instructions` | Stable instruction set selected by SPEC-0007 at the epoch start: list of `{class, path, display_path, role, content}` |
| `skills` | Workbench catalog (§8): `{name, description, location, source, bundle}`, plus `listing` (`prompt` or `frame`) |
| `workbench` | Prompt-relevant settings: `edit.mode`, `read` features, `secrets.enabled`, registered schemes (name, one-line summary) |
| `project_root` | Host session cwd; stable for the session (`<cwd>`). The current Workbench location is a frame (§5), not a template input |

Helpers:

| Helper | Example |
|---|---|
| test `family(name)` | `model is family("claude")` |
| test `glob(pattern)` | `model.id is glob("gpt-5*")`, `"anthropic/" ~ model.id is glob("anthropic/*opus*")` |
| test `regex(pattern)` | `model.id is regex("^deepseek-(v|r)[0-9]")`; RE2, compiled once and cached |
| global `has_tool(name)` | `{% if has_tool("edit") and not has_tool("apply_patch") %}` |
| global `variant(name)` | `{% include variant("autonomy.md") %}` → first existing of `family/<model.family>/autonomy.md`, `autonomy.md` |
| `in` | `"codex" in model.id`, `"web_search" in tools.declared` |

Collections are passed in a defined order (tools in loadout order, skills and families sorted); templates must not depend on map iteration.

## 3.5 Model families

A family names models that are prompted alike. Workbench ships a small table; users extend or override it in `Workbench.toml`:

```toml
[[prompt.families]]
name = "openai"
match = ["openai/*", "*/gpt-*", "*/o[0-9]*", "*codex*"]

[[prompt.families]]
name = "claude"
match = ["anthropic/*", "*/claude-*"]
```

- Patterns are globs matched against `<provider>/<model id>`, case-insensitive. The first matching entry wins; user entries are checked before builtin ones. No match: `generic`.
- The builtin table covers `claude`, `openai`, `gemini`, `deepseek`, `qwen`, `kimi`, `glm`, `grok` and `generic`. It is versioned with Workbench; `/workbench prompt` shows which entry matched.

# 4. Delivery

## 4.1 Modes

| Mode | Mechanism | Recorded | Status |
|---|---|---|---|
| `force` | `before_agent_start` sets `forceSystemPrompt` to the full render | No: requests get the render; the transcript keeps PiG's base sections | works today (PR1) |
| `sections` | `before_agent_start` sets the render as sections: preamble, deletions and section order need fork patch **F-P1** (§11) | Yes, patched by section name; mid-conversation system messages for supporting models | needs F-P1 |

Default `prompt.delivery = "force"`. `sections` becomes the default once the PiG fork carries F-P1, because it keeps the transcript truthful (exports, forks, compaction `systemMessage`) and re-sends only changed sections.

Without F-P1, stock PiG keeps its own preamble, keeps its section order, appends new sections after `<cwd>` and cannot delete a section, so `builtin/workbench` (adapted preamble, `<resources>` before `<docs>`) cannot be expressed. Workbench refuses `sections` on a host without F-P1 (detected by a capability flag in the fork) and says so in `/workbench prompt`.

## 4.2 Failure and other extensions

- **Render error** (syntax, strict undefined, limit, timeout): use the last good render of this epoch; without one, leave the host prompt. Notice names the template, line and cause. Never send an empty or partial prompt.
- **Other extensions**: Workbench passes their custom sections to the template as `host.sections`; `builtin/workbench` appends them where PiG would. A handler that runs after Workbench and edits sections is ignored by `force`; Workbench detects that at the next `before_agent_start` (sections it did not see) and reports the extension. Handler order is probe PR2.

## 4.3 Epochs: what re-renders when

Prompt-cache hits need an identical prefix. The served prompt is rendered at an **epoch** start and kept byte-stable inside it:

| Input change | Effect |
|---|---|
| Session start, resume, compaction, fork, branch switch | new epoch: re-render |
| Model or thinking level | re-render (the cache is invalid anyway) |
| Tool loadout, snippets, guidelines, host options | re-render, as PiG itself does |
| Template or partial file edited | served prompt unchanged; a frame (SPEC-0002 kind `notice`, source `workbench.prompt`) carries a unified diff of the changed sections; the new text is served from the next epoch (nugent's served-section model) |
| Instruction file of the stable set | the same, through SPEC-0007's diff frames |
| Skill catalog | capabilities frame (§5) when `skills.listing = "frame"`; with `prompt`, next epoch |

Reverting an edit sends a reverse diff. A diff that cannot be applied falls back to a full replacement frame.

# 5. Context frames

Session facts that change are frames, not prompt text. Workbench evaluates them at every user input and at boundaries (session start, resume, compaction, fork, branch switch), following nugent:

| Source | Body (example) | Refresh |
|---|---|---|
| `session.date` | `Today is Tuesday, 2026-11-03, 12:26:45 (Europe/Zurich).` | `on-input`: every user input |
| `session.environment` | OS, architecture, shell, terminal mode | `on-change` |
| `session.project` | Project (SPEC-0012) name and root, session ID and title, git branch and short HEAD (no dirty state) | `on-change` |
| `session.location` | Working location with authority, how plain paths resolve (SPEC-0008); a `cd` result also states it at once | `on-change` |
| `session.model` | Model, context window, thinking level | `on-change` |
| `session.capabilities` | Schemes (one line each) and, with `skills.listing = "frame"`, the skill listing (§8) | `on-change` |

- `on-change` commits when the body differs from the last committed body of that source on the active branch since the last boundary; `on-input` commits on every user input, never on continuations; a boundary commits every source again.
- Delivery: `SendMessage(steer)` from `before_agent_start` (SPEC-0002 delivery `beforePrompt`), so frames precede the user's message and are persisted. Persisting keeps history byte-stable; projecting them per request would rewrite the past.
- The ledger is rebuilt from the branch's `pig-workbench.frame` entries; a compaction entry after a source's last frame counts as a boundary.
- Frames use SPEC-0002 rendering, kind `info`, trusted Workbench source. Time zone: setting `context.timezone`, else the system zone; unknown zone gives `Date is unavailable.`, never a guessed date.
- Each source can be switched off (`context.frames.<source>`). All are on by default; `session.capabilities` carries the skill listing only with `skills.listing = "frame"`.

# 6. Model-aware prompts

## 6.1 Rules

- Condition on facts (`model`, `tools`, `workbench`), never on prompt text.
- One shared skeleton, small per-family adapters. A family adapter rewrites wording and adds countersteering; it does not add or remove instructions that all families need.
- Delete instructions for absent tools instead of saying a tool is unavailable (`has_tool`).
- Do not ask reasoning models for chain-of-thought.

## 6.2 Layout

```text
system-prompts/
  system.md                    entry; usually extends builtin/workbench
  partials/
    autonomy.md                shared wording
    family/openai/autonomy.md  family wording, found by variant("autonomy.md")
    family/openai/counter.md   countersteering, included only for that family
```

## 6.3 Family adapters from research

The harness dossiers record how each vendor prompts its own models in its native harness. An analysis artifact (`.project/analyses/<date>-model-family-prompting/`) will compare them per instruction topic (autonomy, tool use, editing, verification, safety posture, output style) and propose adapters, with evidence links. `builtin/workbench` adopts an adapter only after an A/B run on fixed tasks shows a measured gain without regressions (§11). Example (user observation, to be confirmed by the analysis): OpenAI models over-apply security, adding hardening, validation, warnings and refusals where none belongs. The adapter countersteers by stating that the requested scope is the whole job and extra security measures need a concrete threat.

# 7. `builtin/workbench`

The default entry template. It keeps PiG's sections, order and markup, so models and users who know PiG's prompt recognise it, and adapts their content to Workbench. It does not reproduce PiG's text.

| Section | PiG content | Workbench content |
|---|---|---|
| `preamble` | "You are an expert coding assistant operating inside pig…" | Same role, plus one sentence that Workbench resources are reachable by URI through the file tools; `host.custom_prompt` replaces it as in PiG |
| `<tools>` | One line per declared tool, then "you may have access to other custom tools" | Same, from `tools.declared` and snippets (Workbench's tool overrides supply their own snippets) |
| `<rules>` | Tool guidelines, prompt guidelines, "Be concise", "Show file paths clearly", bash fallback for missing grep/find/ls | Tool and prompt guidelines; Workbench rules (selectors, edit mode, secret placeholders) only for active features; PiG's generic lines kept unless a family adapter rewords them; bash fallback dropped when Workbench provides the tools |
| `<resources>` (new) | — | Registered schemes, one line each, and "`ls scheme://` and the scheme guide explain the tree"; changes with registrations, so it re-renders per epoch |
| `<docs>` | Absolute paths to PiG's docs bundle, README and examples, topic → file map | Same topic map as URIs: `docs://harness/…` (PiG), `docs://workbench/…`, `docs://<owner>/…` for registered extensions; source under `docs://harness/source/` |
| `<addendum>` | APPEND_SYSTEM.md and `--append-system-prompt` | Unchanged (`host.append`) |
| `<project_context>` | "Project-specific instructions and guidelines:" and one `<project_instructions path=…>` per context file | Same markup, filled from the SPEC-0007 stable set (`instructions`), with `display_path` |
| `<skills>` | `<available_skills>` with file locations | Same markup with `skill://<name>` locations and the load/unload rule (§8); left out when `skills.listing = "frame"` |
| `<cwd>` | Host cwd | Project root: the host session cwd, which Workbench `cd` does not change (SPEC-0010 §1), so the section is stable for the session. The current Workbench location arrives as the `session.location` frame |
| others | Custom sections of other extensions, appended | Appended where PiG would, from `host.sections` |

- Each section is a block of the same name, so user templates change one section with `{% extends "builtin/workbench" %}` and `{% block rules %}{{ super() }}…{% endblock %}`.
- **Structure check:** Workbench compares the section names of the host-rendered prompt with the names it knows. A section that a later PiG version adds is reported in `/workbench prompt` and passed to templates in `host.sections`, so it is not lost silently.
- Family adapters (§6.3) change wording inside these sections, not the structure.

# 8. Progressive skill disclosure

## 8.1 Catalog and listing

- Sources: PiG's discovered skills (`host.skills`, honouring `disable-model-invocation`) plus Workbench's own frontmatter reading of each `SKILL.md`. Trust follows PiG's skill discovery.
- Listing entry: name, description (routing text) and location `skill://<name>`. Bodies never enter the system prompt.
- `skills.listing = "prompt"` keeps PiG's `<skills>` section; `"frame"` moves the listing into `session.capabilities`, so catalog changes do not break the prompt cache.
- Budget (`skills.listing-budget`, default 4096 bytes): over budget, bundled skills lose their descriptions first, then other descriptions are cut to 80 characters, then lowest-ranked entries become names only. Every skill stays listed by name; only listed names can be loaded by the model.

## 8.2 Load and unload

Through `skill://` (SPEC-0012 §5.3), no new tool:

| Action | Operation |
|---|---|
| Load | `read skill://<name>`: body without frontmatter, its directory, and "resolve relative paths against this directory". Recorded as loaded in the session |
| Unload | `write skill://<name>/ctl` with `unload`: loaded state removed, revealed sub-skills withdrawn (and unloaded), a frame says to stop applying it |
| Loaded set | `ls skill://loaded/` |

Repeated load returns a short note. Loaded state is stored in the session (custom entry `pig-workbench.skills`), so it survives restart, forks and reload.

## 8.3 Conditions

`SKILL.md` frontmatter, same keys as nukii-revived so shared skills in `~/.agents/skills/` behave alike in both harnesses:

```yaml
when:            # discovery: unmet → not listed
  files: ["Cargo.toml", "**/*.rs"]   # exists below the repository root
  env: ["DATABASE_URL"]              # variable is set
auto_load:       # trigger: holds → body delivered without a load
  files: ["Cargo.toml"]
  touched: ["**/*.sql"]              # a successful read/write/edit/grep/find/ls touched a match
```

- All listed kinds must hold; within a kind any entry. Globs resolve against the repository root (cwd outside a repository); one bounded, gitignore-aware walk per pattern set, cached until location or patterns change.
- Auto-load delivers the body once as a frame (`skills:auto:<name>`, capped at 2000 lines / 50 KiB with a pointer to the file). Unloading an auto-loaded skill marks it declined for the session; an explicit load clears the mark.
- An invalid `when` lists the skill unconditionally and reports it in `/workbench skills`; an invalid `auto_load` is ignored and reported. Diagnostics never reach the model.

## 8.4 Nested skills

`<skill>/skills/*/SKILL.md` are sub-skills: unlisted until their parent is loaded, then listed under the parent. A sub-skill name that collides with a top-level name is loaded as `<parent>/<child>`. Conditions apply to top-level skills only.

## 8.5 After compaction

The listing comes back with the boundary commit of `session.capabilities`. Loaded skills come back within a replay budget: explicit loads newest first as pointers ("loaded earlier; read before relying on it"), then auto-loads; a skill used before compaction is never presented as a fresh instruction.

# 9. Several instruction files per directory

SPEC-0007 classes gain `select`:

- `select = "first"` (default): `filenames` are alternatives; the first existing file per directory wins (PiG's and pi-workbench's behaviour).
- `select = "all"`: every listed file that exists in a directory is selected, in list order.

```toml
[instructions.classes.steering]
filenames = ["AGENTS.md", "CONVENTIONS.md"]
select = "all"
placement = "ancestors"
```

Separate classes remain the tool when files need different placement, caps or roles. When Workbench owns the prompt, the host's `contextFiles` are cleared and the stable set (§3.4 `instructions`) replaces them; `builtin/workbench` renders it with PiG's `<project_instructions>` markup. While `prompt.enabled = false`, SPEC-0007 §9 suppression applies unchanged.

# 10. Commands, resources and settings

- `/workbench prompt`: entry template and its chain, matched family, delivery mode, epoch, last error, rendered sections with byte counts, diff against the host-rendered prompt, structure check result. `/workbench prompt render --model <ref> [--tools …]` previews another model without sending anything.
- `session://<id>/system-prompt`: the prompt served in the current epoch (read only). `docs://workbench/prompts`: template variables, helpers, gonja differences.
- Settings (SPEC-0010 §3): `prompt.enabled` (true), `prompt.template` (`builtin/workbench`), `prompt.delivery` (`force`), `prompt.families`; `context.timezone`, `context.frames.<source>`; `skills.listing` (`prompt`), `skills.listing-budget` (4096), `skills.conditions` (true).

# 11. Host probes and requests

| # | Question |
|---|---|
| PR1 | A forced prompt from `before_agent_start` reaches every request of the run, including after a mid-run tool change, and is absent from the transcript. What does the compaction request and its `systemMessage` contain? |
| PR2 | Handler order between extensions; can Workbench run last, and what do later handlers see after a forced prompt? |
| PR3 | Which providers get mid-conversation system messages for section patches; on the fork, that a reordered, preamble-replacing section set is recorded and patched correctly |
| PR4 | `steer` from `before_agent_start` precedes the prompt in TUI, RPC and print modes, on the first prompt of a session and after compaction (extends P30) |
| PR5 | Host inputs reach templates intact for fixtures: default tools, hidden tools, no `read`, `SYSTEM.md`, `APPEND_SYSTEM.md`, repeated `--append-system-prompt`, docs stripped, custom sections from another extension, skills with `disable-model-invocation`, Windows-style cwd |
| PR6 | `ModelInfo` exposes enough to match families (provider and id); thinking level is readable in `before_agent_start` |

Host changes go to a personal PiG fork (user decision 2026-11-03), not upstream:
- **F-P1:** in `before_agent_start`, accept a complete ordered section list (preamble included, base sections deletable and reorderable) and record it as the session's system sections. Exposes a capability flag that Workbench checks.

# 12. Acceptance

- `builtin/workbench` golden renders for every PR5 fixture: PiG's section order and markup, `<docs>` without filesystem paths, `<skills>` with `skill://` locations, `<cwd>` unchanged after a Workbench `cd`; a host prompt with an unknown section is reported by the structure check.
- Template chain: user/project shadowing, `builtin/` not shadowable, untrusted project ignored, traversal and symlink escape rejected, strict undefined error, limits and deadline.
- Conditionals: `family`, `glob`, `regex`, `has_tool`, `variant` select the expected text for fixture models (one per builtin family plus an unmatched one).
- Stability: two consecutive requests of one epoch carry identical prompt bytes; a template edit produces a diff frame and changes the prompt only at the next epoch; a model change re-renders.
- Frames: date on every input, `on-change` sources only on change, all sources after compaction and on a new branch; frames precede the user message; nothing is committed on continuations.
- Skills: listing budget ladder; load, repeated load, unload with sub-skills; `when`, `auto_load`, declined after unload; replay after compaction; listing changes do not alter the system prompt with `listing = "frame"`.
- Instructions: `select = "all"` injects `AGENTS.md` and `CONVENTIONS.md` of one directory in list order; `first` keeps one.
- Prompt changes are evaluated on fixed tasks (golden transcripts, A/B, cache-hit rate) before an adapter or `builtin/workbench` change ships.

# 13. Open decisions

| # | Question | Recommendation |
|---|---|---|
| Q1 | Template engine | gonja v2 (pure Go). Alternative: pongo2 (Django dialect, less Jinja-compatible) |
| Q2 | Takeover on by default | Yes, with `builtin/workbench`; `prompt.enabled = false` restores PiG's own prompt |
| Q3 | Default delivery | `force` on stock PiG; `sections` on the fork with F-P1 |
| Q4 | Skill condition keys | nukii's `when` / `auto_load` (shared skills), not kebab-case |
| Q5 | Skill listing default | `prompt` (in `<skills>`, as PiG); `frame` once A/B runs show the cache gain |
| Q6 | Template directory | `system-prompts/` in agent dir and `.pig/`; PiG's `prompts/` is taken by prompt templates |
