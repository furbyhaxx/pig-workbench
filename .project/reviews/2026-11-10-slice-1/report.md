---
document:
  type: "review"
  id: "REV-2026-11-10-slice-1"
  title: "Slice 1 review and gap analysis"
  description: "Ranked findings on Slice 1 at 00649e9 against PLAN-0001 §9, PLAN-0002, SPEC-0001 to SPEC-0014 and the extending-pig UI standards"
  status: "published"
  revision: 1
  created_at: "2026-11-10T08:03:36+01:00"
  updated_at: "2026-11-10T08:03:36+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User asked for a comparison, review and gap analysis of the completed Slice 1"
      changes: ["Gates, static scans, wiring and concurrency reading, fourteen spec comparisons, live tool and TUI exercise, docs and duplication survey"]
---

# 1. Verdict

Slice 1 is **not ready to be the base other extensions build on**. The engines are real and mostly sound, the wiring is thin and disciplined, and the unit and race gates are green. But one defect breaks everyday shell use, three more break sessions that run long or have normal amounts of data, and several things PLAN-0002 §1 forbids ("no stubs", "not advertised if not in this slice") are present.

Scope: commit `00649e9`, 10.11.2026. Review only; no product code was changed.

**How findings were observed.** *live* = seen in this PiG session, which runs Workbench, or in an isolated tmux session; *test* = a gate or a throwaway test; *static* = a tool; *read* = code reading only, not executed.

In this document URI schemes are written without their two slashes (`https:`, `agent:`), because of finding C1.

# 2. Gates

| Gate | Result | Evidence |
|---|---|---|
| `just check` | pass | `evidence/check.log` |
| `just race` | pass, 27 packages | `evidence/race.log` |
| `just integration` | **fail**, 1 test | `evidence/integration.log` |
| staticcheck | 29 notes, 4 unused items | `evidence/staticcheck.log` |
| TODO, FIXME, stub markers | none | grep |

The failing test is `TestLiveTrustPanelAddsAndRemovesARulePersistently` (`internal/host/live_menu_test.go:476`). It fails 2 of 2 times with the `TMPDIR` the inventory prescribes and passes with a short `TMPDIR`: the typed path scrolls out of the input. A test defect, not a product defect, but the gate is red as documented.

# 3. Findings that block the slice exit

## C1. `bash` refuses any command that mentions a registered scheme (critical, live)

`hasSchemeArg` (`internal/host/schemecommand.go:143-150`) is a substring test over the whole command text, and `dispatchSchemeCommand` (`:180-185`) treats every non-locatable scheme this way, including `http` and `https`. Refused in this session, each with "scheme paths require a standalone ls, cat, rm, mv, cp, mkdir or > redirect":

- `curl -sI` on an `https:` URL
- `git log --grep` and `grep -rn` with a pattern that contains a scheme name
- `echo` and a heredoc whose text mentions `agent:`
- a loop whose `sed` expression contains colon-slash-slash

With Workbench loaded an agent cannot `curl`, `git clone`, `go install` or `docker pull` by URL, and cannot grep for or commit text that names a scheme. It hit this review four times. The setting `bash.route-to-tools` exists but is not consulted.

SPEC-0008 §9 (G8.20) has the same gap: it does not exempt schemes that external programs understand. Fix code and spec together: consider only static argv words that start with a scheme prefix, exempt `http` and `https`, ignore quoted text and heredoc bodies.

## H1. The frame ledger stops accepting frames after 2 MiB per session lineage (test)

`bodySize` in `internal/frames/ledger.go:184,220` only grows; delivered, cancelled and expired records keep their bodies, and the total is restored on resume. A throwaway test admitted and delivered 256 frames of 8 KiB, then `Admit` failed with `ErrLimitExceeded`. From then on `before_agent_start` returns the error (`internal/host/frame_context.go:373-376`): no context frames, no instruction updates and no forced system prompt for the rest of the session. `frames.publish` from other extensions and `agent:` messages fail too. SPEC-0002 §4 bounds *pending* frames, not history. Account pending bodies only, or drop terminal bodies and keep receipts.

## H2. `read` with `paths` never triggers instruction discovery (live, twice)

`toolInstructions` (`internal/host/instruction_context.go:136`) reads `input["path"]` and hashline sections, not `input["paths"]`. A `paths` read of `internal/jobs/process_unix.go` delivered nothing; the single-path read that followed delivered `internal/jobs/AGENTS.md`. This violates PLAN-0001 §6.3 and SPEC-0007 §4. The tool description recommends `paths` for batches, so nested `AGENTS.md` files are routinely missed. No test covers it.

## H3. Session listings fail above 16 entries (live)

`ls` of the `session:` root and of `by-id/` fail with "provider exceeded the requested listing entry limit" on this machine (31 sessions in the bucket). `List` in `internal/host/sessionprovider.go:179-197` ignores limit and cursor, while `internal/host/list.go:126-133` asks for pages of 16 and rejects a larger page. `agentprovider.go:74` and `skillprovider.go:76` follow the same pattern and were not exercised above 16 entries.

