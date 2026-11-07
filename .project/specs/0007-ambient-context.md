---
document:
  type: "specification"
  id: "SPEC-0007"
  title: "Instruction and ambient-file selection, injection and updates"
  description: "Ancestor and root-nearest policies, pins, capped backlog views, watchers, diffs and context epochs"
  status: "approved"
  revision: 7
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-07T08:00:56+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Complete the remaining Workbench specification and design areas"
      changes: ["Defined behavior, state, safety, examples, dependencies and acceptance criteria"]
    - revision: 2
      reason: "M0.2 probes verified host context-file control and projection timing"
      changes: ["Specified structural suppression through systemPromptOptions.contextFiles", "Specified per-request projection via the context hook and resume detection"]
    - revision: 3
      reason: "M0.1 donor parity audit"
      changes: ["Added donor parity rules for remote watching, watch modes, idle delivery and races"]
    - revision: 4
      reason: "SPEC-0012 project model and URI namespace"
      changes: ["Class projectPrivate", "AGENTS.md reminder and BACKLOG.md pointer validation", "Pins through session://self/instructions", "Scheme guides follow the first-touch model"]
    - revision: 5
      reason: "All config files are TOML (SPEC-0010 §1)"
      changes: ["Setting keys renamed to kebab-case"]
    - revision: 6
      reason: "Several instruction files per directory (SPEC-0014 §9)"
      changes: ["Class field select = first | all", "Configuration example in TOML with kebab-case keys (SPEC-0010)", "Prompt engine owns the stable set when enabled"]
    - revision: 7
      reason: "User approved the remaining five specifications on 07.11.2026"
      changes: ["Approved the ambient-context contract for Slice 1; implementation and acceptance remain separately tracked"]
---

# 1. Purpose and inputs

Specification for instruction discovery and user-defined ambient file classes. Implements the user's two use cases without conflating them:
- BACKLOG.md: touched directory plus **all ancestors**, with bounded pointer-item injection.
- Other repeated files: root-most plus nearest, or a chosen pinned instance, avoiding intermediate duplicates.

Depends on [frames](0002-message-frames.md), [resources](0004-resource-providers.md), [locations](0008-locations-jobs.md), [configuration/state/trust](0010-configuration-state-trust.md). Prior art: `host:/mnt/Projects/furbyhaxx/pi-workbench/src/instructions/{discover,manager,runtime,ledger}.ts` and nugent's InstructionLedger/instruction_touch. Their lifecycle model is reused, not their TypeScript runtime assumptions or alternate-harness filenames.

The engine does not create backlog items, claim tasks, execute file contents or treat every read document as instructions.

# 2. Classes and configuration

Configuration uses keyed `instructions.classes` tables in `Workbench.toml`; class IDs are stable. With `select = "first"` (default), ordered filename alternatives select the first existing candidate **per directory**; with `select = "all"`, every listed file that exists in a directory is selected, in list order (e.g. `AGENTS.md` and `CONVENTIONS.md`). Different classes compose independently. Includes add explicit candidate patterns; they do not imply recursive injection of every matching file.

```toml
[instructions]
boundary = "project"

[instructions.classes.steering]
filenames = ["AGENTS.override.md", "AGENTS.md"]
select = "first"
placement = "ancestors"
content-role = "instruction"
update-mode = "auto"
allow-pin = false

[instructions.classes.backlog]
filenames = ["BACKLOG.md"]
placement = "ancestors"
content-role = "context"
item-format = "backlog-pointers"
max-items = 25
max-lines = 200
max-bytes = 16384
update-mode = "auto"
allow-pin = true
pin-mode = "add"

[instructions.classes.project-private]
include = ["{projectDir}/private/*.md"]
placement = "root"
content-role = "instruction"
max-bytes = 16384
update-mode = "auto"
allow-pin = true
pin-mode = "add"

[instructions.classes.architecture]
filenames = ["ARCHITECTURE.md", "DESIGN.md"]
placement = "root+nearest"
content-role = "context"
max-lines = 200
max-bytes = 16384
update-mode = "auto"
allow-pin = true
pin-mode = "replace-nearest"
```

