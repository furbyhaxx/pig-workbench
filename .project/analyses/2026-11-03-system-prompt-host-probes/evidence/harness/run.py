#!/usr/bin/env python3
"""run3.py NAME SCRIPT.json STEPS.json [extra pig args]; like run2 but auto-answers extension_ui_request. Env: EXTS, PIGARGS, WORK, UI_ANSWERS (json map method->response fields)."""
import json, os, subprocess, sys, time, pathlib, threading
name, script, steps = sys.argv[1], sys.argv[2], json.load(open(sys.argv[3]))
extra = sys.argv[4:]
out = pathlib.Path("out")/name; out.mkdir(parents=True, exist_ok=True)
import shutil
for f in out.glob("*"): shutil.rmtree(f) if f.is_dir() else f.unlink()
env = dict(os.environ, PIG_HOME="/tmp/wb-pr/home", PROBE_LOG=str(out.resolve()/"probe.jsonl"))
mock = subprocess.Popen([sys.executable, "mock.py", "18931", script, str(out/"requests.jsonl")])
time.sleep(0.4)
args = ["pig",*os.environ.get("PIGARGS","--mode rpc --no-session").split(),"--offline","--model",os.environ.get("MODEL","mock/mock-1"),*sum([["-e",x] for x in os.environ.get("EXTS","/tmp/wb-pr/probe-prompt-a").split(",")],[]),*extra]
p = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=open(out/"stderr.txt","w"), env=env, cwd=os.environ.get("WORK","/tmp/wb-pr/work"), text=True)
answers = json.loads(os.environ.get("UI_ANSWERS","{}"))
lock = threading.Lock()
def send(o):
    with lock:
        p.stdin.write(json.dumps(o)+"\n"); p.stdin.flush()
def reader():
    with open(out/"rpc.jsonl","w") as log:
        for line in p.stdout:
            log.write(line); log.flush()
            try: o = json.loads(line)
            except Exception: continue
            if o.get("type")=="extension_ui_request" and o.get("method") in answers:
                send({"type":"extension_ui_response","id":o["id"],**answers[o["method"]]})
t = threading.Thread(target=reader, daemon=True); t.start()
for delay, cmd in steps:
    time.sleep(delay); send(cmd)
time.sleep(float(os.environ.get("TAIL","1.5")))
p.stdin.close()
try: p.wait(20)
except subprocess.TimeoutExpired: p.kill()
t.join(2); mock.kill()
print(name, "exit", p.returncode)
