#!/usr/bin/env python3
"""Minimal stdio MCP server: one tool `echo` that logs whether it received the real secret."""
import json, sys, os, hashlib
S="ghp_S3cr3tPr0beValue0123456789abcdefABCD"
log=open(os.environ.get("MCP_LOG","/tmp/wb-sec/mcp.log"),"a")
for line in sys.stdin:
    m=json.loads(line); mid=m.get("id"); meth=m.get("method")
    if mid is None: continue
    if meth=="initialize":
        res={"protocolVersion":m["params"].get("protocolVersion","2025-06-18"),"capabilities":{"tools":{}},"serverInfo":{"name":"echo","version":"1"}}
    elif meth=="tools/list":
        res={"tools":[{"name":"echo","description":"echo text","inputSchema":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}}]}
    elif meth=="tools/call":
        t=m["params"]["arguments"].get("text","")
        log.write(json.dumps({"secret":S in t,"placeholder":"red-sec-" in t})+"\n"); log.flush()
        res={"content":[{"type":"text","text":"mcp echo: "+t}]}
    else:
        res={}
    sys.stdout.write(json.dumps({"jsonrpc":"2.0","id":mid,"result":res})+"\n"); sys.stdout.flush()
