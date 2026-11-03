#!/usr/bin/env python3
"""Extract direct constant-return oneharness name and description methods."""
import json
import re
import subprocess
import sys
from pathlib import Path

binary = sys.argv[1]
funcs = json.loads((Path(__file__).parent.parent / "tools/funcs.json").read_text())["funcs"]
method_re = re.compile(r"oneharness/internal/tools/tools\.\(\*(\w+Tool)\)\.(Name|Description)$")
lea_re = re.compile(r"\blea\s+rax,\[rip[^\]]*\].*#\s+([0-9a-f]+)")
length_re = re.compile(r"\bmov\s+ebx,0x([0-9a-f]+)$")
result = {}
with open(binary, "rb") as inp:
    for fn in funcs:
        m = method_re.search(fn["name"])
        if not m or fn["span"] != 32:
            continue
        start = fn["addr"] + 0x80
        lines = subprocess.check_output([
            "objdump", "-d", "-M", "intel", f"--start-address={start:#x}",
            f"--stop-address={start + fn['span']:#x}", binary,
        ], text=True).splitlines()
        for i, line in enumerate(lines):
            p = lea_re.search(line)
            if not p or i + 2 >= len(lines):
                continue
            n = length_re.search(lines[i + 1])
            if not n or not re.search(r"\bret\s*$", lines[i + 2]):
                continue
            offset, size = int(p.group(1), 16), int(n.group(1), 16)
            inp.seek(offset)
            text = inp.read(size).decode("utf-8")
            result.setdefault(m.group(1), {})[m.group(2)] = {
                "function": fn["name"], "cached_address": hex(fn["addr"]),
                "offset": hex(offset), "bytes": size, "text": text,
            }
json.dump(result, sys.stdout, indent=2, ensure_ascii=False)
print()
