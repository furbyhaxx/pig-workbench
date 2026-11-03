#!/usr/bin/env python3
"""Extract directly stored name/description literals from tool constructors."""
import bisect
import json
import re
import subprocess
import sys
from pathlib import Path

binary = sys.argv[1]
cache = Path(__file__).parent.parent / "tools" / "funcs.json"
funcs = json.loads(cache.read_text())["funcs"]
addresses = [f["addr"] + 0x80 for f in funcs]
length_re = re.compile(r"\bmov\s+QWORD PTR \[(r[a-z0-9]+)\+0x(10|20)\],0x([0-9a-f]+)")
lea_re = re.compile(r"\blea\s+(r[a-z0-9]+),\[rip[^\]]*\].*#\s+([0-9a-f]+)")
call_re = re.compile(r"\bcall\s+([0-9a-f]+)\b")
result = []
with open(binary, "rb") as inp:
    for fn in funcs:
        name = fn["name"]
        if "/jetski/cortex/tools/" not in name or ").GetToolDefinition" not in name:
            continue
        start = fn["addr"] + 0x80
        cmd = ["objdump", "-d", "-M", "intel", f"--start-address={start:#x}",
               f"--stop-address={start + fn['span']:#x}", binary]
        lines = subprocess.check_output(cmd, text=True).splitlines()
        values = {"name": [], "description": []}
        schemas = []
        for i, line in enumerate(lines):
            call = call_re.search(line)
            if call:
                target = int(call.group(1), 16)
                ix = bisect.bisect_right(addresses, target) - 1
                if ix >= 0 and addresses[ix] == target:
                    called = funcs[ix]["name"]
                    if ".ToJSONSchemaString[" in called and called not in schemas:
                        schemas.append(called)
            match = length_re.search(line)
            if not match:
                continue
            obj, length_field, size = match.groups()
            size = int(size, 16)
            if not 1 <= size <= 20000:
                continue
            pointer_field = int(length_field, 16) - 8
            for j in range(i + 1, min(i + 8, len(lines))):
                ptr = lea_re.search(lines[j])
                if not ptr:
                    continue
                reg, address = ptr.groups()
                store = re.compile(rf"\bmov\s+QWORD PTR \[{obj}\+0x{pointer_field:x}\],{reg}\b")
                if not any(store.search(x) for x in lines[j + 1:j + 4]):
                    continue
                offset = int(address, 16)
                inp.seek(offset)
                raw = inp.read(size)
                try:
                    text = raw.decode("utf-8")
                except UnicodeDecodeError:
                    continue
                if any(ord(c) < 32 and c not in "\t\n\r" for c in text):
                    continue
                key = "name" if length_field == "10" else "description"
                value = {"offset": hex(offset), "bytes": size, "text": text}
                if value not in values[key]:
                    values[key].append(value)
                break
        if any(values.values()) or schemas:
            result.append({"function": name, "cached_address": hex(fn["addr"]),
                           "disassembly_start": hex(start), **values,
                           "schema_functions": schemas})
json.dump(result, sys.stdout, indent=2, ensure_ascii=False)
print()
