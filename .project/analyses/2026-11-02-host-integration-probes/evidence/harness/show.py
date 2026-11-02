import json,sys
for l in open(sys.argv[1]):
    r=json.loads(l)
    print("REQUEST",r["i"])
    for m in r["messages"]:
        c=m.get("content")
        if isinstance(c,list): c=" | ".join(x.get("text","<%s>"%x.get("type")) for x in c)
        tc=[t["function"]["name"]+t["function"]["arguments"] for t in m.get("tool_calls") or []]
        if m["role"]=="system": c=(c or "")[:30]
        print("  ",m["role"],(c or "")[:100].replace("\n","\\n"),tc if tc else "")
