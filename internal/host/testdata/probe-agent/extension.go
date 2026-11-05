// Package fixture probes host behaviours Slice 1 depends on.
package fixture

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

var (
	mu       sync.Mutex
	retained sdk.Context
	wakeErr  error
)

func report(ctx sdk.Context, payload map[string]any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		ctx.Notify("probe encode failed: "+err.Error(), "error")
		return
	}
	ctx.Notify(string(encoded), "info")
}

func scheduleWake(base sdk.Context, text string) {
	go func() {
		time.Sleep(150 * time.Millisecond)
		trigger := true
		err := base.SendMessage("probe.wake", text, false, sdk.SendMessageOptions{DeliverAs: "followUp", TriggerTurn: &trigger})
		mu.Lock()
		wakeErr = err
		mu.Unlock()
	}()
}

// Extension exposes probe commands for host-behaviour observations.
func Extension() *sdk.Extension {
	ext := sdk.New("probe-agent")
	ext.OnSessionStart(func(ctx sdk.Context, _ map[string]any) (any, error) {
		mu.Lock()
		retained = ctx
		mu.Unlock()
		return nil, nil
	})
	ext.Command("probe", "Probe host behaviour: wake, name, clear, title, entries", func(ctx sdk.Context, args string) error {
		verb, rest, _ := strings.Cut(strings.TrimSpace(args), " ")
		switch verb {
		case "wake":
			mu.Lock()
			base := retained
			mu.Unlock()
			text := strings.TrimSpace(rest)
			if text == "" {
				text = "PROBE-WAKE"
			}
			scheduleWake(base, text)
			report(ctx, map[string]any{"probe": "wake", "scheduled": true})
			return nil
		case "wake-status":
			mu.Lock()
			err := wakeErr
			mu.Unlock()
			message := ""
			if err != nil {
				message = err.Error()
			}
			report(ctx, map[string]any{"probe": "wake-status", "error": message})
			return nil
		case "name":
			name := strings.TrimSpace(rest)
			if err := ctx.SetSessionName(name); err != nil {
				return err
			}
			current, err := ctx.GetSessionName()
			if err != nil {
				return err
			}
			value := ""
			if current != nil {
				value = *current
			}
			report(ctx, map[string]any{"probe": "name", "requested": name, "read": value, "replicated": ctx.SessionName()})
			return nil
		case "clear":
			if err := ctx.SetSessionName(""); err != nil {
				return err
			}
			current, err := ctx.GetSessionName()
			if err != nil {
				return err
			}
			value := "<nil>"
			if current != nil {
				value = *current
			}
			report(ctx, map[string]any{"probe": "clear", "read": value})
			return nil
		case "title":
			current, _ := ctx.GetSessionName()
			value := "<nil>"
			if current != nil {
				value = *current
			}
			file, err := ctx.GetSessionFile()
			path := "<nil>"
			if err == nil && file != nil {
				path = *file
			}
			count := -1
			infos := 0
			if entries, err := ctx.GetEntries(); err == nil {
				count = len(entries)
				for _, raw := range entries {
					var entry struct {
						Type string `json:"type"`
					}
					if json.Unmarshal(raw, &entry) == nil && entry.Type == "session_info" {
						infos++
					}
				}
			}
			report(ctx, map[string]any{"probe": "title", "read": value, "file": path, "entries": count, "sessionInfoEntries": infos})
			return nil
		case "state":
			idle, err := ctx.IsIdle()
			if err != nil {
				return err
			}
			pending, err := ctx.HasPendingMessages()
			if err != nil {
				return err
			}
			report(ctx, map[string]any{"probe": "state", "idle": idle, "pendingMessages": pending})
			return nil
		case "cwd":
			file, err := ctx.GetSessionFile()
			path := "<nil>"
			if err == nil && file != nil {
				path = *file
			}
			name := ctx.SessionName()
			report(ctx, map[string]any{"probe": "cwd", "cwd": ctx.Cwd(), "sessionFile": path, "sessionName": name})
			return nil
		case "ids":
			entries, err := ctx.GetEntries()
			if err != nil {
				return err
			}
			type row struct {
				ID       string         `json:"id"`
				Type     string         `json:"type"`
				ParentID string         `json:"parentId"`
				Role     string         `json:"role"`
				Message  map[string]any `json:"message"`
			}
			rows := make([]row, 0, len(entries))
			for _, raw := range entries {
				var entry row
				if json.Unmarshal(raw, &entry) == nil {
					if entry.Role == "" && entry.Message != nil {
						entry.Role, _ = entry.Message["role"].(string)
					}
					rows = append(rows, entry)
				}
			}
			report(ctx, map[string]any{"probe": "ids", "entries": rows})
			return nil
		case "entries":
			entries, err := ctx.GetEntries()
			if err != nil {
				return err
			}
			types := make([]string, 0, len(entries))
			for _, raw := range entries {
				var entry struct {
					Type string `json:"type"`
					Data any    `json:"data"`
				}
				if json.Unmarshal(raw, &entry) == nil {
					types = append(types, entry.Type)
				}
			}
			report(ctx, map[string]any{"probe": "entries", "types": types})
			return nil
		case "label":
			entries, err := ctx.GetEntries()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return fmt.Errorf("no entries")
			}
			var last struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(entries[len(entries)-1], &last)
			report(ctx, map[string]any{"probe": "label", "leafId": last.ID})
			return nil
		default:
			return fmt.Errorf("usage: /probe wake [text] | wake-status | name <text> | clear | title | entries | ids | cwd | state | label")
		}
	})
	ext.RegisterTool(sdk.ToolDefinition{Name: "probe_wake", Label: "probe_wake", Description: "Schedule a followUp wake from a background goroutine during the running turn.", Parameters: sdk.Schema{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}, Execute: func(ctx sdk.Context, params map[string]any) (any, error) {
		text, _ := params["text"].(string)
		if text == "" {
			text = "PROBE-WAKE"
		}
		mu.Lock()
		base := retained
		mu.Unlock()
		scheduleWake(base, text)
		return sdk.ToolResult{Content: "scheduled"}, nil
	}})
	return ext
}
