#!/usr/bin/env python3
"""Show relocated slice headers and string entries used by cumulative defaults."""
import mmap
import struct
import sys

globals_ = [0xC1E3680, 0xC1E36A0]
arrays = [0xC225188, 0xC2251D8]
targets = set(globals_) | {array + i * 16 for array in arrays for i in range(5)}
with open(sys.argv[1], "rb") as f:
    d = mmap.mmap(f.fileno(), 0, access=mmap.ACCESS_READ)
    for pos in range(0x9FA8, 0x9FA8 + 0x1619D90, 24):
        dest, kind, source = struct.unpack_from("<QQq", d, pos)
        if kind & 0xFFFFFFFF != 8 or dest not in targets:
            continue
        file_offset = dest - 0x600000
        length = struct.unpack_from("<Q", d, file_offset + 8)[0]
        print(f"reloc={pos:#x} destination={dest:#x} source={source:#x} adjacent_length={length}")
