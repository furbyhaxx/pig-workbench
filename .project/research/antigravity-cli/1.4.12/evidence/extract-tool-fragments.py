#!/usr/bin/env python3
"""Extract conditional description literals identified by constructor disassembly."""
import json
import sys

ranges = {
    "write_to_file_without_artifact_instruction": (0x5E2BBBF, 0x169),
    "write_to_file_with_artifact_instruction": (0x5E35872, 0x1DF),
    "manage_subagents_list": (0x5E316B0, 0x1B2),
    "manage_subagents_list_peers": (0x5E3BFE5, 0x232),
    "run_command_background_format": (0x5E3B953, 0x230),
    "run_command_workspace_rule": (0x5E20C94, 0x106),
    "grep_search_result_limit_format": (0x5E031EB, 0x7F),
    "view_file_text_usage_format": (0x5E43041, 0x2A0),
    "view_file_with_binary_format": (0x5DFD2CF, 0x74),
    "view_file_text_only": (0x5DE5979, 0x55),
    "view_file_content_offset_format": (0x5E04CF3, 0x82),
    "view_file_binary_usage": (0x5DF867D, 0x6D),
}
result = {}
with open(sys.argv[1], "rb") as f:
    for name, (offset, size) in ranges.items():
        f.seek(offset)
        raw = f.read(size)
        if len(raw) != size:
            raise ValueError(f"short read at {offset:#x}")
        result[name] = {"offset": hex(offset), "bytes": size, "text": raw.decode("utf-8")}
json.dump(result, sys.stdout, indent=2, ensure_ascii=False)
print()
