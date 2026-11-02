---
document:
  type: "analysis"
  id: "ANA-2026-11-02-backend-evaluation"
  title: "M0.3 reader, syntax and job backend evaluation"
  description: "Measured candidates for syntax, PDF, SVG, images, video, archives, binaries, PTY, file watching, SQLite and jq, with a recommended dependency matrix pending approval"
  status: "draft"
  revision: 1
  created_at: "2026-11-02T17:50:00+01:00"
  updated_at: "2026-11-02T17:50:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["Recorded backend measurements and recommendations for D1, D2 and D7"]
---

# 1. Scope and method

Answers PLAN-0001 decisions D1 (syntax), D2 (reader backends) and D7 (job input). Each candidate was built with `CGO_ENABLED=0` (PiG Go extensions are packed pure Go) and run against real files on this host. Programs: `evidence/programs/`; raw output: `evidence/results/`; SVG renders: `evidence/renders/`. **Nothing here is approved; adding any dependency needs user sign-off.**

Host: Linux x86_64, Go toolchain from `PATH`, ffmpeg/ffprobe installed, no libheif.

# 2. Recommendation matrix

| Area | Recommended | Version / license | Verdict | Key evidence |
|---|---|---|---|---|
| Syntax (D1) | `github.com/odvcencio/gotreesitter` + its grammars | v0.56.0, MIT | Adopt | Node-identical to C tree-sitter 0.26 on 21 of 24 corpora (§3) |
| PDF text + page render | `github.com/klippa-app/go-pdfium` (WebAssembly mode via wazero) | v1.21.4, MIT; PDFium BSD/Apache | Adopt, lazy-init | 3 real PDFs: text + render OK; corrupt file → typed error; init 1.8 s, ~290 MB heap |
| SVG raster | `resvg` 0.49 compiled to WASI, run under `wazero` | MPL-2.0 (resvg) / Apache-2.0 (wazero) | Adopt (embedded .wasm, 2.5 MB) | Correct text rendering; hostile SVG neutralised (§4) |
| SVG raster alt. | `srwiley/oksvg` | BSD-3 | Reject | Drops `text`/`tspan`; diagram renders blank labels |
| WebP | `golang.org/x/image/webp` | BSD-3 | Adopt | Decodes in 1 ms |
| HEIC/AVIF-family | `github.com/gen2brain/heic` | v0.7.2, MIT (embeds libheif/libde265 WASM, LGPL) | Optional, needs license decision | Decodes 1346² in ~130–250 ms; uses system libheif via purego when present |
| Video | External `ffprobe`/`ffmpeg` | host binaries | Use if present, typed `backendUnavailable` otherwise | Timestamp seek lands on the next frame (12.5 s → 12.52 s at 25 fps); frame-index select exact (412 → 16.48 s); seek past end exits 0 with **no output file** — must check output |
| Archives | `github.com/mholt/archives` + `kdomanski/iso9660` + stdlib `archive/ar` reader | v0.1.5 MIT; v0.4.0 BSD | Adopt with Workbench-side guards | zip/tar.{gz,xz,zst}/7z OK; ISO/cpio/ar not identified by archives (§5) |
| Binaries | stdlib `debug/elf|pe|macho` + `golang.org/x/arch` disassemblers | BSD-3 | Adopt for headers/symbols/disasm | Disassembly matches objdump instruction-for-instruction (displacements printed unsigned, e.g. `0xfffffc90` vs `-0x370`) |
| Pseudocode | External Ghidra headless | Apache-2.0, needs JDK ≥ 21 | External, opt-in only | Produces correct C for test function; minutes of startup, GB-scale install |
| Pseudocode alt. | rizin 0.9.1 | LGPL | Reject | No decompiler without plugins (`pdg` missing) |
| PTY (D7) | `github.com/aymanbagabas/go-pty` | v0.2.3, MIT | Adopt | SIGINT via `^C`, EOF, SIGWINCH size, line input all behave as a terminal (§6) |
| File watching | `github.com/fsnotify/fsnotify` | v1.10.1, BSD-3 | Adopt, directory-level watches | Atomic rename seen as CREATE; nested dirs not recursive (§6) |
| SQLite | `github.com/ncruces/go-sqlite3` (WASM, no cgo) | v0.36.0, MIT | Adopt | Read-only authorizer blocks writes/ATTACH/PRAGMA; multi-statement tail must be rejected by caller (§7) |
| jq | `github.com/itchyny/gojq` | v0.12.20, MIT | Adopt | Big-int precision kept; `input`/`$__loc__` disabled; `env` empty when env func not supplied (§7) |
| HTML → Markdown | `JohannesKaufmann/html-to-markdown/v2` + `codeberg.org/readeck/go-readability/v2` | v2.5.2 MIT; v2.1.3 MIT | Adopt | Build/license checked only; quality not yet measured |
| Profiles | `github.com/google/pprof/profile` | Apache-2.0 | Adopt | Build/license checked only |

