#!/bin/sh
# go.sh NAME SCRIPT STEPS [env...]: one run with sessions persisted; copies the session file
name=$1; script=$2; steps=$3; shift 3
rm -rf home/agent/sessions/*
env PIGARGS="${PIGARGS:---mode rpc}" TAIL=${TAIL:-3} "$@" python3 run.py "$name" "$script" "$steps"
mkdir -p out/$name/sessions; find home/agent/sessions -name '*.jsonl' -exec cp {} out/$name/sessions/ \;
