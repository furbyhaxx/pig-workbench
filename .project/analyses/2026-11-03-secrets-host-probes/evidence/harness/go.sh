#!/bin/sh
# go.sh NAME HOOKS [extra env...]: one probe run with the main scenario, session persisted
name=$1; hooks=$2; shift 2
rm -rf out/$name
rm -f mcp.log; rm -rf home/agent/sessions/*
env GITHUB_TOKEN="ghp_""S3cr3tPr0beValue0123456789abcdefABCD" PROBE_EXTPROV=1 PROBE_HOOKS="$hooks" PIGARGS="--mode rpc" TAIL=3 MCP_LOG=/tmp/wb-sec/mcp.log "$@" \
  python3 run.py "$name" scenarios/s_main.json scenarios/st_main.json
cp mcp.log out/$name/mcp.log 2>/dev/null; mkdir -p out/$name/sessions; find home/agent/sessions -name '*.jsonl' -exec cp {} out/$name/sessions/ \;
