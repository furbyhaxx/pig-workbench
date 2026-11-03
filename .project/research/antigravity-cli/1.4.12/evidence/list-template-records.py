#!/usr/bin/env python3
"""List embedded template paths and their relocated content ranges."""
import mmap
import struct
import sys

segments = [
    (0, 0, 0xA3771D0), (0xA5771E0, 0xA3771E0, 0x1782648),
    (0xBEF9828, 0xBAF9828, 0xBA518), (0xC1B3D40, 0xBBB3D40, 0x10B688),
    (0xC52A8A0, 0xBD2A8A0, 0x2165088),
]
prefix = b"google3/third_party/jetski/prompt/template_provider/templates/"


def file_offset(address):
    for virtual, offset, size in segments:
        if virtual <= address and address + 32 <= virtual + size:
            return offset + address - virtual
    return None


with open(sys.argv[1], "rb") as f:
    d = mmap.mmap(f.fileno(), 0, access=mmap.ACCESS_READ)
    records = {}
    for pos in range(0x9FA8, 0x9FA8 + 0x1619D90, 24):
        dest, kind, src = struct.unpack_from("<QQq", d, pos)
        if kind & 0xFFFFFFFF != 8 or src < 0 or src + len(prefix) > len(d):
            continue
        if d[src:src + len(prefix)] != prefix:
            continue
        off = file_offset(dest)
        if off is None:
            continue
        size = struct.unpack_from("<Q", d, off + 8)[0]
        if len(prefix) < size < 300:
            try:
                name = d[src:src + size].decode("utf-8")
            except UnicodeDecodeError:
                continue
            records[dest + 16] = (name, struct.unpack_from("<Q", d, off + 24)[0])
    for pos in range(0x9FA8, 0x9FA8 + 0x1619D90, 24):
        dest, kind, src = struct.unpack_from("<QQq", d, pos)
        if kind & 0xFFFFFFFF == 8 and dest in records:
            name, size = records[dest]
            if 0 < size < 1000000 and src + size <= len(d):
                print(f"{name}\tcontent={src:#x}\tbytes={size}\trelocation={pos:#x}")
