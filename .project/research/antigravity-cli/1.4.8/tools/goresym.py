#!/usr/bin/env python3
"""Recover symbols and string literals from a stripped, BOLT-optimized Go binary.

The target ships without .symtab, so function names come from pclntab and code
addresses are derived from the `.text` virtual base. Everything here is read-only.

Usage:
  goresym.py --bin AGY init                      # build cache (funcs.json)
  goresym.py --bin AGY funcs [regex]             # list function names
  goresym.py --bin AGY addr <exact-name>         # address/size of a function
  goresym.py --bin AGY disas <exact-name>        # objdump of a function
  goresym.py --bin AGY strs <exact-name>         # string literals referenced by it
  goresym.py --bin AGY grep <regex>              # substring search over rodata
"""
import argparse
import json
import os
import re
import struct
import subprocess
import sys

PTR = 8
PCHEADER_MAGIC = 0xFFFFFFF1


def find_pclntab(d):
    cands = []
    pat = struct.pack("<I", PCHEADER_MAGIC)
    i = 0
    while True:
        j = d.find(pat, i)
        if j < 0:
            break
        if d[j + 4] == 0 and d[j + 5] == 0 and d[j + 6] in (1, 2, 4) and d[j + 7] in (4, 8):
            nfunc = struct.unpack("<Q", d[j + 8:j + 16])[0]
            if 0 < nfunc < 5_000_000:
                cands.append((j, nfunc))
        i = j + 1
    if not cands:
        sys.exit("no pclntab found")
    # a Go binary's own pclntab is the one covering the most functions
    return max(cands, key=lambda c: c[1])


def text_base(path):
    out = subprocess.run(["readelf", "-S", "-W", path], capture_output=True, text=True).stdout
    for line in out.splitlines():
        f = line.split()
        if len(f) >= 5 and f[1] == ".text":
            return int(f[3], 16)
    sys.exit("no .text section")


def read_name(d, base, off):
    end = d.find(b"\x00", base + off)
    return d[base + off:end].decode("utf-8", "replace")


def build(path):
    d = open(path, "rb").read()
    off, nfunc = find_pclntab(d)
    hdr = struct.unpack("<QQQQQQQQ", d[off + 8:off + 8 + 8 * PTR])
    _nfunc, nfiles, _text_start, funcname_off, _cu, filetab_off, _pctab, pcln_off = hdr
    fnames = off + funcname_off
    pcln = off + pcln_off
    base = text_base(path)

    entries = []
    for i in range(nfunc):
        entry_off, func_off = struct.unpack("<II", d[pcln + i * 8:pcln + i * 8 + 8])
        name_off = struct.unpack("<i", d[pcln + func_off + 4:pcln + func_off + 8])[0]
        entries.append((entry_off, read_name(d, fnames, name_off)))
    entries.sort()

    funcs = []
    for i, (entry_off, name) in enumerate(entries):
        nxt = entries[i + 1][0] if i + 1 < len(entries) else entry_off
        funcs.append({"name": name, "addr": base + entry_off, "span": max(nxt - entry_off, 1)})

    # Go 1.28 packs the file table as one blob; recover it opportunistically.
    files = sorted({m.group(0).decode()
                    for m in re.finditer(rb"[A-Za-z0-9_./+-]+\.go", d[off + filetab_off:off + pcln_off])})
    return {"binary": os.path.abspath(path), "sha256_size": os.path.getsize(path),
            "text_base": base, "nfunc": nfunc, "nfiles": nfiles,
            "funcs": funcs, "files": files}


def load_cache(path, cache):
    if os.path.exists(cache):
        with open(cache) as fh:
            c = json.load(fh)
        if c["sha256_size"] == os.path.getsize(path):
            return c
    c = build(path)
    with open(cache, "w") as fh:
        json.dump(c, fh)
    return c


def rodata_str(d, addr, maxlen=400):
    chunk = d[addr:addr + maxlen]
    out = bytearray()
    for b in chunk:
        if 0x20 <= b < 0x7f:
            out.append(b)
        else:
            break
    return out.decode("utf-8", "replace")


