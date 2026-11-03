#!/usr/bin/env python3
"""Locate literals in the supplied code corpus, preserving line and character offsets."""
import argparse
import bisect
import shlex
import sys

p = argparse.ArgumentParser()
p.add_argument("patterns", nargs="+")
p.add_argument("--window", type=int, default=500)
p.add_argument("--max", type=int, default=4)
a = p.parse_args()
print("$ " + shlex.join(["python3", *sys.argv]))
with open("/tmp/cc_code.txt", encoding="utf-8", errors="replace") as corpus:
    code = corpus.read()
starts = [0] + [pos + 1 for pos, char in enumerate(code) if char == "\n"]
counts = {}
for pattern in a.patterns:
    start = 0
    count = 0
    while count < a.max:
        pos = code.find(pattern, start)
        if pos < 0:
            break
        count += 1
        row = bisect.bisect_right(starts, pos)
        print(f"\n=== {pattern!r} C{row}:{pos-starts[row-1]} ===")
        print(code[max(0, pos-a.window):min(len(code), pos+len(pattern)+a.window)])
        start = pos + len(pattern)
    counts[pattern] = count
print("\nCounts shown:", counts)
