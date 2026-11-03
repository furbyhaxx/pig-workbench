---
document:
  type: "specification"
  id: "SPEC-0010"
  title: "Configuration, persistence, privacy and trust"
  description: "Scoped JSON configuration, safe saves, durable records, lifecycle ownership and permission boundaries"
  status: "draft"
  revision: 8
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-03T10:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Complete the remaining Workbench specification and design areas"
      changes: ["Defined behavior, state, safety, examples, dependencies and acceptance criteria"]
    - revision: 2
      reason: "M0.2 probes measured session and skill catalog access"
      changes: ["Recorded that any extension can list all sessions with full text and set the Workbench boundary"]
    - revision: 3
      reason: "User removed session access restrictions"
      changes: ["Replaced the session privacy boundary with plain catalog access", "Dropped cross-project session authorization from the trust matrix"]
    - revision: 4
      reason: "M0.1 donor parity audit"
      changes: ["Added donor parity rules for trust scope and concurrent saves"]
    - revision: 5
      reason: "pig-web harmonisation and user decisions"
      changes: ["Catalog entries for statusline mode, answer history, edit mode rules, bash routing, fetch backends and sessions", "Column cap default 512"]
    - revision: 6
      reason: "SPEC-0012 project model and URI namespace; pig-web R6"
      changes: ["Settings for projects, artifacts, guides, docs, broadcast and coordinator", "Project registry and docs cache storage", "Removed history.globalAccess (session access has no consent gate, history:// retired)"]
    - revision: 7
      reason: "DRAFT-0003 approved (activity line, Q1–Q5 as recommended; fullscreen is the target mode)"
      changes: ["Activity line settings"]
    - revision: 8
      reason: "Secrets redesigned after nukii-revived (SPEC-0013); donor secret guard removed"
      changes: ["Known-secret guard replaced by SPEC-0013", "Secrets settings and vault storage"]
---

# 1. Scope and precedence

Draft cross-cutting contract. Owns config paths/layering, storage lifecycle, trust and privacy shared by SPEC-0001 through SPEC-0009. Domain specs own their semantic defaults; this document owns how values are sourced/validated/applied.

PiG extension convention is JSON, intentionally not a parallel TOML config system. Host-note TOML frontmatter remains a separate existing format and requires round-trip preservation. Sources: `host:/home/furbyhaxx/.pig/agent/skills/extending-pig/references/config-and-state.md`, bundled trust.md and staged SDK path/config helpers. Do not hardcode a home directory or import unrelated runtime environment assumptions.

| Layer | Location/ownership | Meaning |
|---|---|---|
| defaults | Typed versioned Workbench defaults | Complete baseline |
| user | sdk.AgentDir()/pig-workbench.json | User preferences/security policy |
| trusted project | host session cwd + sdk.ConfigDirName()/pig-workbench.json | Only if host project trust currently permits it |
| runtime | Session-owned memory | Explicit overrides; cleared on extension reload/session replacement |

Config project root is host session cwd, **not mutable Workbench cd**. Changing location must not silently load another repository's execution configuration. Remote/project-specific instruction classes are applicability context, not permission to read arbitrary remote Workbench config.

Effective precedence defaults < user < trusted project < runtime. Nested objects merge by known field; arrays replace as a whole; maps with stable keys (classes/tool contributions) merge per key, with explicit enabled:false to disable. Null is not an implicit delete; reset removes an override key from the selected layer.

One typed loader publishes immutable effective snapshots with configRevision and source attribution per key. Features do not re-open config files on every render/tool. Load after session cwd/trust are known; factory registration performs no disk/network configuration discovery.

# 2. Shape and validation

```json
{
  "schemaVersion": 1,
  "edit": {"mode": "hashline", "enforceSeenLines": true},
  "read": {"summarize": {"minTotalLines": 200, "prose": false}},
  "instructions": {"boundary": "project"},
  "jobs": {"yieldAfterMs": 0, "maxFinished": 20},
  "tools": {
    "build-tools": {
      "build_run": {"enabled": true, "density": "normal", "previewLines": 8}
    }
  }
}
```

