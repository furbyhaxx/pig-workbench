---
document:
  type: "analysis"
  id: "ANA-2026-11-02-backend-evaluation"
  title: "M0.3 reader, syntax and job backend evaluation"
  description: "Measured candidates for syntax, PDF, SVG, images, video, archives, binaries, PTY, file watching, SQLite and jq, with an approved dependency matrix and coverage checks"
  status: "published"
  revision: 3
  created_at: "2026-11-02T17:50:00+01:00"
  updated_at: "2026-11-02T20:04:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 3
      reason: "Coverage checks: every required archive format, grammar size, HTML quality, cross-platform builds"
      changes: ["Added §5.1 archive coverage over 200+ libarchive/commons-compress fixtures and new archive backends", "Replaced the grammar-subset recommendation with measured size data", "Added §8 HTML quality and §9 platform coverage", "Listed the new dependencies that need approval"]
---

# 1. Scope and method

Answers PLAN-0001 decisions D1 (syntax), D2 (reader backends) and D7 (job input). Each candidate was built with `CGO_ENABLED=0` (PiG Go extensions are packed pure Go) and run against real files on this host. Programs: `evidence/programs/`; raw output: `evidence/results/`; SVG renders: `evidence/renders/`. **Approved by the user on 2026-11-02 as proposed, including HEIC via `gen2brain/heic` (embedded libheif/libde265 WASM, LGPL; system libheif used when present).** Dependencies are added when the adapter that needs them is implemented, not up front.

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
| Archives | `github.com/mholt/archives` (zip family, tar, 7z, RAR incl. multi-volume, gz/bz2/xz/zst/lz4/br/lz/sz) | v0.1.5 MIT | Adopt with Workbench-side guards | §5, §5.1 |
| Archives, added by coverage check | `go-filesystems/iso9660`, `go-filesystems/cpio`, `NextronSystems/go-cab`, `sassoftware/go-rpmutils`, `depthbomb/go-asar`, own ~100-line ar reader, external 7-Zip fallback | BSD-3, BSD-3, MIT, Apache-2.0, MIT, —, LGPL (external) | **Needs approval** (§5.1) | Replaces `kdomanski/iso9660`, which crashed the process on a crafted ISO |
| Binaries | stdlib `debug/elf|pe|macho` + `golang.org/x/arch` disassemblers | BSD-3 | Adopt for headers/symbols/disasm | Disassembly matches objdump instruction-for-instruction (displacements printed unsigned, e.g. `0xfffffc90` vs `-0x370`) |
| Pseudocode | External Ghidra headless | Apache-2.0, needs JDK ≥ 21 | External, opt-in only | Produces correct C for test function; minutes of startup, GB-scale install |
| Pseudocode alt. | rizin 0.9.1 | LGPL | Reject | No decompiler without plugins (`pdg` missing) |
| PTY (D7) | `github.com/aymanbagabas/go-pty` | v0.2.3, MIT | Adopt | SIGINT via `^C`, EOF, SIGWINCH size, line input all behave as a terminal (§6) |
| File watching | `github.com/fsnotify/fsnotify` | v1.10.1, BSD-3 | Adopt, directory-level watches | Atomic rename seen as CREATE; nested dirs not recursive (§6) |
| SQLite | `github.com/ncruces/go-sqlite3` (WASM, no cgo) | v0.36.0, MIT | Adopt | Read-only authorizer blocks writes/ATTACH/PRAGMA; multi-statement tail must be rejected by caller (§7) |
| jq | `github.com/itchyny/gojq` | v0.12.20, MIT | Adopt | Big-int precision kept; `input`/`$__loc__` disabled; `env` empty when env func not supplied (§7) |
| HTML → Markdown | `JohannesKaufmann/html-to-markdown/v2` + `codeberg.org/readeck/go-readability/v2` | v2.5.2 MIT; v2.1.3 MIT | Adopt; readability only as fallback | 11 real pages measured (§8) |
| Profiles | `github.com/google/pprof/profile` | Apache-2.0 | Adopt | Build/license checked only |

