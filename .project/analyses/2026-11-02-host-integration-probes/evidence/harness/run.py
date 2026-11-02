#!/usr/bin/env python3
"""run.py NAME SCRIPT.json STEPS.json [extra pig args...]; STEPS: list of [sleep_seconds, rpc_obj]"""
import json, os, subprocess, sys, time, pathlib
name, script, steps = sys.argv[1], sys.argv[2], json.load(open(sys.argv[3]))
extra = sys.argv[4:]
out = pathlib.Path("out")/name; out.mkdir(parents=True, exist_ok=True)
for f in out.glob("*"): f.unlink()
env = dict(os.environ, PIG_HOME="/tmp/wb-probes/home", PROBE_LOG=str(out.resolve()/"probe.jsonl"))
mock = subprocess.Popen([sys.executable, "mock.py", "18931", script, str(out/"requests.jsonl")])
time.sleep(0.4)
args = ["pig","--mode","rpc","-ne","--offline","--model","mock/mock-1","-e","/tmp/wb-probes/probe-wb","-e",os.environ.get("PRODUCER","/tmp/wb-probes/probe-producer"),*extra]
p = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=open(out/"rpc.jsonl","w"), stderr=open(out/"stderr.txt","w"), env=env, cwd="/tmp/wb-probes/work", text=True)
for delay, cmd in steps:
    time.sleep(delay)
    p.stdin.write(json.dumps(cmd)+"\n"); p.stdin.flush()
time.sleep(float(os.environ.get("TAIL","1.5")))
p.stdin.close()
try: p.wait(20)
except subprocess.TimeoutExpired: p.kill()
mock.kill()
print(name, "exit", p.returncode)