Known fields reject wrong types/ranges/unknown enums, reporting file/key/expected/actual type and remediation. Reject duplicate JSON keys rather than silently allowing last-wins. Unknown fields warn and are preserved byte-for-byte where feasible or semantically without loss; an unknown field never silently activates behavior. Unknown future schemaVersion is a compatibility error, not permission to reinterpret fields.

Malformed existing file is not replaced with defaults. On startup expose a configuration error and block affected feature admission until fixed; diagnostic commands remain available. On reload retain the last valid effective snapshot marked stale with explicit error, rather than applying half a file. User must see whether saved values are active. Parse errors must not print secret-bearing source lines wholesale.

Config settings catalog has key, type, default, allowed scopes, bounds/choices, description, sensitivity, live/reload application, affected consumers and display section. Generate UI/validation/documentation from this single catalog where practical; do not maintain conflicting key lists manually.

# 3. Initial settings catalog

All defaults below are proposals subject to design review and measurements, not inferred donor behavior. Units are explicit.

| Group/key | Default | Scope/application |
|---|---|---|
| edit.mode | hashline | user/project/runtime; next turn schema+description refresh |
| edit.enforceSeenLines | true | user/runtime for weakening; project may only require true |
| edit.modelRepair.enabled | false | user/runtime; requires authorized inference |
| edit.snapshot.maxPaths/maxVersions/maxBytes/maxFileBytes | 256 / 4 / 67108864 / 4194304 | user/runtime; lowering evicts LRU safely |
| read.maxBytes/maxLines/concurrency | 51200 / 2000 / 4 | user/project/runtime; project can only reduce safety ceilings |
| read.summarize.enabled/minTotalLines/minBodyLines/minCommentLines/prose | true / 200 / 12 / 8 / false | user/project/runtime; cache-key revision |
| read.outputMaxColumns | 512 | user/project/runtime; cut lines remain unseen (SPEC-0006 G6.1) |
| read.query.defaultRows/maxRows/timeoutMs | 100 / 1000 / 30000 | user/runtime ceilings, project reductions |
| read.loopHintAfter | 3 | display hint, not a read lockout |
| instructions.enabled/boundary | true / project | user/runtime; project cannot widen boundary |
| instructions.classes | steering/backlog defaults from SPEC-0007 | bounded project context additions allowed |
| instructions.watch.debounceMs/remote | 100 / auto | user/runtime; no auto-install |
| instructions.updates.context/selfEdits | 3 / notice | user/project/runtime; safety baselines always persisted |
| frames.role | auto | policy selects supported role; project cannot elevate authority |
| frames.display.style/defaultVisibility | rail / shown | user/project/runtime presentation only |
| jobs.timeoutSeconds/yieldAfterMs/terminationGraceMs | 120 / 0 / 2000 | user/runtime; timeout is foreground default, explicit background has no implicit deadline |
| jobs.maxFinished/logBytes/sessionLogBytes | 20 / 10485760 / 104857600 | user/runtime; rotation/gaps explicit |
| jobs.wakeOnCompletion/rawLogs | false / false | user/runtime only |
| locations.worktreeParent | .worktrees | user/project/runtime within authorized repo |
| locations.sshHostWrite | include | user/runtime only |
| ask.autoSubmit/confirmDismissWhenDirty/presentSingleAsMulti | false / true / false | user/project/runtime display behavior |
| ask.remote.timeoutMs | 300000 | user/runtime |
| answer.timeoutMs/retries | 30000 / 1 | user/runtime; configured model/provider only |
| tldr.timeoutMs/concurrency | 120000 / 2 | user/runtime; configured model/provider only |
| tools.<owner>.<key> | contribution defaults | display fields from SPEC-0003 only |
| trust.rules | no extra grants | user/runtime only; never project |
| storage.finishedRetentionDays/outRetentionDays | 7 / 7 | user/runtime; active references protected |
| statusline.mode / statusline.layout | segments / donor default layout | user/runtime; `footer` only by explicit user choice (SPEC-0009 §9) |
| ask.history.enabled / retentionDays | true / 90 | user/runtime (SPEC-0009 §5, provisional) |
| edit.modeRules | donor model-family list → replace | user/runtime (SPEC-0005 G5.9) |
| bash.routeToTools | true | user/project/runtime (SPEC-0008 G8.15) |
| resources.fetchBackends.<key>.enabled / order | true / registration order | user/runtime (SPEC-0004 §11) |
| sessions.enabled/discoverable/inbound/wake | true / true / all / always | user/runtime only (SPEC-0011 §6) |
| sessions.wakeBudget/maxHops/rateLimit | 10 per peer-hour / 8 / 30 per peer-minute | user/runtime only |
| sessions.broadcast | project | user/runtime only (SPEC-0011 §6) |
| coordinator.classes / leaseHours / targetConsent | observe, message / 8 / firstAction | user/runtime only; enabling is a user command (SPEC-0011 §9.3) |
| coordinator.single / confirmDestructive / markTargets / frames / audit / answerAttribution | true each | user/runtime only |
| projects.offerRegistration / storageLinks | true / true | user/runtime; symlinked session buckets of SPEC-0012 §3.5 |
| guides.enabled / maxBytes / disabled | true / 2048 / [] | user/project/runtime presentation (SPEC-0012 §5.5) |
| docs.sourceVersionsKept / sourcePaths | 3 / docs, extensions/sdk, cmd, internal | user/runtime; fetch only on first access (SPEC-0012 §5.4) |
| instructions.agentsMdReminder / backlog.validatePointers | true / true | user/project/runtime (SPEC-0007 §2) |
| secrets.enabled / envInjection / keyFile | true / true / see SPEC-0013 Q1 | user/runtime; keyFile user only (SPEC-0013 §8) |
| activity.enabled / placement / shortcut | true / aboveEditor / alt+w | user/runtime (SPEC-0009 §9) |
| activity.items | state, agents, messages, jobs, instructions, skills, location, contributed, coordinator; each `show: auto` | user/runtime; `always` pins, `never` hides |
| activity.replaceWorkingIndicator | false | user/runtime; enabling requires verified retry/compaction events |