# 3. Syntax (D1)

Compared parse trees from gotreesitter against C tree-sitter 0.26.0 with `tree-sitter-language-pack` 1.23.1 over real files (`samples.txt`, `s2.txt`). Metrics: fraction of identical named nodes, agreement on top-level "anchor" nodes (what hashline block selectors use), ERROR node counts.

- Identical (`same=1.000`, `agree=1.000`, 0 errors): go, rust, python, typescript, tsx, javascript, c, bash, css, html, json, lua, markdown, toml, yaml, elisp, ruby, swift, fortran, java, regex.
- **cpp**: 0.993 nodes, 0.916 anchors, 2 spurious ERROR nodes (namespace / struct specifier mis-parse).
- **diff**: 0.970 nodes, 0.682 anchors (hunk boundaries differ) — use a dedicated unified-diff parser for diffs, not tree-sitter.
- Performance: parse 4–118 ms on 3–86 KB files, grammar load ≤ 29 ms.
- Size (`results/grammar-size.txt`, stripped linux/amd64): runtime alone 2.5 MB; one grammar 10.5 MB; 8 grammars 11.2 MB; 45 curated grammars 16.6 MB; all 206 via the `grammars` registry 19.0 MB. The first grammar pulls in ~8 MB of parser engine; each further grammar costs ~0.14 MB. **Ship the full registry**: curating 45 saves only 2.4 MB and loses extension detection for the rest. PiG compiles extensions itself without build tags, so the library's `grammar_subset` tags are not usable anyway. Budget: tree-sitter ≤ 18 MiB of the Workbench binary (measured 17.8 MiB over an empty Go binary).

Block selectors on C++ must fall back to "ambiguous, ask for line range" when the anchor tree contains ERROR nodes; never guess a block boundary.

# 4. SVG safety

`hostile.svg` (100000×100000 canvas, `<script>`, `<image href="file:///etc/passwd">`) rendered to a bounded 800 px PNG with no file read: resvg has no script engine, `resources_dir = None` disables relative/`file:` loads, and the wazero module only mounts `/usr/share/fonts` read-only. Memory is capped at 4096 pages (256 MiB) and the module dies with its context. Keep those three controls as the contract.

# 5. Archives

- `mholt/archives` identified and listed zip, tar.gz, tar.xz, tar.zst and 7z; reported symlinks with targets (`./link -> ../../etc/passwd`).
- It does **not** identify ISO, cpio or ar. ISO via `kdomanski/iso9660` works; ar via a small stdlib-only reader works; cpio header parse failed on the tested odc (pre-SVR4) sample — needs another library or a ~100-line reader (open item).
- Zip slip member `../../evil.txt` is reported as-is; zip bomb (52 MB expansion) is only stopped by our own cap. Workbench must: normalise member names and reject escapes, never follow member symlinks, enforce per-member and total expansion caps, and stream instead of extracting.

## 5.1 Archive coverage (`results/archives-coverage.txt`)

Every (fixture, backend) pair ran in its own process (`programs/archcov/drive.sh`), so a crash or hang is attributed to that pair. Fixtures: 198 libarchive test archives (7z, RAR4/5, ZIP), 69 libarchive cab/lzh/cpio/rpm/iso/ar fixtures, commons-compress ARJ, a Fedora 43 RPM, and locally built cpio (newc/odc/crc/bin), deb, asar and compressed streams. 7-Zip 26.05 (`7zz`) ran as the reference. Results below count only archives the reference reads; malformed and encrypted fixtures fail in every backend as they should.

