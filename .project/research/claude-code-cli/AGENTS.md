# AGENTS.md — claude-code-cli dossier

Reverse engineering of the shipped `claude` binary: Bun standalone compile, JavaScript carved from `.bun`.

| Dir | Origin | Edit? |
|---|---|---|
| `2.1.310/` | imported from pi-auth-plus `@64be593` | No — re-copy per `../AGENTS.md`; path repairs only |
| `2.1.317/` | written here | Yes |

- `2.1.310/` owns wire contracts: OAuth, tokens, quota headers and resets, model metadata, endpoint inventory. `2.1.317/` owns harness internals. Not interchangeable.
- Don't cite `2.1.310/` findings as current for 2.1.317 — they were not re-verified. `2.1.317/09-delta-from-2.1.310.md` states what was and was not checked.
- **The strongest evidence here is a capture, not an inference.** `2.1.317/evidence/wire/*.json` are complete assembled requests taken from the real binary through a loopback mock. The static chapters explain selectors and conditional families; the captures show what is emitted. Where they disagree for that configuration, the capture wins.
- The live `-p` tool surface is 12 tools; the bundle contains 22 candidate names. Don't report a bundle name as an emitted tool.

## Capture vs carve

```bash
python3 2.1.317/evidence/capture-local.py print         # or: interactive
```

Hermetic: loopback mock answering 400, throwaway `HOME`/XDG, dummy key, `NO_PROXY=*`, telemetry and autoupdater off. No egress, no cost. It refuses to overwrite — delete `evidence/wire/<mode>-request.json` to re-run.

Corpus, when you need literals rather than a request:

```bash
readelf -SW ~/.local/share/claude/versions/<V> | grep -E '\.bun\b'   # re-measure per release
```

`2.1.317/evidence/verify.py` reads `/tmp/cc_code.txt`: build the corpus first.

## Traps

- The `.bun` file offset changes every release. Read it from `readelf`; never carry it over.
- Minified identifiers are module-local and collide. Establish the module context before attributing a code path.
- The corpus helper prints an in-line offset and no line number. A `C<line>:<offset>` citation needs `grep -n` plus `2.1.317/tools/ctx.py`; protocol in `2.1.317/tools/CORPUS.md`.
- A mock or scenario class is not production behaviour. Bundled-gateway code writing compatible headers is not Anthropic's policy.
- The 2.1.310 binary is no longer installed (only 2.1.311/312/316/317), so an exact string-set delta against 2.1.310 needs that version re-downloaded.

## Cross-updates

- New version → new `<version>/`, add a row to `README.md`, keep the old dir.
- New capture → update the `tools`/system-block counts quoted in `README.md` and `10`/`11` if they moved.

## Freshness

A captured prompt or tool array is valid for its version alone. Recapture per release before quoting it.
