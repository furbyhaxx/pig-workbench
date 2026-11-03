#!/usr/bin/env python3
"""Dump an embedded protobuf FileDescriptorProto from the binary's rodata.

protobuf-go stores each file's descriptor uncompressed; the file name is field 1, so a
`\\x0a<len>` prefix immediately before a `.proto` path marks a descriptor start. The walker
parses greedily and therefore runs past the descriptor into its neighbours: the `file:` header
line may name a later file, but every message/field below it is genuine. Cross-check field
names against the generated `Get*` accessors in data/funcs.txt before trusting attribution.

  pbdump.py --bin AGY 0x5de3c49      # LoadCodeAssist / UserTier (onboarding.proto)
  pbdump.py --bin AGY 0x5bf1ee7      # QuotaSummary buckets + groups
  pbdump.py --bin AGY --find quota_summary.proto   # locate descriptor starts

Offsets are file offsets; vaddr == file offset throughout this binary.
"""
import argparse
import re
import sys

BIN = '/home/furbyhaxx/.local/bin/agy'
D = b''


def varint(b, i):
    r = 0
    s = 0
    while True:
        x = b[i]
        i += 1
        r |= (x & 0x7f) << s
        if not x & 0x80:
            return r, i
        s += 7
        if s > 63:
            raise ValueError('varint')


def fields(b, start, end):
    i = start
    out = []
    while i < end:
        try:
            tag, i = varint(b, i)
        except Exception:
            break
        fn = tag >> 3
        wt = tag & 7
        if fn == 0 or fn > 300:
            break
        if wt == 0:
            v, i = varint(b, i)
            out.append((fn, 0, v))
        elif wt == 1:
            v = b[i:i + 8]
            i += 8
            out.append((fn, 1, v))
        elif wt == 2:
            n, i = varint(b, i)
            out.append((fn, 2, b[i:i + n]))
            i += n
        elif wt == 5:
            v = b[i:i + 4]
            i += 4
            out.append((fn, 5, v))
        else:
            break
        if i > end:
            break
    return out, i


TYPES = {1: 'double', 2: 'float', 3: 'int64', 4: 'uint64', 5: 'int32', 6: 'fixed64',
         7: 'fixed32', 8: 'bool', 9: 'string', 10: 'group', 11: 'message', 12: 'bytes',
         13: 'uint32', 14: 'enum', 15: 'sfixed32', 16: 'sfixed64', 17: 'sint32', 18: 'sint64'}
LABEL = {1: '', 2: 'required ', 3: 'repeated '}


def dump_field(f, ind):
    name = ''
    num = None
    typ = 0
    lbl = 0
    tn = ''
    for fn, wt, v in f:
        if fn == 1:
            name = v.decode('utf-8', 'replace')
        elif fn == 3:
            num = v
        elif fn == 4:
            lbl = v
        elif fn == 5:
            typ = v
        elif fn == 6:
            tn = v.decode('utf-8', 'replace')
    t = tn.split('.')[-1] if tn else TYPES.get(typ, str(typ))
    print(f"{'  ' * ind}{LABEL.get(lbl, '')}{t} {name} = {num};")


def dump_enum(b, ind):
    name = ''
    vals = []
    for fn, wt, v in b:
        if fn == 1:
            name = v.decode()
        elif fn == 2:
            vn = ''
            num = 0
            for f2, w2, v2 in fields(v, 0, len(v))[0]:
                if f2 == 1:
                    vn = v2.decode('utf-8', 'replace')
                elif f2 == 2:
                    num = v2
            vals.append((vn, num))
    print(f"{'  ' * ind}enum {name} {{")
    for vn, num in vals:
        print(f"{'  ' * ind}  {vn} = {num};")
    print(f"{'  ' * ind}}}")


def dump_msg(b, ind):
    name = ''
    nested = []
    fieldsl = []
    enums = []
    oneofs = []
    for fn, wt, v in b:
        if fn == 1:
            name = v.decode('utf-8', 'replace')
        elif fn == 2:
            fieldsl.append(v)
        elif fn == 3:
            nested.append(v)
        elif fn == 4:
            enums.append(v)
        elif fn == 8:
            oneofs.append(v.decode('utf-8', 'replace'))
    print(f"{'  ' * ind}message {name} {{")
    for e in enums:
        dump_enum(fields(e, 0, len(e))[0], ind + 1)
    for n in nested:
        dump_msg(fields(n, 0, len(n))[0], ind + 1)
    for fl in fieldsl:
        dump_field(fields(fl, 0, len(fl))[0], ind + 1)
    if oneofs:
        print(f"{'  ' * ind}  // oneofs: {', '.join(oneofs)}")
    print(f"{'  ' * ind}}}")


def top(start):
    f, _ = fields(D, start, min(start + 400000, len(D)))
    name = ''
    pkg = ''
    deps = []
    msgs = []
    enums = []
    for fn, wt, v in f:
        if fn == 1:
            name = v.decode('utf-8', 'replace')
        elif fn == 2:
            pkg = v.decode('utf-8', 'replace')
        elif fn == 3:
            deps.append(v.decode('utf-8', 'replace'))
        elif fn == 4:
            msgs.append(v)
        elif fn == 5:
            enums.append(v)
    print(f"// file: {name}")
    print(f"// package: {pkg}")
    print(f"// deps: {', '.join(d.split('/')[-1] for d in deps)}")
    for e in enums:
        dump_enum(fields(e, 0, len(e))[0], 0)
    for m in msgs:
        dump_msg(fields(m, 0, len(m))[0], 0)


if __name__ == '__main__':
    ap = argparse.ArgumentParser()
    ap.add_argument('--bin', default=BIN)
    ap.add_argument('--find', default=None, help='print descriptor starts for this .proto name')
    ap.add_argument('offset', nargs='?', help='hex file offset of a descriptor start')
    a = ap.parse_args()
    D = open(a.bin, 'rb').read()
    if a.find:
        path = a.find.encode()
        for m in re.finditer(re.escape(path), D):
            # the path run ends at the first non-printable byte; find its start by locating the
            # `0x0a <len>` prefix whose length matches the run
            end = m.start()
            while end < len(D) and 0x20 <= D[end] < 0x7F:
                end += 1
            start = None
            for p in range(m.start(), max(m.start() - 120, 0), -1):
                if D[p - 1] == end - p and D[p - 2] in (0x0A, 0x1A):
                    start = p
                    break
            if start is None:
                print(f'{m.start():#x}  (no length prefix found)')
                continue
            tag = 'NAME-FIELD' if D[start - 2] == 0x0A else 'dependency'
            print(f'{start - 2:#x}  {tag}  len={end - start}')
    elif a.offset:
        top(int(a.offset, 16))
    else:
        ap.error('need an offset or --find')