| Format | Recommended | Result | Gaps |
|---|---|---|---|
| ZIP family, tar | `mholt/archives` | Matches 7-Zip except one LZMA zip whose symlink target it cannot read | — |
| 7z | `mholt/archives` | Copy/LZMA/LZMA2/Deflate/BZip2/zstd/BCJ/BCJ2/delta OK; zstd 7z read where 7-Zip itself fails | PPMd, ARM64/ARM-Thumb/IA64/RISC-V filters, archive properties → 7-Zip fallback |
| RAR 4/5 | `mholt/archives` with `Rar{Name, FS}` | Multi-volume works only when opened by name with an FS; streaming the first volume fails | 2 RAR4 LZSS/PPMd edge cases → 7-Zip fallback |
| ISO 9660 | `go-filesystems/iso9660` | 24 of 25 incl. Joliet, Rock Ridge, zisofs | `kdomanski/iso9660` overflowed the stack on `rockridge_ce_loop.iso` (kills the process; `recover` cannot catch it) and panicked on 4 more — **rejected** |
| cpio | `go-filesystems/cpio` | newc, crc, odc, binary LE/BE, non-UTF-8 names | afio large headers; `cavaliergopher` and `u-root` read newc only |
| CAB | `NextronSystems/go-cab` | None, MSZIP, LZX | Multi-cabinet sets, some LZX streams, Quantum → 7-Zip fallback; the other two CAB libraries lack LZX |
| RPM | `sassoftware/go-rpmutils` | gzip/bzip2 and zstd (Fedora 43) payloads | — |
| deb, ar | Own reader (~100 lines, `programs/arproto`) + tar | GNU `//` long names, BSD `#1/`, `.deb`; glibc `libc.a` 2,227 members = 7-Zip | Both ar libraries failed on GNU long names (one panicked) |
| ASAR | `depthbomb/go-asar` | Reads `@electron/asar` output | 7-Zip cannot read ASAR |
| LZH/LHA | External 7-Zip only | `koron-go/lha` failed 13 of 14 fixtures | No usable Go reader |
| ARJ | External 7-Zip only | — | No Go reader exists |
| `.Z` | External 7-Zip only | — | Not in `mholt/archives` |

**External 7-Zip fallback.** Like ffmpeg: used when `7zz`/`7z` is on `PATH` and the Go backend reports an unsupported method or format, otherwise a typed `backendUnavailable` naming the missing tool. Listing via `7zz l -slt`, member reads via `7zz e -so`. Its RSS stayed ≤ 12 MB on all fixtures.

**Maintenance risk.** The `go-filesystems` org was created in October 2026 and has no stars yet; its readers were the most complete in this test. Pin exact versions; each package is small enough to vendor if it goes stale.

# 6. Jobs: PTY and watching (D7)

PTY (`results/pty.txt`): `sleep 30` + `\x03` → signaled SIGINT; `cat` + `\x04` → exit 0 with echoed input; resize → child's `stty size` reports `40 120` after SIGWINCH; line read works. This confirms D7: control keys are only signals/EOF on a PTY. Pipe-mode jobs must expose `close stdin` and `signal` as distinct operations.

fsnotify (`results/watch.txt`): in-place write → WRITE; temp+rename (most editors, `git checkout`) → CREATE only; vim backup-rename → RENAME, CREATE, WRITE; delete → REMOVE; nested directory changes produce nothing unless that directory is watched. Instruction watchers must watch candidate directories (not files), treat CREATE as change, and re-add watches for new directories. `max_user_watches` here is 1048576; still cap watches per session.

# 7. Query backends

SQLite with a read-only authorizer rejected `DELETE`, `ATTACH`, `PRAGMA writable_schema` and `load_extension` (function absent). `SELECT 1; DELETE FROM users` executed the first statement and returned the remainder as `tail` — Workbench must reject any non-empty tail. 2^53+3 integers round-trip exactly.

gojq kept `12345678901234567890` and `1.10` verbatim, `1e400` without overflow. `input` and `$__loc__` are rejected at compile time when not enabled; `env` returned null because no environment function was supplied — keep it that way so queries cannot read secrets from the environment.