`Project.toml` (`[component]`, `[[components]]`, `[worktrees]`, `[artifacts]`, `[conventions]`) is project data, not settings: it is committed, parsed strictly, and cannot carry any key of this catalog (SPEC-0012 §3.2).

Additional adapter/backend size/pixel/decompression/grammar limits must be added to this catalog before those adapters register. This is an extensible catalog, not authorization to expose undocumented magic settings. Model selections use provider/id references resolved through current host catalog; no hardcoded stale “best model” default. Unset answer/tldr model explicitly uses current host model only if supported/authorized, otherwise asks for configuration.

# 4. Save transaction and consumer reconciliation

1. Resolve intended scope/file and recheck host project trust at save time.
2. Lock within process and across Workbench sessions. Lock acquisition bounded; no indefinite spinner.
3. Re-read file and compare baseline revision. Merge only edited keys; conflicting concurrent changes require user reconciliation.
4. Validate full candidate and scope restrictions. Reset removes key; never write the inherited/default value to simulate reset.
5. Write same-directory private temporary file (0600), flush/close, rename and sync directory where supported; preserve other keys/order/format where the parser supports it. No malformed-file overwrite.
6. Publish new effective snapshot atomically; reconcile all consumers or mark reload-required. Emit one settings.changed event with changed non-sensitive display fields.
7. Return saved vs effective values and any shadowing/reload requirements.

User can Save/Discard/Continue editing when dirty. A failed save retains staged edits. A successful file write followed by a consumer restart failure reports saved-but-not-applied, keeps last working runtime snapshot if safe and exposes reconciliation error. Do not report everything applied because rename succeeded.

