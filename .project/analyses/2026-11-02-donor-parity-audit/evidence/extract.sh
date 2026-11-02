#!/usr/bin/env bash
# Regenerates the test-title inventories from the pinned donor checkouts.
set -euo pipefail
O=${OMP:-/mnt/Projects/nukular/nukular-ai/.references/oh-my-pi}
W=${PIWB:-/mnt/Projects/furbyhaxx/pi-workbench}
N=${NUK:-/mnt/Projects/nukular/nukular-ai}
for f in $(find "$W/tests" -name "*.ts" | sort); do rel=${f#$W/tests/}; grep -oE "^\s*(it|test)\(\s*[\"'\`][^\"'\`]+" "$f" | sed -E "s/^\s*(it|test)\(\s*[\"'\`]//" | sed "s#^#$rel\t#"; done > piwb-tests.tsv
for f in $(grep -rl "#\[test\]" "$O/crates/pi-edit"); do rel=${f#$O/crates/pi-edit/}; grep -A2 "#\[test\]" "$f" | grep -oE "fn [a-z0-9_]+" | sed "s/^fn //; s#^#$rel\t#"; done > omp-edit-tests.tsv
find "$O/packages/coding-agent" -path "*node_modules*" -prune -o -name "*.test.ts" -print \
  | grep -i -E "read|grep|find|glob|fetch|hashline|edit|selector|sqlite|archive|json|pdf|summary|supersede|instruction|agents|jobs|bash|ask" \
  | while read -r f; do rel=${f#$O/packages/coding-agent/}; grep -oE "^\s*(it|test)\(\s*[\"'\`][^\"'\`]+" "$f" | sed -E "s/^\s*(it|test)\(\s*[\"'\`]//" | sed "s#^#$rel\t#"; done \
  | grep -v -E "^test/(task/|marketplace|collab|eval/|mcp|rpc-subagents|hook-editor|keybindings|modes/components/(copy|custom-editor)|rewind|utils/changelog|agent-session-responses|sdk-mcp)" | grep -v -i subagent > omp-inscope.tsv
for f in $(grep -rl "#\[test\]" "$N/crates/nukular-agent-sdk/src/frame"); do rel=${f#$N/crates/}; grep -A2 "#\[test\]" "$f" | grep -oE "fn [a-z0-9_]+" | sed "s/^fn //; s#^#$rel\t#"; done > nuk-tests.tsv
