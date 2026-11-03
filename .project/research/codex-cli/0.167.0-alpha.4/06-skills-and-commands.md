---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-06-skills-and-commands"
  title: "Codex CLI skills and commands"
  description: "Skill discovery, metadata, selection, context, plugins and command surfaces"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:06+01:00"
  updated_at: "2026-11-03T11:54:06+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["documented skills and command surfaces from source"]
---

# Skills and commands

Source root: `host:/tmp/re-recon/codex-tag/codex-rs`. [OBS] is source-observed; [INF] is inference; [UNK] is not traced. No Codex binary was run. Exact skill-catalog and selected-skill builders are preserved in [skills builders](evidence/06-skills-builders.txt).

## Discovery, parsing and selection

[OBS] `skills/` defines parser, metadata and selection primitives; `ext/skills/` provides host/executor discovery, extension contributions and rendering. Host discovery starts with config-layer roots in high-to-low order, then plugin roots, explicit extra roots and repository `.agents/skills` roots, followed by path deduplication. User-layer roots include `$CODEX_HOME/skills`, the home `.agents/skills` and the system cache; system-layer roots have admin scope; project config roots have repo scope (`ext/skills/src/host_roots.rs:55-160`). Repository ancestor directories between the detected project root and cwd are probed for `.agents/skills` (`ext/skills/src/host_roots.rs:215-247`).

[OBS] Discovery walks for `SKILL.md`, either recursively or direct children, with configurable symlink/hidden-directory policies. Recursive depth is 6, at most 2,000 directories and 20,000 entries per root; a truncated walk emits a warning (`ext/skills/src/loader/mod.rs:15-24`; `ext/skills/src/loader/discovery.rs:16-18,46-109,132-149`).

[OBS] Frontmatter must be nonempty YAML between `---` delimiters. `name` falls back to a caller-provided default, is whitespace-normalized and limited to 64 characters; `description` is required and normalized; `metadata.short-description` is optional. An initial YAML parse failure may trigger a limited line-oriented repair of unquoted scalars, then parsing is retried (`skills/src/parser.rs:4-20,30-95,98-180,183-220`). Optional `agents/openai.yaml` supplies interface, tool dependencies and policy; invalid individual dependency declarations are dropped (`skills/src/metadata.rs:18-34`). Skill metadata carries path, scope, plugin identity, optional interface, dependencies and policy; implicit invocation defaults true when not specified (`skills/src/model.rs:6-28,62-95`).

[OBS] Explicit structured skill selections resolve first against enabled skills by canonical or logical discovery path. Text is scanned for `$name` and linked `[$name](path)` mentions; plain-name matching requires one enabled skill of that name and no conflicting connector slug. Matches retain skill ordering and deduplicate paths (`skills/src/selection.rs:33-111,120-199`; `skills/src/mentions.rs:36-55,79-105`). Command inspection separately recognizes some script-run and document-read forms for implicit invocation; this is static token parsing, not evidence that every shell form is detected (`skills/src/invocation.rs:26-65,72-145`).

## Installation and model context

[OBS] Bundled system skills are embedded at build time, installed into `$CODEX_HOME/skills/.system`, and replaced when their embedded fingerprint marker differs. Matching marker skips reinstall (`skills/src/lib.rs:52-105`). The bundled skill-installer's script accepts repository/path or URL, validates a skill's `SKILL.md`, and refuses an existing destination; it defaults under `$CODEX_HOME/skills` (`skills/src/assets/samples/skill-installer/scripts/install-skill-from-github.py:46-47,169-195,219-222,290-309`). Its skill instructions describe curated and GitHub installation options (`skills/src/assets/samples/skill-installer/SKILL.md:1-18,34-54`). [UNK] Full installer network, authentication and archive-security behavior was not traced.

[OBS] Available skills enter a developer-role catalog fragment; selected skill contents use a user-role `<skill>` fragment carrying name, path, optional resource-access metadata and the content (`ext/skills/src/fragments.rs:13-69,72-127`). The catalog builder includes skill-root locators, entries and optional usage instructions. Its instructions distinguish filesystem, executor/cloud packages and custom resources, and call for reading the chosen skill completely; these are prompt instructions, not independently observed compliance (`ext/skills/src/catalog_prompt.rs:3-44,74-133`). Executor skills state emits developer updates when the catalog changes; host-skills state is positioned before permissions when present (`ext/skills/src/world_state.rs:14-26,41-114`; `core/src/context/world_state/mod.rs:537-550`). [UNK] Exhaustive budget allocation, selection timing and all skill sources are not traced here.

## Plugins, slash commands and MCP

[OBS] A plugin manifest has a name, optional version and description, and resources for skills, onboarding skill, MCP servers, apps and hooks; MCP and hooks may be inline or path-based (`plugin/src/manifest.rs:7-39`). Plugin skill roots participate in discovery (`ext/skills/src/host_roots.rs:87-95`). Plugin and app instructions enter as developer fragments when the world-state availability conditions hold (`core/src/session/world_state.rs:337-361`; `core/src/context/apps_instructions.rs:13-34`; `core/src/context/available_plugins_instructions.rs:28-50`). [UNK] Plugin installation, trust, lifecycle and command exposure are not fully traced.

[OBS] The TUI enum contains, among others, `/model`, `/permissions`, `/skills`, `/review`, `/compact`, `/plan`, `/agents`, `/mcp`, `/apps`, `/plugins`, `/resume`, `/fork`, `/diff`, `/status` and `/quit`; enum order controls popup order, some commands accept inline arguments, and task/side-conversation/visibility checks filter availability (`tui/src/slash_command.rs:12-87,165-203,205-331`). `/plan` selects Plan mode; `/compact` requests summarization; `/review` requests review according to their descriptions (`tui/src/slash_command.rs:91-105,132-151`). [UNK] Their full dispatch and any model-facing argument transformations are not traced, so not every slash command should be assumed to inject a prompt.

[OBS] The CLI has an external MCP server management subcommand (`cli/src/main.rs:144-169`). MCP server config supports `enabled_tools` and `disabled_tools`, with the latter applied after the former (`config/src/mcp_types.rs:232-297`). The TUI `/mcp` description lists MCP tools and supports login (`tui/src/slash_command.rs:145-151`). [UNK] Whether and how this particular source snapshot exposes Codex itself as an MCP server, its exact model tool declarations, and detailed plugin-to-MCP startup wiring remain unverified.
