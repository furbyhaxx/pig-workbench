#!/usr/bin/env python3
"""Verify extracted bytes, reproducibility, links and chapter frontmatter."""
import ast
import hashlib
import json
import mmap
import re
import subprocess
import sys
from pathlib import Path

import yaml

root = Path(__file__).resolve().parent.parent
binary = Path(sys.argv[1])
evidence = root / "evidence"
cache = json.loads((root / "tools/funcs.json").read_text())
assert binary.stat().st_size == 234144144
assert hashlib.sha256(binary.read_bytes()).hexdigest() == "1edfb30477447825f0aaa05fd9a91d82f9adf26686f41cd4f5c70b90abe383e8"
assert cache["nfunc"] == len(cache["funcs"]) == 174452
assert len(cache["files"]) == 6875 and cache["nfiles"] == 6931
assert cache["text_base"] == 0x6819000


def assignments(filename, variable):
    tree = ast.parse((evidence / filename).read_text())
    for node in tree.body:
        if isinstance(node, ast.Assign) and any(isinstance(t, ast.Name) and t.id == variable for t in node.targets):
            return ast.literal_eval(node.value)
    raise ValueError(f"{variable} not found in {filename}")


def call_json(script):
    return json.loads(subprocess.check_output([sys.executable, str(evidence / script), str(binary)], text=True))


with binary.open("rb") as f:
    data = mmap.mmap(f.fileno(), 0, access=mmap.ACCESS_READ)
    count = 0
    for name, parts in assignments("extract-system-prompts.py", "entries").items():
        expected = b"".join(data[offset:offset + length] for offset, length in parts)
        assert (evidence / "system-prompt" / name).read_bytes() == expected, name
        count += 1
    for name, (offset, length) in assignments("extract-formatter.py", "entries").items():
        assert (evidence / "system-prompt" / name).read_bytes() == data[offset:offset + length], name
        count += 1
    separator = data[0x6545438:0x6545439]
    assert separator == b"\n"
    for name, parts in assignments("extract-cumulative.py", "entries").items():
        expected = separator.join(data[offset:offset + length] for offset, length in parts)
        assert (evidence / "system-prompt" / name).read_bytes() == expected, name
        count += 1
    print(f"OK: {count} prompt and formatter extracts match binary ranges")

    constructors = json.loads((evidence / "tools/constructor-scan.json").read_text())
    assert constructors == call_json("extract-tool-literals.py")
    oneharness = json.loads((evidence / "tools/oneharness-scan.json").read_text())
    assert oneharness == call_json("extract-oneharness.py")
    conditional = json.loads((evidence / "tools/conditional-descriptions.json").read_text())
    assert conditional == call_json("extract-tool-fragments.py")
    values = []
    for entry in constructors:
        values.extend(entry["name"] + entry["description"])
    for methods in oneharness.values():
        values.extend(methods.values())
    values.extend(conditional.values())
    for value in values:
        offset, length = int(value["offset"], 16), value["bytes"]
        assert data[offset:offset + length] == value["text"].encode(), value["offset"]
    print(f"OK: {len(values)} tool name/description literals match binary ranges and scans reproduce")

rpc = json.loads((evidence / "rpc-delta.json").read_text())
rpc_current = json.loads(subprocess.check_output([sys.executable, str(evidence / "compare-rpcs.py")], text=True))
assert rpc == rpc_current and rpc["baseline_count"] == 359 and rpc["new_count"] == 363
assert rpc["added"] == ["GetGitHubPullRequests", "GetGoogleChatLinkInfo", "GetWorkspaceGitHubPullRequests", "RunTrigger"]
assert rpc["removed"] == []
print("OK: RPC delta reproduces: 359 -> 363, 4 additions, 0 removals")

for script, filename in (
    ("list-template-records.py", "template-records.txt"),
    ("cumulative-layout.py", "cumulative-layout.txt"),
    ("prompt-callgraph.py", "callgraph.txt"),
):
    actual = subprocess.check_output([sys.executable, str(evidence / script), str(binary)], text=True)
    expected = (evidence / "system-prompt" / filename).read_text().split("\n", 1)[1]
    assert actual == expected, script
print("OK: template relocation, cumulative layout and prompt callgraph traces reproduce")

chapters = [root / name for name in ("09-delta-from-1.4.8.md", "10-system-prompt.md", "11-tool-declarations.md")]
links = 0
for chapter in chapters:
    raw = chapter.read_text()
    assert raw.startswith("---\n")
    frontmatter = raw.split("---\n", 2)[1]
    document = yaml.safe_load(frontmatter)["document"]
    assert document["type"] == "research" and document["status"] == "published"
    assert document["revision"] == 1 and document["created_by"] == document["updated_by"] == "worker"
    assert document["id"] == "RES-antigravity-cli-1.4.12-" + chapter.stem
    assert document["created_at"].endswith("+01:00") and document["updated_at"].endswith("+01:00")
    assert document["changelog"][0]["revision"] == 1 and document["changelog"][0]["reason"] == "document creation"
    for target in re.findall(r"\]\(([^)]+)\)", raw):
        path = target.split("#", 1)[0]
        if path and not re.match(r"^[a-z]+://", path):
            assert (chapter.parent / path).exists(), (chapter, target)
            links += 1
    assert "[UNK]" in raw and "[OBS]" in raw
    assert all(not line.endswith((" ", "\t")) for line in raw.splitlines())
print(f"OK: {len(chapters)} chapter frontmatters and {links} local links validate")

text = chapters[2].read_text()
for name in ("list_dir", "find_by_name"):
    row = next(entry for entry in constructors if any(item["text"] == name for item in entry["name"]))
    assert row["description"][0]["text"] in text
row = next(entry for entry in constructors if any(item["text"] == "list_dir" for item in entry["name"]))
shape = row["schema_functions"][0].split("go.shape.", 1)[1][:-1]
assert shape in text
assert '{"name": "write_to_file": "arguments":' in (evidence / "system-prompt/formatter-hermes.txt").read_text()
print("OK: quoted descriptions and list_dir Go shape match extracts; malformed example preserved")
