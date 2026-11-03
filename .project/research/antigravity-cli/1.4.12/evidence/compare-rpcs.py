#!/usr/bin/env python3
"""Compare the prior RPC inventory with the current generated server stubs."""
import hashlib
import json
import re
from pathlib import Path

root = Path(__file__).parent.parent
baseline = root.parent / "1.4.8/data/rpc_methods.txt"
old = {line.split("/", 1)[1].strip() for line in baseline.read_text().splitlines()
       if line.startswith("LanguageServerService/")}
funcs = json.loads((root / "tools/funcs.json").read_text())["funcs"]
pattern = re.compile(r"\.\(\*UnimplementedLanguageServerServiceServer\)\.([A-Za-z0-9_]+)$")
new = {match.group(1) for fn in funcs if (match := pattern.search(fn["name"]))}
print(json.dumps({
    "baseline_source": "../1.4.8/data/rpc_methods.txt",
    "baseline_sha256": hashlib.sha256(baseline.read_bytes()).hexdigest(),
    "new_source": "tools/funcs.json: UnimplementedLanguageServerServiceServer methods",
    "baseline_count": len(old), "new_count": len(new),
    "added": sorted(new - old), "removed": sorted(old - new),
    "unchanged": sorted(old & new),
}, indent=2))
