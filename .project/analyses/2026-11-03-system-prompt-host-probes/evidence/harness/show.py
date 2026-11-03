#!/usr/bin/env python3
"""show.py RUN: per provider request, system text head, tools, and message sequence; then probe log and session system entries."""
import json,sys,glob,os
run=sys.argv[1]; d=f"out/{run}"
def short(c):
    if isinstance(c,list): c=" ".join(x.get("text","") if isinstance(x,dict) else str(x) for x in c)
    c=(c or "").replace("\n","⏎")
    return c[:70]
for line in open(f"{d}/requests.jsonl"):
    r=json.loads(line); msgs=r["messages"] or []
    print(f"req {r['i']}: tools={r['tools']}")
    for m in msgs:
        role=m.get("role"); c=m.get("content")
        if role in ("system","developer"):
            txt=c if isinstance(c,str) else json.dumps(c)
            secs=[l.strip("<>") for l in txt.split("\n") if l.startswith("<") and l.endswith(">") and not l.startswith("</") and " " not in l]
            print(f"   {role}: len={len(txt)} head={short(txt)!r} sections={secs}")
        elif role=="tool": print(f"   tool: {short(c)!r}")
        else:
            tc=[t['function']['name'] for t in m.get('tool_calls') or []]
            print(f"   {role}: {short(c)!r}{' calls='+str(tc) if tc else ''}")
print("--- probe log")
for line in open(f"{d}/probe.jsonl"):
    o=json.loads(line)
    keep={k:o[k] for k in o if k in ("kind","role","n","promptLen","promptSections","seenForced","seenSections","model","provider","thinking","thinkingErr","findApi","findReasoning","err","before","after","reason","infoErr")}
    print("  ",json.dumps(keep)[:400])
print("--- session")
for f in glob.glob(f"{d}/sessions/*.jsonl"):
    for line in open(f):
        o=json.loads(line); t=o.get("type"); m=o.get("message") or {}
        if t=="message":
            role=m.get("role"); c=m.get("content")
            if role=="system":
                secs=c if isinstance(c,(dict,list)) else short(c)
                print("   system:", json.dumps(m)[:300])
            else: print(f"   {role} {m.get('customType','')}: {short(c if isinstance(c,str) else json.dumps(c))!r}")
        elif t in ("compaction","custom_message","custom"): print("  ",t, json.dumps(o)[:300])
        else: print("  ",t)
