---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-tools-corpus"
  title: "Claude Code 2.1.317 binary — extraction corpus"
  description: "How to carve the shipped claude 2.1.317 Bun bundle and read the resulting corpus, with the offsets and sizes measured for this version"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:00+01:00"
  updated_at: "2026-11-03T11:52:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["extraction protocol adapted from ../2.1.310/tools/CORPUS.md", "target path, .bun offset and corpus sizes measured for 2.1.317"]
---

# Claude Code 2.1.317 binary — extraction corpus

Target: `/home/furbyhaxx/.local/share/claude/versions/2.1.317`
(331,362,672 bytes / 316 MiB ELF, Bun standalone compile, `Bun v1.x` + JavaScriptCore).

## Why this exists

The executable is a Bun standalone build. The `.bun` section
(`file offset 0x8cfd000` = 141 MiB, `size 0xaef787a` = 183,466,106 bytes ≈ 175 MiB)
holds the compiled module graph: a Bun interned **string table** plus the
**minified JavaScript source** of every module, interleaved. There is no source
map, but the JS is plaintext.

Section offsets differ per release and must be re-measured, not remembered:

```bash
readelf -SW ~/.local/share/claude/versions/<version> | grep -E '\.bun\b'
```

## Reproduce

```bash
# 1. carve the .bun section
dd if=/home/furbyhaxx/.local/share/claude/versions/2.1.317 of=/tmp/claude.bun \
   bs=1M skip=$((0x8cfd000/1048576)) count=180

# 2. split printable runs (offsets are rewritten to file offsets)
python3 extract_runs.py     # reads /tmp/claude.bun
                            # -> /tmp/cc_code.txt (runs >= 160 chars)
                            #    /tmp/cc_strings.txt (runs >= 24 chars)
```

`extract_runs.py` has the input path hardcoded to `/tmp/claude.bun`; place the
carved section there. Extraction of this version took a few seconds.

## Corpus files

Measured for 2.1.317:

| File | Size | Contents |
|---|---|---|
| `/tmp/cc_code.txt` | 53,294,577 B (53.3 MB) | printable runs ≥ 160 chars: minified JS function bodies, template literals, string-table clusters. Prefix `@<fileoffset>@` on some runs. |
| `/tmp/cc_strings.txt` | 58,694,939 B (58.7 MB) | printable runs ≥ 24 chars with `<offset>\t` prefix: every interned string — identifiers, endpoints, env var names, headers, model IDs, error copy. |

Both are `grep`-able and search order follows binary layout, so **adjacent hits
usually belong to the same module** — that adjacency is the main tool for
reconstructing which strings belong together.

The corpus is regenerable and is not committed; only this protocol is.

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

Citations in this dossier use `C<line>:<character-offset>` into `/tmp/cc_code.txt`:
1-based line, 0-based character offset. That is not what `ctx.py` prints — it
reports `line-offset <n>`, the offset **within** the matched line, and prints no
line number at all. To produce a `C<line>:<offset>` citation:

```bash
grep -n -F '<literal>' /tmp/cc_code.txt | head   # gives the line number
python3 ctx.py '<literal>' --window 200 --max 1  # gives the in-line offset
```

These are corpus coordinates, not ELF or byte offsets.

## Rules for this investigation

- **Never print, copy, or quote a credential value.** Token *structure*, JWT claim
  names and file layout are in scope; the values are not.
- Distinguish **observed** (a literal in the bundle), **inferred** (what the code
  does with it), and **unknown** (not resolvable statically).
- Quote the exact minified snippet as evidence for any behavioural claim.
- Minified identifiers are module-local and collide across scopes. Establish the
  module context of a snippet before claiming which code path it belongs to.
