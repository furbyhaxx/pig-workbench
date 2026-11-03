#!/usr/bin/env python3
"""Extract formatter literals directly from disassembly-proven byte ranges."""
import hashlib
import sys
from pathlib import Path

out = Path(__file__).parent / "system-prompt"
entries = {
    "formatter-hermes.txt": (0x5F1F200, 0xE8F),
    "formatter-xml.txt": (0x5EC49BE, 0xA27),
    "formatter-xml-examples.txt": (0x5EB102F, 0x8D1),
    "formatter-xml-required.txt": (0x5D88F67, 0x33),
    "formatter-xml-required-named.txt": (0x5D9D73E, 0x38),
}
with open(sys.argv[1], "rb") as f:
    for name, (offset, size) in entries.items():
        f.seek(offset)
        data = f.read(size)
        if len(data) != size:
            raise ValueError(f"short read at {offset:#x}")
        data.decode("utf-8")
        with (out / name).open("xb") as result:
            result.write(data)
        print(f"{name}\t{offset:#x}+{size}\t{hashlib.sha256(data).hexdigest()}")
