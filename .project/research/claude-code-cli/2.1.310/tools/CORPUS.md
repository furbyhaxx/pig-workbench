---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-tools-corpus"
  title: "Corpus"
  description: "Development artifact: Corpus."
  status: "published"
  revision: 2
  created_at: "2026-10-26T12:36:12+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/tools/CORPUS.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/tools/CORPUS.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      reason: "imported into pi-auth-plus"
      changes: ["copied from pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/tools/CORPUS.md"]
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/tools/CORPUS.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---
# Claude Code 2.1.310 binary — extraction corpus

Target: `/home/furbyhaxx/.local/share/claude/versions/2.1.310`
(279 MiB ELF, Bun standalone compile, `Bun v1.x` + JavaScriptCore).

## Why this exists

The executable is a Bun standalone build. The `.bun` section
(`offset 0x6e14000`, size `0xa7fea02` ≈ 176 MB) holds the compiled module graph:
a Bun interned **string table** plus the **minified JavaScript source** of every
module, interleaved. There is no source map, but the JS is plaintext.

## Reproduce

```bash
# 1. carve the .bun section
dd if=/home/furbyhaxx/.local/share/claude/versions/2.1.310 of=/tmp/claude.bun \
   bs=1M skip=$((0x6e14000/1048576)) count=200

# 2. split printable runs (offsets are rewritten to file offsets)
python3 extract_runs.py     # -> /tmp/cc_code.txt (runs >= 160 chars)
                            #    /tmp/cc_strings.txt (runs >= 24 chars)
```

## Corpus files

| File | Size | Contents |
|---|---|---|
| `/tmp/cc_code.txt` | ~52 MB | printable runs ≥ 160 chars: minified JS function bodies, template literals, string-table clusters. Prefix `@<fileoffset>@` on some runs. |
| `/tmp/cc_strings.txt` | ~57 MB | printable runs ≥ 24 chars with `<offset>\t` prefix: every interned string — identifiers, endpoints, env var names, headers, model IDs, error copy. |

Both are `grep`-able and search order follows binary layout, so **adjacent hits
usually belong to the same module** — that adjacency is the main tool for
reconstructing which strings belong together.

## Reading it

One "line" can be megabytes long, so never print a line raw. Use the helper:

```bash
python3 ctx.py '<pattern>' --window 900 --max 4        # both corpora
python3 ctx.py '<pattern>' --files code --window 1500  # code corpus only
python3 ctx.py '<regex.*goes.*here>' --regex           # regex mode
```

For short inventory sweeps, plain `grep -o -E` over the two files is fastest:

```bash
grep -o -E '/api/[a-zA-Z0-9_./-]+' /tmp/cc_strings.txt | sort -u
```

## Rules for this investigation

- **Never print, copy, or quote a credential value.** Token *structure*, JWT claim
  names and file layout are in scope; the values are not.
- Distinguish **observed** (a literal in the bundle), **inferred** (what the code
  does with it), and **unknown** (not resolvable statically).
- Quote the exact minified snippet as evidence for any behavioural claim.
- Record the binary offset when you can, so a claim is re-verifiable.
