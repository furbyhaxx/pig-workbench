#!/usr/bin/env python3
import hashlib
import os
from pathlib import Path
import re
import subprocess

import yaml

root = Path(__file__).resolve().parents[2]
binary = Path.home() / ".local/bin/agy"
expected_sha = "1edfb30477447825f0aaa05fd9a91d82f9adf26686f41cd4f5c70b90abe383e8"
assert binary.stat().st_size == 234144144
with binary.open("rb") as stream:
    digest = hashlib.file_digest(stream, "sha256").hexdigest()
assert digest == expected_sha
assert subprocess.check_output([str(binary), "--version"], text=True).strip() == "1.4.12"
print("PASS binary size, SHA-256, and version")

chapters = [
    "12-instructions-and-injections.md",
    "13-modes-and-approvals.md",
    "14-skills-and-commands.md",
]
for name in chapters:
    text = (root / name).read_text()
    assert text.startswith("---\n")
    _, frontmatter, body = text.split("---\n", 2)
    doc = yaml.safe_load(frontmatter)["document"]
    assert doc["type"] == "research"
    assert doc["id"] == f"RES-antigravity-cli-1.4.12-{name[:-3]}"
    assert doc["status"] == "published" and doc["revision"] == 1
    assert doc["created_by"] == doc["updated_by"] == "worker"
    assert doc["created_at"] == doc["updated_at"] == "2026-11-03T12:02:40+01:00"
    assert len(doc["changelog"]) == 1
    assert doc["changelog"][0]["revision"] == 1
    assert doc["changelog"][0]["reason"] == "document creation"
    assert doc["changelog"][0]["changes"]
    assert all(tag in body for tag in ("[OBS]", "[INF]", "[UNK]"))
print("PASS frontmatter and evidence grades for 3 chapters")

links = 0
for name in chapters:
    text = (root / name).read_text()
    for target in re.findall(r"(?<!@)\[[^\]]+\]\(([^)]+)\)", text):
        if "://" in target or target.startswith("#"):
            continue
        local = target.split("#", 1)[0]
        assert (root / local).is_file(), (name, target)
        links += 1
print(f"PASS {links} local evidence/chapter links")

samples = {
    (0x5d2ef39, 35): "<RULE[%[1]s]>\n%[2]s\n</RULE[%[1]s]>\n",
    (0x5d00df7, 30): "The current local time is: %s.",
    (0x5df0bcc, 98): '<SKILL>The user requested you read and use the %q skill. The path to the skill file is:\n%s</SKILL>',
    (0x5de9f63, 90): '<WORKFLOW>The user requested you run the %q workflow. Call the %s tool with ScriptPath=%q.',
    (0x5e0bd50, 153): "The following changes were made by %v to: %v. If relevant, proactively run terminal commands to execute this code for the USER. Don't ask for permission.",
}
with binary.open("rb") as stream:
    for (offset, length), expected in samples.items():
        stream.seek(offset)
        assert stream.read(length).decode() == expected, hex(offset)
    stream.seek(0x5e8c9be)
    skill_template = stream.read(1757).decode()
assert skill_template in (root / "evidence/context-recovery/skill-template.txt").read_text()
print("PASS 5 exact template pointer/length samples and full skill template")

schema = (root / "evidence/context-recovery/proto-schemas.txt").read_text()
for fragment in (
    "AGENT_PERMISSION_PRESET_REQUEST_REVIEW = 2;",
    "AGENT_PERMISSION_PRESET_TURBO = 3;",
    "AGENT_PERMISSION_PRESET_VETTED = 4;",
    "AGENT_PERMISSION_PRESET_NONE = 5;",
    "AGENT_PERMISSION_PRESET_AUTO = 6;",
    "PermissionGrants permission_grants = 9;",
    "repeated string ask = 3;",
    "TYPE_FORCE_ASK_HOOK = 6;",
):
    assert fragment in schema, fragment
print("PASS selected preset, grant and interaction schema values")

out = subprocess.run([str(binary), "--help"], capture_output=True, text=True, check=True)
help_text = out.stdout + out.stderr
for flag in ("--mode", "--sandbox", "--dangerously-skip-permissions", "--disable-slash-commands", "--settings"):
    assert flag in help_text
print("PASS CLI help mode/approval switches")

secret = re.compile("GOC" + r"SPX-|AIza[0-9A-Za-z_-]{30}|ya29\.")
files = list((root / "evidence/context-recovery").glob("*.txt"))
for path in files:
    text = path.read_text(errors="replace")
    assert not secret.search(text), path
    assert "/home/" + "furbyhaxx" not in text, path
print(f"PASS credential-pattern and home-path scan of {len(files)} evidence extracts")
print("NOTE checks validate artifacts and byte anchors; no agent turn or approval was executed")
