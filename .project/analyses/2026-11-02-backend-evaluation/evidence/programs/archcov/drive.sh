#!/usr/bin/env bash
# Runs every (fixture, backend) pair in its own process so crashes and hangs are attributed per pair.
export SEVENZIP=/tmp/wb-cov/7z/7zz
NAMES=$(grep -oE '\{"[^"]+", [a-zA-Z]+\}' eval/main.go | sed -E 's/\{"([^"]+)".*/\1/' | sort -u)
for f in "$@"; do
  while IFS= read -r b; do
    out=$(ONLY="$b" timeout 30 eval/cov "$f" 2>/tmp/wb-cov/err.txt); rc=$?
    if [ -n "$out" ]; then echo "$out" | cut -c1-260
    elif [ $rc -ne 0 ]; then
      why=$(grep -m1 -E '^(fatal error|panic|runtime:)' /tmp/wb-cov/err.txt); [ $rc -eq 124 ] && why="TIMEOUT 30s"
      printf '%-52s %-24s CRASH rc=%s %s\n' "$(basename "$f")" "$b" "$rc" "$why"
    fi
  done <<< "$NAMES"
done