## H4. Stubs and inert surface, against PLAN-0002 §1

- `session:self/system-prompt` returns one hardcoded sentence (`internal/host/sessionprovider.go:302-303`), and the sentence is out of date. `s.promptText` is already retained. (live)
- **65 of 100 settings control nothing** (`internal/config/catalog.go:171`): they validate, save and appear in the panel. In Slice 1 scope: `prompt.template` (the host always renders `builtin/workbench`, `commands.go:283`, `frame_context.go:441`), `guides.*`, `frames.role`, `frames.display.*`, `secrets.env-injection`, six `jobs.*` limits, `edit.snapshot.max-*`, `read.concurrency`, `context.timezone`, `skills.conditions`, `projects.offer-registration` (the project offer cannot be switched off). The rest belong to Slice 2 and 3 features that do not exist. (read)
- Unused code: `fetchRouter.list`, `statusBoard.list`, `jobCatalog`, `managerLogWriter` (`evidence/staticcheck.log`). (static)

## H5. Skills cover only the working directory (live)

The skill tree is fed from `<cwd>/.agents/skills` and `<cwd>/.pig/skills` only (`internal/host/app.go:305-308`). In this session 18 host skills are active, the `skill:` root is empty and `/workbench skills` says "no skills" under a PiG header that lists eight. SPEC-0014 §8.1 names PiG's discovered skills as the source. Load, unload, `when`, `auto_load`, the listing budget and replay after compaction therefore apply to almost no real skill. The task that was to verify this (17.8.7.2) was abandoned.

## H6. Frames are shown to the user as raw envelopes (live, capture)

No message renderer is registered (`Register` in `internal/host/app.go:115`), although the SDK offers `Extension.MessageRenderer`. Each frame appears in the transcript as `[pig-workbench.context]` followed by the XML envelope and JSON: about 70 lines at session start and 11 more per `cd` (`.project/analyses/2026-11-05-slice-1-inventory/evidence/own-tool-rows-tui/tool-rows/w140.txt`). SPEC-0002 §7 specifies one row per frame with an expand hint. Skill and ask messages are the same.

# 4. Medium findings

**Soundness**

- **Start race** (read). `app.start` publishes `a.session` (`app.go:321`) and then assigns `s.endpoint` (`sessions.go:185`) and `s.activity` (`activity.go:170`) unlocked; handlers read them (`services.go:88`, `ask.go:146`, `agentprovider.go:51`). The race gate does not exercise it.
- **Job retention** (read). Every foreground `bash` call is a retained job and only 20 finished jobs are kept (`internal/jobs/AGENTS.md`, `MaxFinished`), so 20 quick commands can retire a finished background job and its log before the model reads it.
- **Edit lock files** (live) are never removed: one per file ever edited under the agent directory's `workbench/locks/edits/` (131 after a day here); `internal/fileedit/lock.go` has no sweep.
- **Secrets** (live). A `.env` file outside the cwd chain, read through `read`, returned its `API_KEY` value in plaintext. SPEC-0013 §2 requires learning any env file a tool call references; `internal/host/secrets_env.go:79-100` learns only the process environment, the home `.env` and files from repository root to cwd. Token-shaped values were redacted correctly.

**Contract gaps other extensions will meet**

- SPEC-0001 §4 `operation.status` is not implemented (`internal/host/services.go:162`) and §5 `frames.deliveryChanged` is never emitted. The product docs say so; the approved spec does not. (read)
- `session:self/frames`, `session:self/instructions/` and, under `<id>/`, `tree` and `messages` do not exist. PLAN-0002 §6 names `self/instructions/` as the script-facing state for print and JSON mode. (live)
- `jobs` `list` returns Go-marshalled JSON with PascalCase keys and nanosecond durations and without command, cwd or start time. `bash` has no `title` or `cwd` argument and `jobs wait` requires an id, unlike SPEC-0008 §5. (live)
- SPEC-0009 §8 subcommands assigned to Slice 1 are missing (`internal/host/commands.go:290-330`): `frames ls|show|hide|collapse`, `prompt show|render|diff`, `skills ls|load|unload`, `tools --format`, `jobs monitor|write`. `/cd`, `/wb:cd`, `/tools` and `/wb:jobs` have no argument completions (`commands.go:20-27`). (read)
- The activity line lacks the state item, instruction files and away-location that SPEC-0009 §9 requires (`internal/host/activity.go:52-73`), and it polls the endpoint registry every 500 ms (`activity.go:21,60`). (read)
- Reading a bare `https:` origin without a trailing slash fails (`internal/resource/http.go:187`). (live)

Each needs a decision: implement, or revise the spec and move the row to PLAN-0003.

# 5. TUI and UX

Captured live at 36, 57, 80 and 140 columns and at 80x16; nothing overflows.