# 8. HTML → Markdown quality (`results/html-quality.txt`)

Eleven real pages: Wikipedia, Go blog, MDN, GitHub README, Python docs, pkg.go.dev, Rust blog, Arch wiki, LWN, the Hacker News front page and a JavaScript-only redirect page.

| Strategy | Result |
|---|---|
| Whole page → Markdown | Keeps all navigation: MDN 68 KB with 566 links for a 10 KB article |
| Readability → Markdown | Drops structure: GitHub README 40→0 headings, pkg.go.dev 32→0, Wikipedia 13→2; strips `language-*` classes, so every fenced block loses its language (Go blog 22→0) |
| **Semantic main element first**, readability as fallback | `.markdown-body`, `<article>`, `<main>`, `[role=main]`, `#content` matched 8 of 11 pages; headings kept (GitHub 15, pkg.go.dev 26, Wikipedia 12) and code languages kept (Go blog 22/22). 1–9 ms per page |

Required Workbench rules on top of the library, all prototyped in `programs/htmlq`:
- Resolve `href`/`src` against the page URL before conversion. The library's `WithDomain` turns `#section` into `https://host#section`.
- Strip `nav`, `footer`, `aside`, `form`, `script`, `style`, hidden elements, `role=navigation`, and permalink anchors (`¶`, `#`, `headerlink`).
- Mark tables without `<th>` as presentation tables so layout tables (HN) render as text, not 100-row Markdown tables. Disable cell padding.
- A page whose only content is a script redirect (the Rust blog's old URL) is reported as "JavaScript redirect to <target>", never executed. Follow `<meta http-equiv=refresh>` within the redirect limit.

Known residue: Wikipedia infobox cells run together ("Release1977") and icon images remain in pkg.go.dev headings. Good enough for model reading; `:raw` returns the original HTML.

# 9. Platforms

No macOS or Windows host or emulator was available, so this is compile-level evidence plus source review, not runtime evidence.

- Every evaluation program, including the WASM-based PDF and SVG paths, the archive libraries and the HTML pipeline, builds with `CGO_ENABLED=0` for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and windows/amd64.
- **macOS:** fsnotify uses kqueue, which opens one descriptor per watched file *and* per file inside a watched directory. The per-session watch cap must count files, not directories, and stay well under `kern.maxfilesperproc`.
- **Windows:** go-pty uses ConPTY (Windows 10 1809+). There is no SIGWINCH; resize goes through `Resize`. Ctrl-C to a pipe job needs a console control event or a job-object kill, not a POSIX signal. fsnotify uses `ReadDirectoryChangesW`, which can drop events on buffer overflow, so reconcile by stat (already required by SPEC-0004). HEIC uses system libheif through purego on Windows/macOS/Linux when present, otherwise the embedded WASM.
- External tools: Ghidra, ffmpeg and 7-Zip have different names and install paths per platform (`7z.exe` vs `7zz`); resolve them on `PATH` and report what was found.

Runtime verification on macOS and Windows remains open. It needs a machine or CI runner, which this repository deliberately does not have.

# 10. Open items

1. ~~Approve/decline each dependency in §2~~ — approved.
2. ~~HEIC licensing~~ — accepted; ship the LGPL notice with the binary.
3. ~~cpio backend~~ — `go-filesystems/cpio` (§5.1).
4. ~~Grammar subset and size budget~~ — full registry, ≤ 18 MiB (§3).
5. ~~HTML quality~~ — main-element-first (§8).
6. Approve the archive additions in §5.1: `go-filesystems/iso9660`, `go-filesystems/cpio`, `NextronSystems/go-cab`, `sassoftware/go-rpmutils`, `depthbomb/go-asar`, and the external 7-Zip fallback.
7. Runtime runs on macOS and Windows (§9); compile coverage is done.
