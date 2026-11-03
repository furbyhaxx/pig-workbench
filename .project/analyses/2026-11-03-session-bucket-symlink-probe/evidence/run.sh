#!/bin/bash
# run.sh <cwd> <label> [pig args]  — one prompt, keep stdin open until agent_end
P=/tmp/wb-symprobe; cwd=$1; label=$2; shift 2
out=$P/rpc-$label.jsonl
( echo '{"type":"prompt","message":"hi"}'; sleep 3; echo '{"type":"get_state"}'; sleep 1 ) | (cd $cwd && timeout 20 env PIG_HOME=$P/home pig --mode rpc -ne --offline --model mock/mock-1 "$@" > $out 2>>$P/err.txt)
grep -o '"type":"agent_end"' $out | head -1; grep -o '"sessionFile":"[^"]*"' $out | head -1