| Area | State | Gap against `extending-pig` |
|---|---|---|
| Tool rows | The ten own tools share one grammar and all states through one function; contributed tools share the header grammar | No glyph, colour or density, so `edit · failed` looks like every other row. Agreed deferral (PLAN-0004), but it is the failure signal |
| Renderers | Two parallel ones: `own_tools_render.go` from arguments and result text, `presentation_render.go:103` from snapshots | The own tools do not use the SPEC-0003 contract they define for others |
| Frames | Raw envelopes (H6) | No row, no visibility setting |
| Panels | Border, title, body, status, hint (`panels.go:63`); search, scope, provenance and staged save in settings | Height follows content; not the dashboard frame (deferred) |
| Keys | — | Single-key actions in five panels (`jobs_panel.go:180-186`, `instructions_panel.go:101-109`, `sessions_panel.go:97-117`, `trust_panel.go:68-70`, `settings_array.go:79`) and hardcoded `enter`, `ctrl+s`, `tab` (`settings_panel.go:223`) instead of `sdk.KeyHint`: a remap makes the hints wrong |
| Commands | One handler per verb and alias | Verbs differ by family: `ls` (`instruction_pins.go:323`), `list` (`projectmanage.go:81`), `rm` (`trustrules.go:157`); no alias canonicalization |

Smaller (live): hub descriptions are cut without an ellipsis, empty states differ in wording and case, result bodies show absolute paths while call rows show relative ones, the unseen-line refusal prints `[{100 100}]`, job IDs are 32 hex characters on every `bash` result.

The single-key actions are not covered by the chrome deferral; they are an interaction rule, and the README documents them as the interface.

# 6. Documentation and process

- 80 of 100 settings are not in `docs/configuration.md`, against the root `AGENTS.md` rule.
- C1, H5 and the system-prompt stub appear nowhere in the README limitations or `docs/tools.md`.
- The inventory's PASS table was verified at `eb55bf7`, 141 commits before HEAD. It marks rows PASS that this review contradicts (core trees, the instruction barrier, the skills cases). PASS there means "a named test passes", not "the spec sentence holds".
- PLAN-0002 exit items 2 (progress table in the plan) and 6 (handoff naming the exit commit) are open. Both plans are still `draft`. PLAN-0001 Progress still says no tool is implemented, and SPEC-0002 §1 still says nothing is implemented.
- `AGENTS.md` routes open work to `BACKLOG.md` and a backlog directory under `.project`; neither exists, so known open items live in plan prose.
- `internal/modelknowledge` has no `AGENTS.md`; `internal/AGENTS.md` omits seven packages.

# 7. Duplication

No large clone groups (static). Repeated primitives:

- `syncDirectory` four times (`config/sync_unix.go:11`, `endpoint/platform_unix.go:37`, `fileedit/platform_unix.go:12`, `project/registry_platform_unix.go:25`), with Windows twins that behave differently; `nonblockFlag` three times although `internal/fileio` exists for it.
- `sameToken` (`endpoint/registry.go:13`, `messaging/wire.go:169`) and `validSessionID` (`endpoint/record.go:117`, `frames/frame.go:206`): two definitions of a valid session ID.
- Six rename-based atomic-write sites and six lock-file sites across `config`, `endpoint`, `fileedit`, `project`, `secrets`, `jobs` and `sessionstore`.
- Two tool-row renderers (§5); signal-name validation twice (`host/jobprovider.go:238`, `host/jobs.go:296`); text sanitizers spread over `host` and `resource`.

# 8. What held up

- Hashline editing: block replace, cut and paste across files, CRLF, unseen-line refusal and stale-tag relocation (live).
- Read and search: literal filenames with colon and comma, tails, ranges, binary refusal, per-target errors, code summaries, honest incompleteness (live).
- Jobs: background run, completion notice with wake, the job tree; `cd` without changing the host cwd (live).
- Instruction placement broadest to nearest and host files first; the six context frames; token-shape redaction (live).
- Lifecycle: admission per operation, joined shutdown, no mutex across Emit, one guarded package-level variable (read).

# 9. Not verified

- Sending between sessions; it would have woken the user's other live agents.
- Windows, the fused binary, real-provider inference and the pig-web co-load.
- Compaction and branch behaviour beyond the existing tests.
- The start race (§4) comes from reading, not from a reproduction.

# 10. Recommended order

1. C1, with the SPEC-0008 revision.
2. H1, H2 and H3, each with a regression test at realistic size.
3. H4: serve the real prompt; remove or hide inert settings until their consumer exists.
4. H5 and H6: decide the skill source, register a frame renderer.
5. Single-key actions and `KeyHint`; completions; the start race; lock cleanup.
6. Decide each §4 contract gap; revise specs or move rows to PLAN-0003.
7. Re-run the inventory at the fix commit, repair the trust-panel test, then write the exit handoff.