cfg:// reads expose values and provenance. Writes use typed key/value/scope/revision, same validator/save path and confirmation policy. Never serialize credential inventories or allow arbitrary path writes through config. A project cannot enable custom notification commands, host-shell execution, model billing destinations or broader resource access.

# 5. Trust and authorization matrix

Trust means different things; keep them separate:
| Dimension | Source of truth |
|---|---|
| Whether project config/extensions load | PiG host project trust |
| Whether a tool operation is permitted | Host operation policy plus Workbench capability/scope checks |
| Whether a file is instruction vs data | Workbench class + user source policy, constrained by host |
| Whether a frame can use native elevated role | Granted producer authority + verified provider capability |
| External bridge identity | Approved authenticated bridge, not JSON owner fields. Inter-session peers: token from the 0600 record file plus peer UID where available (SPEC-0011 §3) |
| Presentation settings | Cosmetic; never grant execution permissions |

Workbench /workbench trust manages its source/path/host rules, not PiG's host /trust store. Host docs state saved trust changes may require restart; Workbench cannot claim immediate host trust activation. Nearest explicit rule over canonical root/authority wins; ties use most-specific path then last declared user rule, shown in inspection. Deny/ignore decisions cannot be overridden by lower-trust project content.

Remote authority defaults to untrusted data unless user explicitly grants instruction trust. Local project steering follows host/project source policy; explicit global user steering may have instruction authority. Trusted code/config does not make every file/tool output a system instruction.

Approvals bind operation type, canonical target(s), scope and relevant revision. If target changes between approval and commit, reauthorize. A broad URI alias, locate result, pinned file, nonce, short tag or producer registration is not an authorization token. Restrict credentials to the specific backend request; strip authorization on cross-origin HTTP redirect and never place secrets in display facts.

# 6. Storage domains and records

Use host-resolved config/agent/state roots (or platform equivalent), not current working directory. Workbench-owned state under <config-root>/state/pig-workbench; directories 0700/files 0600. Do not create a second session database replacing PiG JSONL.

| Data | Location/lifetime |
|---|---|
| Branch facts | Versioned custom session entries; replay active branch |
| Frame bodies/receipts | SPEC-0002 session entries; separate delivery state |
| Location/pins/ask/checkpoints | Structured session entries with explicit source/branch identity |
| Active jobs/watchers/services | Session supervisor memory, never revived by replay |
| Job metadata/logs | Private state/session/job directory; retention policy |
| Spilled outputs (`out://`) | Private state/session/out, referenced by resource URI |
| Project registry | `{agentDir}/projects/` (index, per-project registry copy, private notes, sessions, sidecars; SPEC-0012 §3.4) |
| Secret vault | `$XDG_DATA_HOME/pig-workbench/secrets/vault.jsonl`, encrypted values, shared by all sessions (SPEC-0013 §3.3) |
| PiG source checkouts | `$XDG_DATA_HOME/pig-workbench/docs/source/<version>/`, symlinked from `~/.pig/docs/source` (SPEC-0012 §5.4) |
| Scratch local:// | Private per-session directory under platform temporary root |
| Syntax/suffix/index caches | Rebuildable bounded state/cache; no secrets by default |
| Short-tag snapshots/registers | Branch-aware memory; named register facts persisted only under bounded explicit format |
| Runtime settings | Memory; reset on reload |
| Contribution preferences | User/project config keyed by stable owner/key |

Stored namespaces: pig-workbench.frame (existing SPEC-0002 decision), pig-workbench.frame-delivery, pig-workbench.location, pig-workbench.pin, pig-workbench.instruction-state, pig-workbench.ask, pig-workbench.checkpoint, pig-workbench.edit-register, pig-workbench.job-reference. All include schemaVersion, recordId, source identity and relevant revision. Event keys/operation IDs are durable only where the domain contract promises it.

Record facts, not rendered ANSI; canonical payload limits apply. Read/parse every historic version the released extension actually wrote. No migrations for imaginary donor formats; if an actual format changes, add explicit migration/read compatibility and fixtures in that change. Unknown versions remain inspectable as unsupported and block unsafe reconstruction rather than defaulting to empty state.

