#!/usr/bin/env python3
"""Extract the five-string cumulative-prompt slices and join separator."""
import hashlib
import sys
from pathlib import Path

out = Path(__file__).parent / "system-prompt"
entries = {
    "cumulative-antigravity-agent.txt": [(0x5DF61FB, 106), (0x5E0DB5C, 161),
                                        (0x5E133EC, 187), (0x5E1B977, 230), (0x5DEAF9A, 91)],
    "cumulative-cascade.txt": [(0x5E0D3DF, 159), (0x5E13E32, 189),
                               (0x5E133EC, 187), (0x5E1B977, 230), (0x5DEAF9A, 91)],
}
with open(sys.argv[1], "rb") as f:
    f.seek(0x6545438)
    sep = f.read(1)
    if sep != b"\n":
        raise ValueError("unexpected join separator")
    for name, parts in entries.items():
        texts = []
        for offset, size in parts:
            f.seek(offset)
            text = f.read(size)
            if len(text) != size:
                raise ValueError(f"short read at {offset:#x}")
            text.decode("utf-8")
            texts.append(text)
        data = sep.join(texts)
        with (out / name).open("xb") as result:
            result.write(data)
        print(f"{name}\t{len(data)}\t{hashlib.sha256(data).hexdigest()}\t" +
              ", ".join(f"{offset:#x}+{size}" for offset, size in parts))
