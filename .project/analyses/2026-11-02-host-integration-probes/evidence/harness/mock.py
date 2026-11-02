#!/usr/bin/env python3
"""Scripted OpenAI-compatible streaming server. Script: JSON list of steps; each step {"text":..} or {"tools":[{"name":..,"args":{..}}]}, optional "delay"."""
import json, sys, time, threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
port=int(sys.argv[1]); script=json.load(open(sys.argv[2])); log=open(sys.argv[3],'a')
lock=threading.Lock(); idx=[0]
class H(BaseHTTPRequestHandler):
    def log_message(self,*a): pass
    def do_GET(self):
        self.send_response(200); self.send_header('content-type','application/json'); self.end_headers()
        self.wfile.write(b'{"data":[{"id":"mock-1","object":"model"}]}')
    def do_POST(self):
        body=json.loads(self.rfile.read(int(self.headers['content-length'])))
        with lock:
            i=idx[0]; idx[0]+=1
            log.write(json.dumps({"i":i,"t":time.time(),"messages":body.get("messages"),"tools":[t["function"]["name"] for t in body.get("tools",[])],"other":{k:v for k,v in body.items() if k not in ("messages","tools")}})+"\n"); log.flush()
        step=script[i] if i<len(script) else {"text":"done"}
        self.send_response(200); self.send_header('content-type','text/event-stream'); self.end_headers()
        def send(d):
            self.wfile.write(b"data: "+json.dumps(d).encode()+b"\n\n"); self.wfile.flush()
        base={"id":"c%d"%i,"object":"chat.completion.chunk","created":0,"model":"mock-1"}
        time.sleep(step.get("delay",0))
        if "tools" in step:
            for n,t in enumerate(step["tools"]):
                send({**base,"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":n,"id":"call_%d_%d"%(i,n),"type":"function","function":{"name":t["name"],"arguments":json.dumps(t["args"])}}]}}]})
            send({**base,"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]})
        else:
            send({**base,"choices":[{"index":0,"delta":{"role":"assistant","content":step["text"]}}]})
            send({**base,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]})
        send({**base,"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}})
        self.wfile.write(b"data: [DONE]\n\n"); self.wfile.flush()
ThreadingHTTPServer(("127.0.0.1",port),H).serve_forever()
