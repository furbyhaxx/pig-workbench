#!/usr/bin/env python3
"""Cross-reference rodata strings to the functions that reference them.

Complements goresym.py: goresym answers "what strings does function F use",
this answers "which functions use strings matching R". Read-only.

  xwalk.py --bin AGY refs '<regex>'      # string -> referencing functions
  xwalk.py --bin AGY strfn '<funcregex>' # quick summary: strings per function
"""
import argparse
import bisect
import json
import os
import re
import struct
import sys

HERE = os.path.dirname(os.path.abspath(__file__))


def load_funcs(cache):
    with open(cache) as fh:
        d = json.load(fh)
    return d


def build_index(funcs):
    addrs = [f[0] for f in funcs]
    return addrs


def find_func(funcs, addrs, site):
    i = bisect.bisect_right(addrs, site) - 1
    if i < 0:
        return None
    a, span, name = funcs[i]
    if a <= site < a + span:
        return name
    return None


def printable(d, addr, maxlen=300):
    out = bytearray()
    for b in d[addr:addr + maxlen]:
        if 0x20 <= b < 0x7f:
            out.append(b)
        else:
            break
    return out.decode()


def lea_sites(d, lo, hi):
    """RIP-relative LEA sites in [lo,hi) -> target address."""
    out = []
    for i in range(lo, hi - 7):
        if d[i] not in (0x48, 0x4C) or d[i + 1] != 0x8D:
            continue
        if d[i + 2] & 0xC7 != 0x05:
            continue
        disp = struct.unpack("<i", d[i + 3:i + 7])[0]
        out.append((i, i + 7 + disp))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--bin", required=True)
    ap.add_argument("--cache", default=os.path.join(HERE, "funcs.json"))
    ap.add_argument("--funcmap", default=os.path.join(
        os.path.dirname(HERE), "data", "funcmap.json"))
    ap.add_argument("cmd", choices=["refs"])
    ap.add_argument("arg")
    a = ap.parse_args()

    d = open(a.bin, "rb").read()
    funcs = load_funcs(a.funcmap)
    addrs = build_index(funcs)

    # target windows: every occurrence of the regex, take start address
    rx = re.compile(a.arg.encode())
    windows = []
    seen = set()
    for m in rx.finditer(d):
        s = m.start()
        if s in seen:
            continue
        seen.add(s)
        windows.append((s, m.end()))

    # one pass over .text collecting all LEA sites
    lo = funcs[0][0]
    hi = max(f[0] + f[1] for f in funcs)
    sites = lea_sites(d, lo, hi)
    targets = {}
    for site, tgt in sites:
        targets.setdefault(tgt, []).append(site)

    for s, e in windows:
        text = printable(d, s)
        refs = targets.get(s) or []
        # also accept sites pointing up to 8 bytes before the match start
        for back in range(1, 9):
            refs = refs + targets.get(s - back, [])
        names = sorted({find_func(funcs, addrs, x) or f"<0x{x:x}>" for x in refs})
        print(f"### {s:#x} {text[:160]}")
        for n in names:
            print(f"    {n}")


main()
