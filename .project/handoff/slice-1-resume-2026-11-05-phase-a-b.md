---
document:
  type: "handoff"
  id: "HO-slice-1-resume-2026-11-05-phase-a-b"
  title: "Slice 1 resume: Phase A probes/ledger, Phase B complete, Phase D dependency truth"
  description: "What landed after the Phase C close, what the task list now says honestly, and the next ordered work with its real dependencies"
  status: "open"
  revision: 1
  created_at: "2026-11-05T17:50:00+01:00"
  updated_at: "2026-11-05T17:50:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Session end; record committed work, list changes and the one blocked approval"
      changes: ["Recorded commits 777f56f..89838b3", "Recorded the D-depends-on-H/J/L1 correction", "Named the single user decision outstanding"]
---

# State

`dev` at `89838b3`. Gates green at that commit: `just check`, `just race`, `just integration`.

# What landed this session

| Commit | Change |
|---|---|
| `777f56f` | Slice 1 inventory: every PLAN-0001 §9 row classified done/partial/missing with evidence (ANA-2026-11-05-slice-1-inventory). |
| `8f88850` | Phase A probes with a runnable fixture (`internal/host/testdata/probe-agent/`, `live_probes_test.go`): idle wake, busy steer, idle state, titles T1–T3, resumed cwd, credential forms, annotation order (ANA-2026-11-05-slice-1-probes). |
| `3e8e083` | PLAN-0002 A0 ledger rewritten with status/evidence/owner; SPEC-0011 §10 and SPEC-0012 §3.5/§6 updated with the observed answers. |
| `5b4ebe9` | Authoring API frozen: `api/contract_test.go` compiles a producer flow and pins every payload field name; SPEC-0001 rev 7 adds the implemented-surface table and migration notes. |
| `a92d0eb` | pig-web M1–M6 answered with evidence; inbound handoff archived. |
| `c6dc366` | `Secrets.toml` is a real appending detection-policy layer with origin enforcement, trust gating and diagnostics. |
| `dc2d049` | `internal/pathdisplay` implements SPEC-0003 §10 with all seven goldens and 36/57/80/140 width guards; the presentation header renders through it. |
| `89838b3` | Contributed scheme guides are injected on first touch, once per session and descriptor revision. |

# Task-list correction

Phase D's core-scheme row cannot finish before H, J and L1: `job://` needs 10.2, `skill://` needs
12.2, `agent://` needs 14.2. Row 5.3.1 is now blocked on 10.2 with the full set named in its detail,
so the list no longer promises an order the code cannot follow. Expect the same shape for 5.1.1
(codemode parity) which is closer to Phase M.

# Outstanding user decision

The freeze (`2.4`): approve SPEC-0001/0002/0004/0005/0009/0010/0012/0013/0014 plus the ≤120-character
`resourceProvider.summary` field. Everything the freeze needed is prepared and committed; only the
approval is missing, and no spec may carry `status: approved` without it.

# Next ordered work

1. **10.1** jobs supervision rows (bounded output, descendant cancellation, reload cleanup, shared policy).
2. **10.2** `job://` resources, `/workbench jobs`, `/wb:jobs`, G8.20 dispatch — which unblocks 5.3.1.
3. **5.3.1** remaining core schemes and the SPEC-0004 §10 / SPEC-0006 §8 rows.
4. Then 6 (E1 project request field), 8 (F replay/compaction), 9 (G snapshots service), 11 (I secrets boundary), 12 (J prompt takeover and skills), 13 (K instructions), 14 (L1 messaging), then E2, L2, M, 17.

# Known gaps that are recorded, not hidden

- `RegisterNativeProvider` custom-resolver credentials are unobservable from an extension (SPEC-0013 §2).
- `snapshots`, `sessions` and `secrets` services are unadvertised, so their absence fails explicitly.
- Presentation `Target` carries no resolution root, so rules 4–5 cannot shorten against the working root yet.
- SPEC-0003 rule 7 (shell-command word display) is not implemented.
