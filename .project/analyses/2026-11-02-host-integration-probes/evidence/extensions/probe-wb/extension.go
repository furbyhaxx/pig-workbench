package probe_wb

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

var mu sync.Mutex
var cancelled sync.Map
var claimBash atomic.Bool

func logf(kind string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(os.Getenv("PROBE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fields["ext"] = "wb"
	fields["kind"] = kind
	fields["pid"] = os.Getpid()
	fields["t"] = float64(time.Now().UnixMicro()) / 1e6
	b, _ := json.Marshal(fields)
	f.Write(append(b, '\n'))
}

func keys(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func Extension() *sdk.Extension {
	e := sdk.New("probe-wb")
	pendingContext := false
	entryCounter := 0

	e.OnLoaded(func(ctx sdk.Context) { logf("loaded", map[string]any{"cwd": ctx.Cwd()}) })
	for _, name := range []string{"session_start", "session_shutdown", "session_before_fork", "session_before_tree", "session_tree", "session_before_switch", "before_agent_start", "agent_start", "agent_end", "turn_start", "tool_execution_start", "tool_execution_end", "session_compact"} {
		ev := name
		e.OnEvent(ev, func(ctx sdk.Context, data map[string]any) (any, error) {
			f := map[string]any{"event": ev, "keys": keys(data)}
			for _, k := range []string{"reason", "toolName", "toolCallId", "parentToolCallId", "previousSessionFile", "targetId", "oldLeafId", "newLeafId", "fromExtension"} {
				if v, ok := data[k]; ok {
					f[k] = v
				}
			}
			logf("event", f)
			return nil, nil
		})
	}

	e.OnEvent("tool_call", func(ctx sdk.Context, data map[string]any) (any, error) {
		input, _ := data["input"].(map[string]any)
		logf("event", map[string]any{"event": "tool_call", "keys": keys(data), "toolName": data["toolName"], "toolCallId": data["toolCallId"], "input": input})
		if b, _ := input["block"].(bool); b {
			return map[string]any{"block": true, "reason": "WB-BARRIER: new instructions apply; re-read before retrying"}, nil
		}
		return nil, nil
	})

	e.OnToolResult(func(ctx sdk.Context, data map[string]any) (any, error) {
		input, _ := data["input"].(map[string]any)
		path, _ := input["path"].(string)
		logf("event", map[string]any{"event": "tool_result", "keys": keys(data), "toolName": data["toolName"], "toolCallId": data["toolCallId"], "path": path})
		if data["toolName"] != "read" {
			return nil, nil
		}
		switch {
		case strings.Contains(path, "inject-result"):
			content, _ := data["content"].([]any)
			content = append(content, map[string]any{"type": "text", "text": "[WB-INJECT-RESULT]"})
			return map[string]any{"content": content}, nil
		case strings.Contains(path, "inject-steer"):
			err := ctx.SendMessage("wb-frame", "[WB-INJECT-STEER]", true, sdk.SendMessageOptions{DeliverAs: "steer"})
			logf("send", map[string]any{"mode": "steer", "err": fmt.Sprint(err)})
		case strings.Contains(path, "inject-next"):
			err := ctx.SendMessage("wb-frame", "[WB-INJECT-NEXTTURN]", true, sdk.SendMessageOptions{DeliverAs: "nextTurn"})
			logf("send", map[string]any{"mode": "nextTurn", "err": fmt.Sprint(err)})
		case strings.Contains(path, "inject-context"):
			pendingContext = true
		}
		return nil, nil
	})

	e.OnEvent("context", func(ctx sdk.Context, data map[string]any) (any, error) {
		msgs, _ := data["messages"].([]any)
		logf("event", map[string]any{"event": "context", "messages": len(msgs)})
		if !pendingContext {
			return nil, nil
		}
		msgs = append(msgs, map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "[WB-INJECT-CONTEXT]"}}, "timestamp": time.Now().UnixMilli()})
		return map[string]any{"messages": msgs}, nil
	})

	render := func(tool string) *sdk.ToolRenderers {
		return &sdk.ToolRenderers{
			Call: func(ctx sdk.Context, args map[string]any, rc sdk.ToolRenderContext, width int) ([]string, error) {
				logf("render", map[string]any{"phase": "call", "tool": tool, "id": rc.ToolCallID, "partial": rc.IsPartial, "started": rc.ExecutionStarted, "argsComplete": rc.ArgsComplete, "width": width})
				return []string{fmt.Sprintf("WB-CALL %s %v", tool, args)}, nil
			},
			Result: func(ctx sdk.Context, r sdk.ToolRenderResult, o sdk.ToolRenderResultOptions, rc sdk.ToolRenderContext, width int) ([]string, error) {
				logf("render", map[string]any{"phase": "result", "tool": tool, "id": rc.ToolCallID, "partial": o.IsPartial, "error": rc.IsError, "expanded": o.Expanded, "width": width})
				return []string{fmt.Sprintf("WB-RESULT %s partial=%v error=%v", tool, o.IsPartial, rc.IsError)}, nil
			},
		}
	}
	e.ToolRenderer(func(tool string, next func() *sdk.ToolRenderers) *sdk.ToolRenderers {
		logf("resolve", map[string]any{"tool": tool})
		if tool == "producer_echo" || tool == "read" || (tool == "bash" && claimBash.Load()) {
			return render(tool)
		}
		return next()
	})

	if _, err := e.Events().On("wb.cancel", func(ctx sdk.Context, data any) error {
		m, _ := data.(map[string]any)
		logf("bus", map[string]any{"step": "cancel-received", "id": m["id"]})
		cancelled.Store(m["id"], true)
		return nil
	}); err != nil {
		logf("bus", map[string]any{"step": "subscribe-failed", "err": err.Error()})
	}
	if _, err := e.Events().On("wb.request", func(ctx sdk.Context, data any) error {
		m, _ := data.(map[string]any)
		sleep, _ := m["sleepMs"].(float64)
		logf("bus", map[string]any{"step": "request-received", "id": m["id"]})
		deadline := time.After(time.Duration(sleep) * time.Millisecond)
	wait:
		for {
			select {
			case <-ctx.Done():
				logf("bus", map[string]any{"step": "listener-cancelled", "id": m["id"], "err": fmt.Sprint(ctx.Err())})
				return ctx.Err()
			case <-deadline:
				break wait
			case <-time.After(50 * time.Millisecond):
				if _, ok := cancelled.Load(m["id"]); ok {
					logf("bus", map[string]any{"step": "listener-saw-cancel-message", "id": m["id"]})
					return nil
				}
			}
		}
		err := ctx.Events().Emit("wb.reply", map[string]any{"id": m["id"], "ok": true})
		logf("bus", map[string]any{"step": "reply-emitted", "id": m["id"], "err": fmt.Sprint(err)})
		return nil
	}); err != nil {
		logf("bus", map[string]any{"step": "subscribe-failed", "err": err.Error()})
	}

	e.Command("probe-claim", "claim bash rendering", func(ctx sdk.Context, args string) error {
		claimBash.Store(true)
		logf("command", map[string]any{"cmd": "claim"})
		return nil
	})
	e.Command("probe-entry", "append a custom entry", func(ctx sdk.Context, args string) error {
		entryCounter++
		err := ctx.AppendEntry("pig-workbench.probe", map[string]any{"n": entryCounter, "label": args})
		logf("command", map[string]any{"cmd": "entry", "n": entryCounter, "err": fmt.Sprint(err)})
		return err
	})
	e.Command("probe-branch", "log branch custom entries", func(ctx sdk.Context, args string) error {
		branch, err := ctx.SessionManager().GetBranch(nil)
		found := []any{}
		for _, m := range branch {
			if m["customType"] == "pig-workbench.probe" {
				found = append(found, m["data"])
			}
		}
		logf("command", map[string]any{"cmd": "branch", "entries": len(branch), "custom": found, "err": fmt.Sprint(err)})
		return err
	})
	e.Command("probe-tree", "navigate tree", func(ctx sdk.Context, args string) error {
		target := strings.TrimSpace(args)
		if target == "first-custom" {
			entries, _ := ctx.GetEntries()
			for _, raw := range entries {
				var m map[string]any
				json.Unmarshal(raw, &m)
				if m["customType"] == "pig-workbench.probe" {
					target, _ = m["id"].(string)
					break
				}
			}
		}
		res, err := ctx.NavigateTree(target, map[string]any{})
		logf("command", map[string]any{"cmd": "tree", "target": args, "res": res, "err": fmt.Sprint(err)})
		return err
	})
	e.Command("probe-reload", "reload", func(ctx sdk.Context, args string) error {
		logf("command", map[string]any{"cmd": "reload-start"})
		err := ctx.Reload()
		logf("command", map[string]any{"cmd": "reload-returned", "err": fmt.Sprint(err)})
		return err
	})
	return e
}
