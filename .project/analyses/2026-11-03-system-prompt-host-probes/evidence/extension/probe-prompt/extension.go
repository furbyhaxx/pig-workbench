package probe_prompt

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

// role is replaced per copy (A, B) so two instances can run in one host.
const role = "A"

var (
	mu    sync.Mutex
	calls int
)

func logf(kind string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(os.Getenv("PROBE_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fields["kind"] = kind
	fields["role"] = role
	fields["t"] = float64(time.Now().UnixMicro()) / 1e6
	b, _ := json.Marshal(fields)
	f.Write(append(b, '\n'))
}

func modes() map[string]bool {
	out := map[string]bool{}
	for _, m := range strings.Split(os.Getenv("PROBE_"+role), ",") {
		if m != "" {
			out[m] = true
		}
	}
	return out
}

func dump(name string, text string) {
	dir := filepath.Dir(os.Getenv("PROBE_LOG"))
	os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600)
}

func sectionNames(prompt string) []string {
	names := []string{}
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "<") && strings.HasSuffix(line, ">") && !strings.HasPrefix(line, "</") && !strings.Contains(line, " ") {
			names = append(names, strings.Trim(line, "<>"))
		}
	}
	return names
}

func Extension() *sdk.Extension {
	e := sdk.New("probe-prompt-" + strings.ToLower(role))

	e.OnEvent("before_agent_start", func(ctx sdk.Context, data map[string]any) (any, error) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		m := modes()
		opts, _ := data["systemPromptOptions"].(map[string]any)
		sp := ""
		if p, ok := data["systemPrompt"].(*sdk.SystemPrompt); ok && p != nil {
			sp, _ = p.Get()
		}
		dump(fmt.Sprintf("bas-%s-%d-prompt.txt", role, n), sp)
		ob, _ := json.MarshalIndent(opts, "", " ")
		dump(fmt.Sprintf("bas-%s-%d-options.json", role, n), string(ob))
		info, ierr := ctx.GetModelInfo()
		level, lerr := ctx.GetThinkingLevel()
		var find map[string]any
		if info != nil {
			find, _ = ctx.ModelRegistry().Find(info.Provider, info.ID)
		}
		findKeys := []string{}
		for k := range find {
			findKeys = append(findKeys, k)
		}
		forced, _ := opts["forceSystemPrompt"]
		logf("bas", map[string]any{
			"n": n, "keys": keysOf(data), "optKeys": keysOf(opts), "promptLen": len(sp), "promptSections": sectionNames(sp),
			"seenForced": forced, "seenSections": opts["sections"],
			"model": ctx.Model(), "provider": ctx.ModelProvider(), "qualified": ctx.ModelQualified(),
			"info": info, "infoErr": fmt.Sprint(ierr), "thinking": level, "thinkingErr": fmt.Sprint(lerr),
			"findKeys": findKeys, "findApi": find["api"], "findCompat": find["compat"], "findReasoning": find["reasoning"],
		})
		if m["steer"] {
			f := false
			err := ctx.SendMessage("wb-frame", fmt.Sprintf("[FRAME-%s-%d]", role, n), true, sdk.SendMessageOptions{DeliverAs: "steer", TriggerTurn: &f})
			logf("steer", map[string]any{"n": n, "err": fmt.Sprint(err)})
		}
		if m["sections"] {
			if s, ok := opts["sections"].(*sdk.SystemPromptSections); ok {
				s.Set("probe", fmt.Sprintf("SECTION-%s-%d", role, n))
				s.Set("docs", fmt.Sprintf("DOCS-%s-%d", role, n))
				if m["sections-stable"] {
					s.Set("probe", "SECTION-"+role+"-stable")
					s.Set("docs", "DOCS-"+role+"-stable")
				}
			}
		}
		if m["forceopt"] {
			v := fmt.Sprintf("FORCEOPT-%s-%d", role, n)
			opts["forceSystemPrompt"] = v
		}
		if m["force"] {
			return map[string]any{"systemPrompt": fmt.Sprintf("FORCED-%s-%d\n%s", role, n, sp)}, nil
		}
		return nil, nil
	})

	e.OnSessionStart(func(ctx sdk.Context, data map[string]any) (any, error) {
		active, _ := ctx.GetActiveTools()
		kept := []string{}
		for _, t := range active {
			if t != "probe_extra_"+strings.ToLower(role) {
				kept = append(kept, t)
			}
		}
		ctx.SetActiveTools(kept)
		logf("session_start", map[string]any{"reason": data["reason"], "active": kept})
		return nil, nil
	})

	e.OnEvent("before_provider_request", func(ctx sdk.Context, data map[string]any) (any, error) {
		logf("bpr", map[string]any{"keys": keysOf(data)})
		return nil, nil
	})

	e.Tool("probe_add_tool_"+strings.ToLower(role), "activate the probe_extra tool for the rest of the run", sdk.Schema{"type": "object", "properties": map[string]any{}}, func(ctx sdk.Context, p map[string]any) (any, error) {
		active, err := ctx.GetActiveTools()
		if err != nil {
			return nil, err
		}
		ctx.SetActiveTools(append(active, "probe_extra_"+strings.ToLower(role)))
		after, _ := ctx.GetActiveTools()
		logf("add_tool", map[string]any{"before": active, "after": after})
		return "added", nil
	})
	e.ToolPromptSnippet("probe_add_tool_"+strings.ToLower(role), "probe_add_tool: activate an extra tool")

	e.Tool("probe_extra_"+strings.ToLower(role), "extra probe tool", sdk.Schema{"type": "object", "properties": map[string]any{}}, func(ctx sdk.Context, p map[string]any) (any, error) {
		return "extra", nil
	})
	e.ToolPromptSnippet("probe_extra_"+strings.ToLower(role), "probe_extra_"+strings.ToLower(role)+": extra tool added mid-run")

	return e
}

func keysOf(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
