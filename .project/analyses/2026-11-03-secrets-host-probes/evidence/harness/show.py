import json,sys
S="ghp_"+"S3cr3tPr0beValue0123456789abcdefABCD"; P="red-sec-probeprobeprobep"
d=sys.argv[1]
print("== requests")
for l in open(d+"/requests.jsonl"):
    r=json.loads(l); raw=json.dumps(r["raw"])
    sysmsg=[m for m in r["raw"].get("messages",[]) if m.get("role") in("system","developer")]
    where=[]
    for k,m in enumerate(r["raw"].get("messages",[])):
        if S in json.dumps(m): where.append("%d:%s"%(k,m.get("role")))
    print(r["i"], r["path"], "auth=",(r["auth"] or "")[:14]+"…", "n=%d"%len(r["raw"].get("messages",[])), "secret_in:",where, "PH" if P in raw else "", "1af7534e" if "1af7534e" in raw else "")
print("== probe")
for l in open(d+"/probe.jsonl"):
    o=json.loads(l); o.pop("t",None); print(json.dumps(o)[:260])
