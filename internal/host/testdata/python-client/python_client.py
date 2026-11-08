#!/usr/bin/env python3
"""Foreign-runtime Workbench v1 client: speaks the wire protocol with plain JSON.

It is the SPEC-0001 §7 evidence that a non-Go extension behaves like the Go client:
discover, a service call, a stale generation, an oversized request, provider
registration and a served resource read.
"""
import json
import threading
import uuid

import pig_sdk

OWNER = "python-client"
REQUEST, REPLY = "workbench:v1:request", "workbench:v1:reply"
PROVIDER_REQUEST, PROVIDER_REPLY = "workbench:v1:provider-request", "workbench:v1:provider-reply"
CONTROL_BYTES = 65536
DOC = "python line 1\npython line 2\npython line 3\npython line 4\n"


def new_extension() -> pig_sdk.Extension:
    ext = pig_sdk.Extension("python-client")
    instance = uuid.uuid4().hex
    me = {"owner": OWNER, "instance": instance}
    lock = threading.Lock()
    pending = {}

    def on_reply(ctx, data):
        if not isinstance(data, dict) or data.get("to") != me:
            return
        with lock:
            slot = pending.get(data.get("id"))
            if slot is not None:
                slot.append(data)

    ext.events.on(REPLY, on_reply)

    def call(ctx, method, params, generation="", registration_id="", session=None):
        request = {"v": 1, "id": uuid.uuid4().hex, "sessionId": session or ctx.get_session_id(), "from": me, "method": method, "params": params, "timeoutMs": 30000}
        if generation:
            request["generation"] = generation
        if registration_id:
            request["registrationId"] = registration_id
        if len(json.dumps(request).encode()) > CONTROL_BYTES:
            return {"error": {"code": "limitExceeded", "local": True}}
        slot = []
        with lock:
            pending[request["id"]] = slot
        try:
            ctx.events.emit(REQUEST, request)
        finally:
            with lock:
                pending.pop(request["id"], None)
        if len(slot) != 1:
            return {"error": {"code": "unavailable", "replies": len(slot)}}
        return slot[0]

    def discover(ctx, *capabilities):
        reply = call(ctx, "discover", {"majors": [1], "capabilities": list(capabilities)})
        if not reply.get("ok"):
            raise RuntimeError("discover: %r" % (reply,))
        return reply["result"]

    def serve(ctx, data):
        if not isinstance(data, dict) or data.get("to") != me or data.get("method") != "resource.execute":
            return
        params = data["params"]
        lines = DOC.splitlines()
        selector = params.get("selector") or {}
        ranges = selector.get("ranges") or [{"start": 1, "end": len(lines)}]
        start, end = ranges[0]["start"], ranges[0].get("end") or len(lines)
        shown = lines[start - 1:end]
        reply = {"v": 1, "id": data["id"], "sessionId": data["sessionId"], "generation": data["generation"], "to": data["from"], "ok": True}
        if params["operation"] == "stat":
            reply["result"] = {"canonicalUri": "pyfix://fixture/doc.txt", "kind": "file", "size": len(DOC), "lines": len(lines), "revision": "py-1"}
        else:
            reply["result"] = {"canonicalUri": "pyfix://fixture/doc.txt", "kind": "file", "revision": "py-1", "page": {"text": "\n".join(shown) + "\n", "shownRanges": [{"start": start, "end": start + len(shown) - 1}], "totalLines": len(lines), "selectionComplete": True, "resourceComplete": end >= len(lines)}}
        ctx.events.emit(PROVIDER_REPLY, reply)

    ext.events.on(PROVIDER_REQUEST, serve)

    descriptor = {"label": "Python fixture", "summary": "Text served by a Python extension", "schemes": ["pyfix"], "capabilities": ["stat", "read"], "selectorMode": "lines", "backing": "virtual", "scope": "session", "contentKinds": ["text"], "snapshots": {"stableRevision": True, "rangedRead": True, "compareAndSwap": False}, "limits": {"pageBytes": 51200, "pageLines": 2000, "concurrency": 2}}

    def register(ctx):
        endpoint = discover(ctx, "resources")
        reply = call(ctx, "register", {"kind": "resourceProvider", "key": "pyfix", "descriptor": descriptor}, generation=endpoint["generation"])
        if not reply.get("ok"):
            raise RuntimeError("register: %r" % (reply,))

    ext.on_event("session_start", lambda ctx, data: register(ctx))

    def fixture(ctx, args):
        endpoint = discover(ctx, "location")
        location = call(ctx, "location.get", {}, generation=endpoint["generation"])
        stale = call(ctx, "location.get", {}, generation="old-generation")
        oversized = call(ctx, "location.get", {"text": "x" * CONTROL_BYTES}, generation=endpoint["generation"])
        unknown = call(ctx, "no.such.method", {}, generation=endpoint["generation"])
        ctx.notify(json.dumps({"pythonFixture": {"major": endpoint["major"], "capabilities": endpoint["capabilities"], "cwd": location["result"]["cwd"], "stale": stale["error"]["code"], "staleRetryable": stale["error"]["retryable"], "oversized": oversized["error"]["code"], "unknown": unknown["error"]["code"]}}), "info")

    ext.command("python-fixture", "Probe the Workbench v1 service bus from Python.", fixture)
    return ext
