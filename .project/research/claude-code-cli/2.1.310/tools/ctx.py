#!/usr/bin/env python3
"""Print +/- window of characters around each match in the extracted Claude Code corpus.

Usage:
  ctx.py PATTERN [--window 700] [--max 6] [--files code|strings|both] [--regex]

The corpus is minified: one "line" can be megabytes long, so grep line output is
useless. This prints character windows instead, with the match marker.
"""
import argparse
import os
import re
import sys

CORPUS = {
    "code": "/tmp/cc_code.txt",
    "strings": "/tmp/cc_strings.txt",
}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("pattern")
    ap.add_argument("--window", type=int, default=700)
    ap.add_argument("--max", type=int, default=6)
    ap.add_argument("--files", default="both", choices=["code", "strings", "both"])
    ap.add_argument("--regex", action="store_true")
    args = ap.parse_args()

    names = ["code", "strings"] if args.files == "both" else [args.files]
    needle = args.pattern if args.regex else re.escape(args.pattern)
    rx = re.compile(needle)

    shown = 0
    for name in names:
        path = CORPUS[name]
        if not os.path.exists(path):
            print(f"!! missing corpus {path}", file=sys.stderr)
            continue
        with open(path, "r", encoding="utf-8", errors="replace") as fh:
            for line in fh:
                for m in rx.finditer(line):
                    if shown >= args.max:
                        print(f"... stopped at --max {args.max}")
                        return 0
                    shown += 1
                    start = max(0, m.start() - args.window)
                    end = min(len(line), m.end() + args.window)
                    snippet = line[start:end]
                    print(f"\n===== [{name}] hit {shown} at line-offset {m.start()} =====")
                    print(snippet.replace("\t", " "))
    if shown == 0:
        print("no matches", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
