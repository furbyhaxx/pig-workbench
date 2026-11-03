#!/usr/bin/env python3
"""Extract known binary ranges; fail instead of replacing existing evidence."""
import hashlib
import sys
from pathlib import Path

binary = Path(sys.argv[1])
out = Path(__file__).parent / "system-prompt"
entries = {
    "identity-assistant.txt": [(0x5DEA233, 90)],
    "identity-agent.txt": [(0x5DF61FB, 106)],
    "identity-coding.txt": [(0x5E05BC7, 132)],
    "template-artifacts.txt": [(0x5FBAFFC, 6022)],
    "template-communication-style.txt": [(0x5EFF11A, 3388)],
    "template-conversation-transcript.txt": [(0x5E956D2, 1885)],
    "template-ephemeral-message.txt": [(0x5E2237F, 271)],
    "template-function-call.txt": [(0x5E3EAE2, 605)],
    "template-guidelines.txt": [(0x5E6D319, 1292)],
    "template-identity.txt": [(0x5E4CE41, 821)],
    "template-persistent-context.txt": [(0x5F1C69B, 3695)],
    "template-plugins.txt": [(0x5E608E6, 1114)],
    "template-skills.txt": [(0x5E8C9BE, 1757)],
    "template-slash-commands.txt": [(0x5E3F45C, 609)],
    "template-test-funcmap.txt": [(0x5E08BA8, 140)],
    "template-async-termination-rule.txt": [(0x5E4A346, 764)],
    "worker-l0.txt": [(0x5E8E54C, 0x6EB), (0x5E76A2F, 0x57E), (0x5E57892, 0x3BD)],
}

with binary.open("rb") as f:
    for name, parts in entries.items():
        data = b""
        for start, length in parts:
            f.seek(start)
            part = f.read(length)
            if len(part) != length:
                raise ValueError(f"short read at {start:#x}")
            data += part
        data.decode("utf-8")
        with (out / name).open("xb") as output:
            output.write(data)
        ranges = ", ".join(f"{start:#x}+{length}" for start, length in parts)
        print(f"{name}\t{len(data)}\t{hashlib.sha256(data).hexdigest()}\t{ranges}")
