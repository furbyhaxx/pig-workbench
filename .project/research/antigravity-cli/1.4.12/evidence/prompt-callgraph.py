#!/usr/bin/env python3
"""Resolve direct calls in selected prompt builders against the Go function cache."""
import bisect
import json
import re
import subprocess
import sys
from pathlib import Path

funcs = json.loads((Path(__file__).parent.parent / "tools/funcs.json").read_text())["funcs"]
addresses = [fn["addr"] + 0x80 for fn in funcs]
suffixes = (
    "(*CascadeConversationalMixin).GetSystemPrompt", "(*PromptBuilder).Build",
    "systemprompt.Construct", "systemprompt.Build", "prompt.DefaultSystemPromptForCumulativePrompt",
)
for fn in funcs:
    if not fn["name"].endswith(suffixes):
        continue
    start = fn["addr"] + 0x80
    print(f"\n{fn['name']} start={start:#x}")
    data = subprocess.check_output([
        "objdump", "-d", "-M", "intel", f"--start-address={start:#x}",
        f"--stop-address={start + fn['span']:#x}", sys.argv[1],
    ], text=True)
    seen = set()
    for line in data.splitlines():
        match = re.search(r"\bcall\s+([0-9a-f]+)\b", line)
        if not match:
            continue
        target = int(match.group(1), 16)
        i = bisect.bisect_right(addresses, target) - 1
        if i < 0 or addresses[i] != target or funcs[i]["name"].startswith("runtime."):
            continue
        value = (target, funcs[i]["name"])
        if value not in seen:
            print(f"  {target:#x} {value[1]}")
            seen.add(value)