Session replay derives location/pins/instruction/ask/checkpoint state from the active branch, not every entry in the file. On fork, original provenance remains; new records get new IDs. Live jobs are session process facts independent of branch cursor. No-session mode uses in-memory facts, labels durability honestly and cleans private artifacts on orderly shutdown subject to user export choices.

# 7. Cleanup, privacy and crash behavior

Retention does not delete active job logs, pending ask state, active artifact references or current-session scratch beneath running operations. Cleanup is ownership-checked, bounded and explicit about expired references. Session deletion is host-owned; Workbench may clean its own orphan state only after verifying the session relationship. No whole-tree cleanup outside owned roots.

Atomic state writes prevent torn JSON; append journals tolerate incomplete last record with diagnostic. Reconcile receipts after crash before retrying mutation. Do not relaunch shell jobs, reissue inference or replay user submissions because a pending record exists.

Secrets: [SPEC-0013](0013-secrets.md). Nothing here hides data from the user, the screen or local files.

Session catalog ([probes](../analyses/2026-11-02-host-integration-probes/README.md) P24–P25): `SessionManagerListAll` returns every project's sessions with `allMessagesText`, also in `--no-session` runs. Workbench exposes all of them through `session://` (SPEC-0004) without consent steps. In-memory sessions have no file (`GetSessionFile` = nil) but do have an ID. Skills come from `GetSystemPromptOptions().Skills` (enabled set, path, scope, `disableModelInvocation`), not from a directory scan.

Debug payload capture is off by default; opt-in evidence goes to a private local path with bounded retention, never to the stdout protocol stream.

# 8. Lifecycle ownership

One session supervisor owns registrations, pending operations, timers/watchers, process groups and cache lifetimes. Each request has cancellation and immutable session/generation/config/location capture. Shutdown stops admission, cancels requests, drains queues, stops/reaps owned jobs, flushes bounded durable facts and unsubscribes events. A late result cannot write into a new session's UI/state.

Reload creates a new generation: dormant settings remain, contribution handles are rediscovered, runtime overrides reset, branch facts rebuild, jobs follow SPEC-0008 shutdown policy. Feature-specific reconciliation runs before tools resume mutations. Never retain an SDK request context after its handler returns; long-lived work uses explicit session ownership and bounded cleanup.

# 9. Acceptance

- Correct host path helpers under alternate agent/config roots; no hardcoded ~/.pig leakage.
- Layer precedence, keyed-map/array behavior, reset, unknown keys, duplicate keys, malformed/unsupported versions and project trust gating.
- Concurrent sessions save different/same keys, interruption around rename, lock timeout, saved-but-not-applied consumer failure.
- Security keys denied from project config; runtime changes cannot fake host trust.
- Replay/fork/compaction rebuild correct branch; unknown stored version blocks unsafe operations.
- No-session durability labels and cleanup; ownership checks prevent deleting another session's data.
- SPEC-0013 §11 acceptance.
- Crash recovery refuses PID reuse and mutation replay; retention protects active references.
- Schema/UI/docs generated from same catalog or consistency-tested; per-feature tests prove changed consumers applied settings.

Cross-cutting decisions requiring approval: final storage/retention defaults, capability-specific trust grants, custom notification execution, inference provider configuration and platform-specific lock/atomicity behavior. Draft defaults are not permission to alter user configuration.

# 10. Donor parity (M0.1)

From the [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), pi-workbench settings tests. Normative.
- **G10.1** The project layer can never define trust rules; only the user and runtime layers can.
- Saves re-read the file and are serialised, so concurrent disjoint saves both survive, including across processes. A symlinked config file stays a symlink and its target is updated.
- Explicit `false`, `0` and `[]` override lower layers and are written back.
- Conflicting key bindings are refused at every scope. Inherit and promote work between scopes. Runtime saves stay in memory.