# 3. Syntax (D1)

Compared parse trees from gotreesitter against C tree-sitter 0.26.0 with `tree-sitter-language-pack` 1.23.1 over real files (`samples.txt`, `s2.txt`). Metrics: fraction of identical named nodes, agreement on top-level "anchor" nodes (what hashline block selectors use), ERROR node counts.

- Identical (`same=1.000`, `agree=1.000`, 0 errors): go, rust, python, typescript, tsx, javascript, c, bash, css, html, json, lua, markdown, toml, yaml, elisp, ruby, swift, fortran, java, regex.
- **cpp**: 0.993 nodes, 0.916 anchors, 2 spurious ERROR nodes (namespace / struct specifier mis-parse).
- **diff**: 0.970 nodes, 0.682 anchors (hunk boundaries differ) — use a dedicated unified-diff parser for diffs, not tree-sitter.
- Performance: parse 4–118 ms on 3–86 KB files, grammar load ≤ 29 ms.
- Size: the full grammar set adds ~17.6 MB to a stripped binary (28.2 MB with everything in §2 vs 10.5 MB without grammars). Recommend registering a curated grammar subset or lazy-loading grammar blobs.

Block selectors on C++ must fall back to "ambiguous, ask for line range" when the anchor tree contains ERROR nodes; never guess a block boundary.

# 4. SVG safety

`hostile.svg` (100000×100000 canvas, `<script>`, `<image href="file:///etc/passwd">`) rendered to a bounded 800 px PNG with no file read: resvg has no script engine, `resources_dir = None` disables relative/`file:` loads, and the wazero module only mounts `/usr/share/fonts` read-only. Memory is capped at 4096 pages (256 MiB) and the module dies with its context. Keep those three controls as the contract.

# 5. Archives

- `mholt/archives` identified and listed zip, tar.gz, tar.xz, tar.zst and 7z; reported symlinks with targets (`./link -> ../../etc/passwd`).
- It does **not** identify ISO, cpio or ar. ISO via `kdomanski/iso9660` works; ar via a small stdlib-only reader works; cpio header parse failed on the tested odc (pre-SVR4) sample — needs another library or a ~100-line reader (open item).
- Zip slip member `../../evil.txt` is reported as-is; zip bomb (52 MB expansion) is only stopped by our own cap. Workbench must: normalise member names and reject escapes, never follow member symlinks, enforce per-member and total expansion caps, and stream instead of extracting.

# 6. Jobs: PTY and watching (D7)

PTY (`results/pty.txt`): `sleep 30` + `\x03` → signaled SIGINT; `cat` + `\x04` → exit 0 with echoed input; resize → child's `stty size` reports `40 120` after SIGWINCH; line read works. This confirms D7: control keys are only signals/EOF on a PTY. Pipe-mode jobs must expose `close stdin` and `signal` as distinct operations.

fsnotify (`results/watch.txt`): in-place write → WRITE; temp+rename (most editors, `git checkout`) → CREATE only; vim backup-rename → RENAME, CREATE, WRITE; delete → REMOVE; nested directory changes produce nothing unless that directory is watched. Instruction watchers must watch candidate directories (not files), treat CREATE as change, and re-add watches for new directories. `max_user_watches` here is 1048576; still cap watches per session.

# 7. Query backends

SQLite with a read-only authorizer rejected `DELETE`, `ATTACH`, `PRAGMA writable_schema` and `load_extension` (function absent). `SELECT 1; DELETE FROM users` executed the first statement and returned the remainder as `tail` — Workbench must reject any non-empty tail. 2^53+3 integers round-trip exactly.

gojq kept `12345678901234567890` and `1.10` verbatim, `1e400` without overflow. `input` and `$__loc__` are rejected at compile time when not enabled; `env` returned null because no environment function was supplied — keep it that way so queries cannot read secrets from the environment.

# 8. Open items before approval

1. Approve/decline each dependency in §2 (user decision).
2. HEIC: decide whether embedding LGPL WASM (libheif/libde265) is acceptable, or require system libheif.
3. cpio backend.
4. Grammar subset and binary size budget.
5. Measure HTML→Markdown and readability quality on real pages.
6. Repeat the matrix on macOS (and Windows, if supported); only Linux was tested.
