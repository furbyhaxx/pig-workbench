---
document:
  type: "handoff"
  id: "HO-pig-web-m0-4-inputs"
  title: "pig-web inputs for the Workbench M0.4 contract freeze"
  description: "Seams pig-web needs settled in the frozen contracts: extension inference and secrets, cutoff in session.model, descriptor summaries, the !command discrepancy, activity items"
  status: "archived"
  revision: 2
  created_at: "2026-11-03T14:05:00+01:00"
  updated_at: "2026-11-05T17:25:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 2
      reason: "Answered by HO-pig-web-m0-4-answers and archived"
      changes: ["Every M1–M6 row answered with probe evidence", "Superseded by .project/handoff/pig-web-m0-4-answers.md"]
---

# Goal

> **Archived 05.11.2026.** Answered in [HO-pig-web-m0-4-answers](pig-web-m0-4-answers.md). Kept for context only; do not act on this file.

Settle these pig-web seams inside Workbench's M0.4 contract freeze, so pig-web can correct its specs once against the frozen text and start implementing. Reply as before: a handoff into `host:/mnt/Projects/furbyhaxx/pig-web/.project/handoff/` naming the freeze commit.

# Current state

- pig-workbench `main` at `3c5cbb7`, PLAN-0001 rev 20 (M0.4 *now*). Untracked `.pi/` left untouched.
- pig-web `dev`: SPEC-0001 rev 6, SPEC-0002 rev 2 (knowledge lifecycle and assist), SKETCH-0001 rev 7, PLAN-0001 rev 7. pig-web's delta review: `host:/mnt/Projects/furbyhaxx/pig-web/.project/analyses/2026-11-03-workbench-delta-pre-freeze/README.md` (C1–C11). pig-web will not edit its specs until the freeze lands.

# M1: pig-web's own inference and SPEC-0013

pig-web calls models itself: `web_answer` synthesis, `web_kb ask`, research child sessions (`ctx.CreateAgentSession` with `NoExtensions:true`), and later tagging, catch-up and a hint judge (pig-web SPEC-0002). Inputs are fetched pages and KB content, which can contain credentials (private wikis, credentialed fetches). SPEC-0013 §4.5 puts extension inference outside the boundary.

Options:
- A. A bus method `secrets.redact {text} → {text}` (local process only; the reply carries placeholders, the request carries plaintext to Workbench's listener). Same-process bus listeners see the request, so it is limited to trusted owners or delivered through a private channel.
- B. Fork hook H-S1 covers extension-initiated model streams; then Workbench redacts pig-web's requests like agent turns. pig-web's child sessions would also need Workbench loaded (`NoExtensions:false` with only Workbench), or the same hook.
- C. pig-web redacts itself with the same detection rules: Workbench publishes the detector as a Go package pig-web may import, plus the shared key for identical placeholders.
- D. Out of scope; pig-web documents that its inference sends fetched content as is.

pig-web's preference: **B** where the fork is present, **A** otherwise, with B's child-session question answered explicitly. Whatever is chosen, research child sessions need a stated answer: do they get the boundary?

# M2: knowledge cutoff in `session.model`

pig-web derives the model's knowledge cutoff from models.dev (`knowledge`, else `release_date` − 12 months, marked estimated) and tells the model once (pig-web SPEC-0002 §6). Workbench's `session.model` frame (SPEC-0014 §5) already says which model runs, and `session.date` says today's date.

Ask: let a registered owner contribute fields to a Workbench context frame source (here: `session.model` gains `knowledge_cutoff: {date, basis: explicit|estimated, source}`), or add a contributed context-frame source kind with the same refresh policies (`on-change`) and `beforePrompt` delivery. Otherwise pig-web sends its own `beforePrompt` frame without a date line.

pig-web's preference: contributed fields on `session.model`; one line, refreshed on model change and after compaction.

# M3: one-line summary in resource descriptors

SPEC-0014 §3.4 renders "registered schemes (name, one-line summary)" into the prompt. Please freeze where that summary comes from in a `resourceProvider` descriptor (`summary`, ≤ 120 characters?) so pig-web can supply it for `kb://`, `research://`, `git://`.

# M4: `!command` resolution for extension providers

Your S6 says `GetProviderAuth` expands `!command` keys for an extension-registered key-only provider. Your probe registered it with `RegisterProvider(ProviderConfig)`. pig-web's probe (`host:/mnt/Projects/furbyhaxx/pig-web/.project/analyses/2026-11-02-auth-json-probe/README.md`, row 9) registered with `RegisterNativeProvider` and a custom `Resolve`, which received the raw `!printf …` string, and `GetApiKeyForProvider` returned it unexpanded. Please qualify S6 in the frozen SPEC-0013 by registration kind. pig-web will probe `RegisterProvider` vs `RegisterNativeProvider` + `sdk.EnvAPIKeyAuth` (its A8) and report back; if the native path does not expand, Workbench's inventory would see raw command strings for such providers.

# M5: tool-result annotations and entry-time redaction

pig-web may append one trailing text block to top-level tool results (knowledge hints, pig-web SPEC-0002 §5.3; never on codemode children, never modifying existing blocks or the owner's Details). Please freeze that SPEC-0013's `tool_result` patch and SPEC-0005's snapshot check both tolerate an extra trailing block added by another extension, and in which order the handlers run relative to Workbench's redaction (pig-web's block should also be redacted, so Workbench's patch should run after, or redact all blocks regardless of order).

# M6: activity items

pig-web registers a `statusSegment` with `placement:"activity"`, `glyph`, `open:"/web research"` for running research, and a `placement:"status"` segment for cooldown/offline. Please confirm the frozen field set and whether `open` may be a resource URI (`research://<id>`) as well as a command.

# Traps

- pig-web's KB is central per user; private items are project-bound through `project.id` in provider requests (your SPEC-0004 §4). Nothing in the freeze should reintroduce per-project scoping for pig-web.
- pig-web specs are drafts; re-read them at the commit you answer against.
