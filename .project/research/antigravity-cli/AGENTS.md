# AGENTS.md — antigravity-cli dossier

Reverse engineering of Google's Antigravity CLI (`agy`), recovered from the shipped binary.

| Dir | Origin | Edit? |
|---|---|---|
| `1.4.8/` | imported from pi-auth-plus `@64be593` | No — re-copy per `../AGENTS.md`; path repairs only |
| `1.4.12/` | written here | Yes |

- `1.4.8/` stays authoritative for network, auth, quota and the turn loop. `1.4.12/` covers harness internals only and cites `1.4.8/06-mechanics.md` rather than restating it.
- Don't treat 1.4.12 as newer truth for OAuth, quota or the model catalogue — nobody re-derived them for 1.4.12. `1.4.12/09-delta-from-1.4.8.md` records what actually changed.
- `1.4.8/08-integration-notes.md` is written for **pi-auth-plus**, not this repo. Its "contract" section is that repository's decision.

## Verify

Binary `~/.local/bin/agy`. Symbol cache per version: `tools/funcs.json` (~26 MB, git-ignored).

```bash
cd 1.4.12
python3 tools/goresym.py --bin ~/.local/bin/agy init      # rebuild cache, ~25 min; skip if present
python3 evidence/verify.py "$HOME/.local/bin/agy"          # prompt/tool/RPC byte anchors
python3 evidence/context-recovery/verify.py                # links, templates, schemas, redaction
```

## Traps

- `goresym.py grep` scans rodata: a hit is a literal, not proof it is used on that path. Pair with `strs '<func>'` or `disas '<func>'`.
- Offsets differ per version. Never reuse an offset, hit count or file/function total across 1.4.8 and 1.4.12.
- BOLT reordering makes objdump alignment at a function entry untrustworthy. Use the toolkit's alignment-independent scans.
- Several identity prompts exist (consumer, enterprise, agent, worker). Establish the selector before quoting one as "the" system prompt.

## Cross-updates

- New version → new `<version>/` beside the old, add a row to `README.md`, leave old evidence untouched.
- Prompt or tool finding → check whether `1.4.12/09-delta-from-1.4.8.md` needs the delta line.

## Freshness

Prompts, tool names and mode enums are days-to-weeks fresh; architecture and the quota model age in months. Re-extract before quoting a prompt for a version you have not pinned.
