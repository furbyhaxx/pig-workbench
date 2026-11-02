package probe_m0b

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

var mu sync.Mutex

func logf(kind string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(os.Getenv("PROBE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fields["ext"] = "m0b"
	fields["kind"] = kind
	fields["pid"] = os.Getpid()
	fields["t"] = float64(time.Now().UnixMicro()) / 1e6
	b, _ := json.Marshal(fields)
	f.Write(append(b, '\n'))
}

func mark(res any, tag string) any {
	if r, ok := res.(sdk.AgentToolResult); ok {
		r.Content = append(r.Content, sdk.ToolResultContent{Type: "text", Text: tag})
		return r
	}
	if r, ok := res.(*sdk.AgentToolResult); ok && r != nil {
		r.Content = append(r.Content, sdk.ToolResultContent{Type: "text", Text: tag})
		return r
	}
	return res
}

func Extension() *sdk.Extension {
	e := sdk.New("probe-m0b")
	loc := os.Getenv("PROBE_LOC")
	override := os.Getenv("PROBE_OVERRIDE") == "1"

	e.Flag("list-tools", sdk.FlagOptions{Description: "list tools and exit", Type: sdk.FlagBoolean, Default: false})

	if override {
		readDef, err := sdk.CreateReadToolDefinition(loc, nil)
		if err != nil {
			logf("override", map[string]any{"tool": "read", "err": err.Error()})
		} else {
			run := readDef.Execute
			readDef.Execute = func(ctx sdk.Context, p map[string]any) (any, error) {
				orig, _ := p["path"].(string)
				if os.Getenv("PROBE_ABS") == "1" && loc != "" && !filepath.IsAbs(orig) {
					p["path"] = filepath.Join(loc, orig)
				}
				logf("override", map[string]any{"tool": "read", "phase": "start", "id": ctx.ToolCallID(), "path": orig, "effective": p["path"], "ctxCwd": ctx.Cwd()})
				res, err := run(ctx, p)
				logf("override", map[string]any{"tool": "read", "phase": "end", "id": ctx.ToolCallID(), "type": fmt.Sprintf("%T", res), "err": fmt.Sprint(err)})
				return mark(res, "[WB-OVERRIDE read]"), err
			}
			e.RegisterTool(readDef.ToolDefinition)
		}
		bashDef, err := sdk.CreateBashToolDefinition(loc, &sdk.BashToolOptions{SpawnHook: func(c sdk.BashSpawnContext) (sdk.BashSpawnContext, error) {
			logf("spawn", map[string]any{"cmd": c.Command, "cwd": c.Cwd})
			if loc != "" && os.Getenv("PROBE_HOOKCWD") == "1" {
				c.Cwd = loc
			}
			return c, nil
		}})
		if err != nil {
			logf("override", map[string]any{"tool": "bash", "err": err.Error()})
		} else {
			run := bashDef.Execute
			bashDef.Execute = func(ctx sdk.Context, p map[string]any) (any, error) {
				logf("override", map[string]any{"tool": "bash", "phase": "start", "id": ctx.ToolCallID(), "cmd": p["command"], "ctxCwd": ctx.Cwd()})
				res, err := run(ctx, p)
				logf("override", map[string]any{"tool": "bash", "phase": "end", "id": ctx.ToolCallID(), "type": fmt.Sprintf("%T", res), "err": fmt.Sprint(err), "ctxErr": fmt.Sprint(ctx.Err()), "reason": ctx.CancellationReason()})
				return mark(res, "[WB-OVERRIDE bash]"), err
			}
			e.RegisterTool(bashDef.ToolDefinition)
		}
	}

	for _, ev := range []string{"session_start", "session_before_compact", "session_compact", "session_tree", "session_before_switch", "session_switch", "session_shutdown", "agent_end"} {
		ev := ev
		e.OnEvent(ev, func(ctx sdk.Context, data map[string]any) (any, error) {
			f := map[string]any{"event": ev, "reason": data["reason"]}
			if ev == "session_start" {
				v, err := ctx.GetFlag("list-tools")
				f["flag"] = v
				f["flagErr"] = fmt.Sprint(err)
				f["mode"] = ctx.Mode()
				f["hasUI"] = ctx.HasUI()
				if v == true {
					tools, _ := ctx.GetAllTools()
					names := []string{}
					for _, t := range tools {
						names = append(names, t.Name)
					}
					fmt.Fprintln(os.Stderr, "WB-LIST-TOOLS", names)
					ctx.Notify(fmt.Sprint("WB-LIST-TOOLS ", names), "info")
					f["listed"] = len(names)
					if os.Getenv("PROBE_STDOUT") == "1" {
						fmt.Fprintln(os.Stdout, "WB-STDOUT-LIST", names)
					}
					if os.Getenv("PROBE_SHUTDOWN") == "1" {
						ctx.Shutdown()
					}
				}
			}
			logf("event", f)
			return nil, nil
		})
	}

	e.OnEvent("before_agent_start", func(ctx sdk.Context, data map[string]any) (any, error) {
		opts, err := ctx.GetSystemPromptOptions()
		files := []string{}
		for _, c := range opts.ContextFiles {
			files = append(files, c.Path)
		}
		skills := []string{}
		for _, s := range opts.Skills {
			skills = append(skills, s.Name)
		}
		logf("prompt", map[string]any{"cwd": opts.Cwd, "contextFiles": files, "skills": skills, "err": fmt.Sprint(err)})
		switch os.Getenv("PROBE_BAS") {
		case "steer":
			f := false
			serr := ctx.SendMessage("wb-frame", "[WB-BAS-STEER]", true, sdk.SendMessageOptions{DeliverAs: "steer", TriggerTurn: &f})
			logf("send", map[string]any{"mode": "bas-steer", "err": fmt.Sprint(serr)})
		case "msg":
			return map[string]any{"message": map[string]any{"customType": "wb-frame", "content": "[WB-BAS-MESSAGE]", "display": true}}, nil
		}
		if os.Getenv("PROBE_SUPPRESS") == "1" {
			po, _ := data["systemPromptOptions"].(map[string]any)
			cf := po["contextFiles"]
			logf("prompt", map[string]any{"cfType": fmt.Sprintf("%T", cf)})
			kept := []any{}
			if list, ok := cf.([]any); ok {
				for _, c := range list {
					m, _ := c.(map[string]any)
					if m["path"] == "/tmp/wb-probes/work/AGENTS.md" {
						continue
					}
					kept = append(kept, c)
				}
			}
			po["contextFiles"] = kept
			full, gerr := ctx.GetSystemPrompt()
			logf("prompt", map[string]any{"afterEditHasRoot": strings.Contains(full, "root agents"), "getErr": fmt.Sprint(gerr)})
		}
		return nil, nil
	})

	e.OnEvent("context", func(ctx sdk.Context, data map[string]any) (any, error) {
		msgs, _ := data["messages"].([]any)
		roles := []string{}
		for _, m := range msgs {
			mm, _ := m.(map[string]any)
			r := fmt.Sprint(mm["role"])
			if ct, ok := mm["customType"].(string); ok {
				r += ":" + ct
			}
			roles = append(roles, r)
		}
		logf("context", map[string]any{"roles": roles})
		msgs = append(msgs, map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "[WB-AMBIENT]"}}, "timestamp": time.Now().UnixMilli()})
		return map[string]any{"messages": msgs}, nil
	})

	e.Tool("probe_ask", "ask the user", sdk.Schema{"type": "object", "properties": map[string]any{}}, func(ctx sdk.Context, p map[string]any) (any, error) {
		v, ok, err := ctx.Select("Pick one", []string{"alpha", "beta"})
		logf("dialog", map[string]any{"type": "select", "value": v, "ok": ok, "err": fmt.Sprint(err), "hasUI": ctx.HasUI(), "mode": ctx.Mode()})
		in, ok2, err2 := ctx.Input("Name?", "placeholder")
		logf("dialog", map[string]any{"type": "input", "value": in, "ok": ok2, "err": fmt.Sprint(err2)})
		c, err3 := ctx.Confirm("Sure?", "really")
		logf("dialog", map[string]any{"type": "confirm", "value": c, "err": fmt.Sprint(err3)})
		return fmt.Sprintf("select=%v/%v input=%v/%v confirm=%v", v, ok, in, ok2, c), nil
	})

	e.Command("tools", "list tools", func(ctx sdk.Context, args string) error {
		tools, err := ctx.GetAllTools()
		active, _ := ctx.GetActiveTools()
		out := []map[string]any{}
		for _, t := range tools {
			b, _ := json.Marshal(t)
			var m map[string]any
			json.Unmarshal(b, &m)
			delete(m, "parameters")
			delete(m, "description")
			out = append(out, m)
		}
		logf("command", map[string]any{"cmd": "tools", "tools": out, "active": active, "err": fmt.Sprint(err)})
		ctx.Notify(fmt.Sprintf("WB-TOOLS %d", len(tools)), "info")
		return nil
	})

	e.Command("probe-catalog", "catalog access", func(ctx sdk.Context, args string) error {
		f := map[string]any{"cmd": "catalog", "cwd": ctx.Cwd()}
		file, err := ctx.GetSessionFile()
		f["sessionFile"], f["sessionFileErr"] = file, fmt.Sprint(err)
		id, _ := ctx.GetSessionID()
		f["sessionId"] = id
		list, err := sdk.SessionManagerList(ctx, ctx.Cwd(), nil, nil)
		f["listCount"], f["listErr"] = len(list), fmt.Sprint(err)
		if len(list) > 0 {
			b, _ := json.Marshal(list[0])
			var m map[string]any
			json.Unmarshal(b, &m)
			ks := []string{}
			for k := range m {
				ks = append(ks, k)
			}
			f["listKeys"] = ks
			f["firstPath"] = m["path"]
		}
		all, err := sdk.SessionManagerListAll(ctx, nil, nil)
		f["allCount"], f["allErr"] = len(all), fmt.Sprint(err)
		opts, err := ctx.GetSystemPromptOptions()
		sk := []string{}
		for _, s := range opts.Skills {
			b, _ := json.Marshal(s)
			sk = append(sk, string(b))
		}
		f["promptSkills"], f["optsErr"] = sk, fmt.Sprint(err)
		cmds, err := ctx.GetCommands()
		names := []string{}
		for _, c := range cmds {
			b, _ := json.Marshal(c)
			names = append(names, string(b))
		}
		f["commands"], f["cmdErr"] = names, fmt.Sprint(err)
		trusted, err := ctx.IsProjectTrusted()
		f["trusted"], f["trustErr"] = trusted, fmt.Sprint(err)
		home, err := ctx.ConfigHome()
		f["configHome"], f["homeErr"] = home, fmt.Sprint(err)
		logf("command", f)
		return nil
	})

	e.Command("probe-send", "send message: <mode> <text>", func(ctx sdk.Context, args string) error {
		var mode, text string
		fmt.Sscan(args, &mode, &text)
		var err error
		if mode == "user-followUp" {
			err = ctx.SendUserMessage("[WB-USER-FOLLOWUP] "+text, "followUp")
		} else {
			err = ctx.SendMessage("wb-frame", "[WB-"+mode+"] "+text, true, sdk.SendMessageOptions{DeliverAs: mode})
		}
		idle, _ := ctx.IsIdle()
		pending, _ := ctx.HasPendingMessages()
		logf("send", map[string]any{"mode": mode, "err": fmt.Sprint(err), "idle": idle, "pending": pending})
		return nil
	})
	e.Command("probe-sendtrigger", "send followUp with trigger", func(ctx sdk.Context, args string) error {
		t := true
		err := ctx.SendMessage("wb-frame", "[WB-TRIGGER] "+args, true, sdk.SendMessageOptions{DeliverAs: "followUp", TriggerTurn: &t})
		logf("send", map[string]any{"mode": "followUp+trigger", "err": fmt.Sprint(err)})
		return nil
	})

	e.Command("probe-tree-first", "navigate to first user entry", func(ctx sdk.Context, args string) error {
		br, err := ctx.GetBranch()
		if err != nil || len(br) == 0 {
			logf("command", map[string]any{"cmd": "tree", "err": fmt.Sprint(err)})
			return err
		}
		var target string
		for _, b := range br {
			raw, _ := json.Marshal(b)
			var m map[string]any
			json.Unmarshal(raw, &m)
			if m["type"] == "message" {
				target, _ = m["id"].(string)
				break
			}
		}
		res, err := ctx.NavigateTree(target, nil)
		logf("command", map[string]any{"cmd": "tree", "target": target, "res": res, "err": fmt.Sprint(err)})
		return nil
	})

	e.Command("probe-entry", "append entry", func(ctx sdk.Context, args string) error {
		err := ctx.AppendEntry("pig-workbench.probe", map[string]any{"label": args})
		logf("command", map[string]any{"cmd": "entry", "label": args, "err": fmt.Sprint(err)})
		return nil
	})
	e.Command("probe-branch", "list probe entries", func(ctx sdk.Context, args string) error {
		br, err := ctx.SessionManager().GetBranch(nil)
		labels := []any{}
		types := map[string]int{}
		for _, m := range br {
			types[fmt.Sprint(m["type"])]++
			if m["customType"] == "pig-workbench.probe" {
				d, _ := m["data"].(map[string]any)
				labels = append(labels, d["label"])
			}
		}
		logf("command", map[string]any{"cmd": "branch", "labels": labels, "types": types, "err": fmt.Sprint(err)})
		return nil
	})
	e.Command("probe-compact", "compact", func(ctx sdk.Context, args string) error {
		logf("command", map[string]any{"cmd": "compact-start"})
		ctx.CompactWithOptions(sdk.CompactOptions{
			OnComplete: func(r map[string]any) { logf("command", map[string]any{"cmd": "compact-complete", "keys": len(r)}) },
			OnError:    func(err error) { logf("command", map[string]any{"cmd": "compact-error", "err": err.Error()}) },
		})
		logf("command", map[string]any{"cmd": "compact-returned"})
		return nil
	})
	return e
}