def lea_targets(d, start, end):
    """Every RIP-relative LEA target in [start,end).

    Scan by opcode rather than by decoded instruction boundaries: BOLT reorders
    and splits function bodies, so objdump alignment at an entry point is not
    trustworthy. False positives are filtered by requiring readable text at the
    resolved target.
    """
    out = []
    for i in range(start, end - 7):
        if d[i] not in (0x48, 0x4C) or d[i + 1] != 0x8D:
            continue
        if d[i + 2] & 0xC7 != 0x05:  # mod=00, rm=101 => rip-relative
            continue
        disp = struct.unpack("<i", d[i + 3:i + 7])[0]
        out.append((i, i + 7 + disp))
    return out


def disas_window(path, start, length=56):
    out = subprocess.run(["objdump", "-d", f"--start-address={start:#x}",
                          f"--stop-address={start + length:#x}", "-M", "intel", path],
                         capture_output=True, text=True).stdout
    return out.splitlines()[6:]


def resolve_len(path, d, site, target):
    """Recover the Go string length that follows a RIP-relative LEA.

    Go passes strings as (ptr,len); the length lands in a nearby register as a
    small immediate. Take the first plausible one whose slice is fully printable.
    """
    for line in disas_window(path, site)[1:8]:
        m = re.search(r"mov\s+\w+,(0x[0-9a-f]+)$", line.strip())
        if not m:
            continue
        n = int(m.group(1), 16)
        if 2 <= n <= 600:
            chunk = d[target:target + n]
            if len(chunk) == n and all(0x20 <= b < 0x7f for b in chunk):
                return n
    return None


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bin", required=True)
    ap.add_argument("--cache", default=None)
    ap.add_argument("cmd", choices=["init", "funcs", "addr", "disas", "strs", "grep"])
    ap.add_argument("arg", nargs="?")
    a = ap.parse_args()

    cache = a.cache or os.path.join(os.path.dirname(os.path.abspath(__file__)), "funcs.json")
    c = load_cache(a.bin, cache)

    if a.cmd == "init":
        print(f"{c['nfunc']} funcs, {len(c['files'])} files, text base {c['text_base']:#x}")
        return

    if a.cmd == "funcs":
        rx = re.compile(a.arg or "", re.I)
        for f in c["funcs"]:
            if rx.search(f["name"]):
                print(f"{f['addr']:#x}\t{f['span']}\t{f['name']}")
        return

    if a.cmd == "grep":
        d = open(a.bin, "rb").read()
        rx = re.compile(a.arg.encode())
        seen = set()
        for m in rx.finditer(d):
            s = rodata_str(d, m.start())
            if s and s not in seen:
                seen.add(s)
                print(f"{m.start():#x}\t{s}")
        return

    hits = [f for f in c["funcs"] if f["name"] == a.arg]
    if not hits:
        sys.exit(f"no exact function {a.arg!r}; use `funcs <regex>`")

    if a.cmd == "addr":
        for f in hits:
            print(f"{f['addr']:#x}\t{f['span']}\t{f['name']}")
        return

    if a.cmd == "disas":
        for f in hits:
            s, e = f["addr"], f["addr"] + f["span"]
            out = subprocess.run(["objdump", "-d", f"--start-address={s:#x}",
                                  f"--stop-address={e:#x}", "-M", "intel", a.bin],
                                 capture_output=True, text=True).stdout
            print("\n".join(out.splitlines()[6:]))
        return

    if a.cmd == "strs":
        d = open(a.bin, "rb").read()
        for f in hits:
            s, e = f["addr"], f["addr"] + f["span"]
            print(f"### {f['name']}  {s:#x}+{f['span']}")
            seen = set()
            for site, target in lea_targets(d, s, e):
                n = resolve_len(a.bin, d, site, target)
                text = d[target:target + n].decode() if n else rodata_str(d, target)[:120]
                if len(text) < 2 or text in seen:
                    continue
                seen.add(text)
                print(f"  {target:#x}{'' if n else '?'}\t{text}")


if __name__ == "__main__":
    main()
