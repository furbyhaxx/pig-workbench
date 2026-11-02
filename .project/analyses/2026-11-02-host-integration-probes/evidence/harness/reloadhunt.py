import json, os, subprocess, sys, time, signal, pathlib
out = pathlib.Path("out/reload-hunt"); out.mkdir(parents=True, exist_ok=True)
mock = subprocess.Popen([sys.executable, "mock.py", "18931", "s_life.json", str(out/"requests.jsonl")]); time.sleep(0.4)
hangs = 0
for i in range(int(sys.argv[1])):
    log = out/f"probe-{i}.jsonl"
    env = dict(os.environ, PIG_HOME="/tmp/wb-probes/home", PROBE_LOG=str(log.resolve()), GOTRACEBACK="all")
    err = open(out/f"stderr-{i}.txt", "w")
    p = subprocess.Popen(["pig","--mode","rpc","--no-session","-ne","--offline","--model","mock/mock-1","-e","/tmp/wb-probes/probe-wb","-e","/tmp/wb-probes/probe-producer"], stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=err, cwd="/tmp/wb-probes/work", text=True, env=env)
    for d, c in [(0.5, {"id":"e1","type":"prompt","message":"/probe-entry a"}), (0.5, {"id":"r1","type":"prompt","message":"/probe-reload"})]:
        time.sleep(d); p.stdin.write(json.dumps(c)+"\n"); p.stdin.flush()
    time.sleep(5)
    ok = log.exists() and "reload-returned" in log.read_text()
    if not ok:
        hangs += 1
        p.send_signal(signal.SIGQUIT); time.sleep(2)
        print("hang at", i, flush=True)
    else:
        os.remove(out/f"stderr-{i}.txt")
    try: p.stdin.close()
    except Exception: pass
    try: p.wait(10)
    except subprocess.TimeoutExpired: p.kill()
    if ok: log.unlink()
mock.kill()
print("runs", sys.argv[1], "hangs", hangs)