`project-private` reads the personal notes of SPEC-0012 §3.6 from `{projectDir}` (the project's registry directory, outside any repository). It is user-owned, so it carries the user's authority, not the project's.

Recognition of AGENTS.override.md preserves the existing proposal's precedence; Workbench authors steering in AGENTS.md only and creates no alternate harness-specific files. Users may explicitly configure additional names; never auto-discover CLAUDE.md/GEMINI.md as aliases.

Class fields:
| Field | Contract |
|---|---|
| filenames / include | Basenames / authorized relative patterns; bounded matching |
| select | `first` (alternatives, first existing per directory) or `all` (every existing one, list order) |
| placement | ancestors, root+nearest, nearest or root |
| content-role | instruction or context; does not itself grant authority |
| update-mode | auto, diff or full; diff still requires valid delivered baseline |
| max-items/max-lines/max-bytes | Positive view budgets; item format required for item counting |
| allow-pin/pin-mode | Policy, not automatic permission; add or replace-nearest |
| after-compaction | reinject or on-touch; root/global/pinned steering always rebuilt |
| enabled | Disable class selection with explicit withdrawals |

Conventions (SPEC-0012 §4.2), each a user/runtime setting: when an agent creates a directory that holds a module, package or crate manifest and no `AGENTS.md`, a reminder frame names the `agents-md` skill (`instructions.agents-md-reminder`, default true). `BACKLOG.md` pointers are validated when the file is selected: a target that does not exist or whose `status` is not `"open"` is shown in the view, never silently dropped (`instructions.backlog.validate-pointers`, default true). Scheme guides (SPEC-0012 §5.5) reuse this first-touch and after-compaction machinery with `after-compaction = "on-touch"`.

Project classes can add bounded context within the trusted project; they cannot widen boundaries/trust, enable external paths or change authority to instruction without user policy. User config owns safety-affecting controls. A filename that matches several classes is deduplicated by canonical identity while preserving all applicability reasons; differing role/trust settings use the less-privileged effective interpretation and expose the conflict.

# 3. Roots, boundaries and targets

A location supplies authority/host, cwd and project/worktree root. A touched file uses its containing directory; a directory target uses itself. Resolve symlinks/canonical identity for containment and retain display path separately. Nested repository/worktree boundaries stop inheritance from an enclosing checkout by default; explicitly configured global/host steering remains additive.

- project: current repository/worktree root, or captured origin if no repository exists.
- home: ascend only to the resolved authority's home if target lies below it; otherwise project.
- root: filesystem root, explicitly enabled by user, independently per local/remote authority.

Global user steering and authorized host notes are separate broad scopes. Includes outside the boundary need explicit authorized roots; no glob escape via .., symlinks or host alias.

Selection for the chain `/repo → /repo/a → /repo/a/b`:
| Available copies | ancestors | root+nearest | nearest | root |
|---|---|---|---|---|
| all three | all three | /repo + /repo/a/b | /repo/a/b | /repo |
| only /repo/a | /repo/a | /repo/a once | /repo/a | none |
| /repo + /repo/a | both | both | /repo/a | /repo |

Thus root+nearest means **broadest existing applicable match**, not “inject nothing if the root has no copy”; root alone means exact boundary root. Root/nearest coincide → inject once.

Every tool batch captures immutable location/config/branch generation. For explicit file/SSH URIs outside the active cwd, compute an authorized target-specific root; do not inherit instructions from the wrong host/project. Search touches its declared root and result-containing directories only within bounded admission; do not traverse every descendant just for instruction discovery.

# 4. Concurrent touches and retention

The active scope is the union of:
1. Global/host and cwd-derived scopes.
2. Path scopes currently leased by admitted operations/batches.
3. Ancestor-policy scopes touched in the current context epoch (until cwd boundary changes or compaction).
4. Explicit branch pins.

For root+nearest/nearest, retain the most recent **completed batch target set** after its leases close, not the last goroutine's target. Each batch receives a sequence at admission; completing an older batch cannot replace a newer batch's retained selection. During overlapping batches, union their selected instances. At safe boundary release, withdraw no-longer-applicable nearest instances deterministically. Pins with replaceNearest replace automatic nearest selection for that class on the same authority/boundary, not across unrelated projects. Root-most stays; additive pins do not suppress others.

Sort delivery: global → host → broadest-to-nearest path depth → configured class order → canonical URI. Same bytes at equal trust may share one content delivery, but each applicability reason remains tracked.

A cwd transition builds the full new selection before publishing it, then emits required withdrawals/additions/relocations in one ordered reconciliation. It cannot erase already delivered text; withdrawal explicitly tells the model that prior content no longer applies.

# 5. Before-tool instruction barrier

Read-only operations may discover context and deliver it alongside the result before the next model continuation. Mutations must not execute newly authored changes under undisclosed newly discovered instructions.

When a mutating call targets a file with a new/changed applicable instruction view not yet delivered in the active epoch:
1. Resolve/discover and queue the new baseline/change.
2. Do **not** mutate.
3. Return typed instructionContextChanged plus actionable retry information.
4. Model receives the frame and prepares a new call. That call revalidates scope/revision before commit.

The retry is required: the host offers no before-tool hook that lets the model see new context before the mutating call executes. A host-native before-tool mechanism may avoid the retry only if it proves the model had a chance to incorporate the instruction, not merely that an event fired before the syscall. Multi-file hashline preflight discovers all targets first, so no first file commits before a later file exposes missing instructions. Codemode cannot bypass this barrier by hiding targets; tool-level hooks remain authoritative.

Bash cannot statically reveal every future touched path. Inject cwd ancestry before launch, optionally accept explicit declared roots, and reconcile changes after execution. Document this coverage limit; do not claim arbitrary shell writes were preflighted. Known instructions should guide the agent to use edit/write where path-sensitive barriers matter.

# 6. Pins

Pin request: classId + canonical URI + expected class/config revision; branch-scoped fact with origin user/agent and pinMode. Commands: `/workbench instructions pin <path> [--class <id>]`, unpin, ls, rules, reload. Ambiguous class matches require selection. Agent path: write `session://self/instructions/ctl` (`pin <uri> [--class <id>]`, `unpin <uri>`); same policy checks apply.

Pins persist on branch replay/fork and receive live changes across cd. They cannot expand trust, read permission or cross-project consent. replaceNearest can be selected only by an allowed class; ordinary steering disallows pins by default so an agent cannot pin a convenient steering file to suppress closer rules. A removed pinned file remains an unavailable pin record with a removal notice, not stale active content. Unpin recomputes automatic selection; no disappearance before replacement baseline is ready.

User can inspect effective selection before confirming a class/pin change. Reading/pinning is not evidence of executing or completing backlog work.

# 7. Capped views and diffs

Maintain two independent identities:
- Source revision/digest: exact full source bytes.
- Delivered view revision/digest: capped content and continuation actually injected.

For `backlog-pointers`, an item is a complete pointer record `- B-NNNN kind priority — title → path`; each valid line counts once. Preserve comments/non-item lines within maxLines/maxBytes; malformed pointer lines remain visible with a diagnostic, never silently discarded. Do not use a loose “any nonempty line” guess to claim an exact item count.

Inject first N items in file order, then `(+K more → canonical-or-repo-relative-path)`. Count omitted items by bounded full scan when possible. If scan limits prevent exact count, say “more items; count unavailable” rather than invent K. Keep a complete item or omit it; never cut a pointer in half. Caps affect injection only, never mutate BACKLOG.md. Source read still obeys ordinary paging.

For any class, apply item/line/byte budgets together, reserving footer space. Steering truncation is explicit and conservative: emit partial baseline with continuation, block mutations requiring the missing steering until it is delivered, or fail the prerequisite; never treat truncated steering as complete. Context-class cap does not imply every pointer body is loaded.

Update auto: unified diff against previous **delivered view** when smaller and reconstructible, otherwise full view. diff mode still falls back to full baseline after compaction/missing base, with reason. Changes beneath the cap can alter source revision/omitted count without a visible content hunk. Cap changes or item reordering produce a correct new view; diffing full unseen source is forbidden.

# 8. Watchers, self-edits and delivery ledger

Watch selected files and candidate directories for create/delete/rename/atomic replace; missing-candidate cache entries expire on directory change. Default debounce 100 ms. Bound watch count and scan work; if native watcher unavailable, reconcile at turn boundaries and relevant tool results. Remote auto uses an approved available watcher or on-turn stat/hash; no auto-install of remote tooling and no hidden new transport just to watch.

A check uses canonical URI and content digest, not only mtime/size. Coalesce repeated changes to the latest unsent view but preserve removals/trust changes. Discovery failures retain a visible stale/unavailable state and prevent safety-relevant mutation; they do not withdraw instructions as if absence were proven.

Agent self-edits:
- notice: emit concise change acknowledgement; keep complete replacement view in persisted ledger for replay.
- diff: emit the same reconstructible view diff as external edits.
- silent: suppress redundant UI/model echo only when the authoring tool result supplied the actual applied change; persist ledger update.
If auto-format/repair/backend transformation changed bytes the model did not author, use diff/full instead of pretending silent self-knowledge.

Ledger is branch/context-epoch scoped, rebuilt from structured frame records. Equal bytes **and trust** plus compatible applicability permit SkipIdentical. Relocation references existing same-epoch content and updates scope without resending. Loaded/added/changed/selfEdit retain replacement view; removed/withdrawn remove applicability; pinned facts are independent of content frames. Do not advance delivered baseline on enqueue alone: use the delivery status available in SPEC-0002.

On compaction, invalidate diff bases and seen state; rebuild global/root/pins plus nested reinject scopes. on-touch nested scopes wait for their next relevant access. A failed compaction retains the prior epoch. Session/tree/fork transitions cancel stale discovery work and reconcile before accepting new mutations.

# 9. Trust, host integration and external notifications

When the prompt engine is enabled (SPEC-0014 §9), Workbench owns the whole prompt: host `contextFiles` are cleared and the stable set is rendered by the template; the rest of this paragraph applies while `prompt.enabled = false`. Suppress only host project_context content that Workbench demonstrably owns and reproduces; do not string-replace arbitrary system prompt sections or discard unrelated context. Verified mechanism ([probes](../analyses/2026-11-02-host-integration-probes/README.md) P23): in every `before_agent_start`, remove exactly the owned paths from `data["systemPromptOptions"]["contextFiles"]`; the host re-renders the prompt without them. The edit is per prompt and must be repeated; never return a `systemPrompt` string (the value is a `*sdk.SystemPrompt`, and a string would replace the whole prompt). If another extension re-adds a suppressed path, report the conflict and keep both copies rather than fight over it.

Projection timing (P5, P21): instruction views and pins are appended by the `context` hook to every outgoing provider request, exactly once per request. The host does not persist them, compaction summary requests do not pass through the hook, and after compaction the hook sees `compactionSummary` plus kept messages — so the "rebuild after compaction" rule is satisfied by recomputing the projection on the next hook call. Resume (`--continue`) reports `session_start.reason = "startup"` (P22); Workbench detects resume by finding its own records on the restored branch, not by the reason. Disabling Workbench instructions restores host behavior at a safe boundary with visible reconciliation.

ContentRole and source trust remain separate: BACKLOG.md is context, not a new system-command authority. Pinning, filesystem proximity, frame nonce and identical-byte dedup never elevate trust. User/global/remote steering authority follows SPEC-0010.

External claim/completion messages enter through registered frame producers with project/path scope and event deduplication. Workbench filters path overlap and renders/delivers facts; it implements no task store. Session bus does not provide project-wide cross-process fan-out. A separately approved bridge must supply authenticated project events when that feature is needed.

# 10. Acceptance

- Selection table, filename precedence, `select = "all"` order and dedup, root missing, same root/nearest and linked worktree boundaries.
- Parallel multi-read/edit batches in different subtrees, completion out of order and cwd transition during pending discovery.
- Pin add/replace-nearest/unpin, deletion, trust revocation, fork/resume and authority separation.
- BACKLOG first-N exact counts, malformed pointers, oversized source, hidden change, cap change and item reorder.
- Diff application reconstructs injected view byte-for-byte; no use of an absent epoch baseline.
- New file, atomic replacement, watcher overflow, remote disconnect and turn-boundary healing.
- Mutation blocked before newly discovered instructions; codemode cannot hide affected targets.
- Self-edit with actual transformation vs authored bytes; failed persistence never marks content delivered.
- Equal bytes/different trust do not deduplicate; worktree identity alone does not prove sameness.
- Host duplicate context suppression and restoration proven in live tests; no false “all agents notified” claim from a local event bus.

# 11. Donor parity (M0.1)

From the [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), pi-workbench `instructions/*` at `bc7ce0e`. Normative.
- **G7.1 Remote watching.** Detect an already-installed remote inotify tool once per host, otherwise fall back to on-turn checks. A forced native mode that fails names the host and the recovery step. The remote monitor exits on stdin EOF, restarts with backoff, and is torn down by abort with a process-group kill fallback. NUL-separated events are parsed across chunk boundaries.
- **G7.2** `instructions.watch = auto|native|off`. `off` never rechecks remote host instructions on later prompts, withdraws remote ancestors on a local transition without remote reads, and still checks local changes.
- **G7.3 Idle delivery.** Changes while idle coalesce to the latest view and are delivered at the next prompt. Idle compaction defers to the next prompt's full baseline. An unreachable restored host never blocks local deliveries; host connection failures are reported at the prompt.
- **G7.4** BOM stripping never produces a false change. Local reads report caps, skip directories and honour abort. SSH reads fetch all ancestors in one batched round trip, keep first-filename precedence, and distinguish absent from empty.
- **G7.5** A cancelled self-edit cannot acknowledge or clear a newer edit. A self-edit suppresses the matching watcher diff and emits exactly one notice.
- **G7.6** A prepared plan applies exactly once without further filesystem reads. A root or manager revision change discards a stale plan or scan. An aborted preparation changes nothing.
- **G7.7** An authorised SSH host note delivers its body as host-scoped steering. A note with only frontmatter delivers nothing.
- **G7.8** SSH host/path trust rules are isolated from local and global rules. Host trust defaults apply only to local sources.
