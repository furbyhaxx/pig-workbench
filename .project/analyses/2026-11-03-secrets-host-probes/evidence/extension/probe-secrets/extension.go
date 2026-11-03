package probe_secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

const (
	secret      = "ghp_" + "S3cr3tPr0beValue0123456789abcdefABCD"
	placeholder = "red-sec-probeprobeprobep"
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
	fields["kind"] = kind
	fields["t"] = float64(time.Now().UnixMicro()) / 1e6
	b, _ := json.Marshal(fields)
	f.Write(append(b, '\n'))
}

func active(hook string) bool {
	for _, h := range strings.Split(os.Getenv("PROBE_HOOKS"), ",") {
		if h == hook || h == "all" {
			return true
		}
	}
	return false
}

func has(v any) bool {
	b, _ := json.Marshal(v)
	return strings.Contains(string(b), secret)
}

func hasPH(v any) bool {
	b, _ := json.Marshal(v)
	return strings.Contains(string(b), placeholder)
}

// replace walks JSON-shaped values and rewrites strings in place where possible.
func replace(v any, from, to string) any {
	switch t := v.(type) {
	case string:
		return strings.ReplaceAll(t, from, to)
	case map[string]any:
		for k, x := range t {
			t[k] = replace(x, from, to)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = replace(x, from, to)
		}
		return t
	case []map[string]any:
		for i, x := range t {
			t[i] = replace(x, from, to).(map[string]any)
		}
		return t
	default:
		b, err := json.Marshal(v)
		if err != nil || !strings.Contains(string(b), from) {
			return v
		}
		var out any
		if json.Unmarshal(b, &out) != nil {
			return v
		}
		return replace(out, from, to)
	}
}

func keys(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:4])
}

func Extension() *sdk.Extension {
	e := sdk.New("probe-secrets")

	if os.Getenv("PROBE_EXTPROV") == "1" {
		e.RegisterProvider("probext", sdk.ProviderConfig{
			"baseUrl": "http://127.0.0.1:18931/v1",
			"api":     "openai-completions",
			"models":  []any{map[string]any{"id": "probext-1", "name": "Probe ext", "contextWindow": 128000, "maxTokens": 4096}},
		})
	}

	e.OnEvent("input", func(ctx sdk.Context, data map[string]any) (any, error) {
		text, _ := data["text"].(string)
		logf("input", map[string]any{"keys": keys(data), "source": data["source"], "secret": has(data)})
		if active("input") && strings.Contains(text, secret) {
			return map[string]any{"action": "transform", "text": strings.ReplaceAll(text, secret, placeholder)}, nil
		}
		return nil, nil
	})

	e.OnEvent("before_agent_start", func(ctx sdk.Context, data map[string]any) (any, error) {
		sp := ""
		if p, ok := data["systemPrompt"].(*sdk.SystemPrompt); ok && p != nil {
			sp, _ = p.Get()
		}
		logf("before_agent_start", map[string]any{"keys": keys(data), "promptSecret": strings.Contains(sp, secret), "promptLen": len(sp)})
		return nil, nil
	})

	e.OnEvent("tool_call", func(ctx sdk.Context, data map[string]any) (any, error) {
		logf("tool_call", map[string]any{"tool": data["toolName"], "keys": keys(data), "placeholder": hasPH(data["input"]), "secret": has(data["input"])})
		if active("tool_call") && !(os.Getenv("PROBE_SKIP_CODEMODE") == "1" && data["toolName"] == "codemode") {
			if in, ok := data["input"].(map[string]any); ok {
				replace(in, placeholder, secret)
				logf("tool_call_after", map[string]any{"tool": data["toolName"], "secret": has(in)})
			}
		}
		return nil, nil
	})

	e.OnToolResult(func(ctx sdk.Context, data map[string]any) (any, error) {
		logf("tool_result", map[string]any{"tool": data["toolName"], "keys": keys(data), "secret": has(data["content"]), "detailsSecret": has(data["details"]), "inputSecret": has(data["input"])})
		if active("tool_result") && has(data["content"]) {
			return map[string]any{"content": replace(data["content"], secret, placeholder)}, nil
		}
		return nil, nil
	})

	e.OnEvent("user_bash", func(ctx sdk.Context, data map[string]any) (any, error) {
		logf("user_bash", map[string]any{"keys": keys(data), "command": data["command"]})
		if !active("user_bash") {
			return nil, nil
		}
		return map[string]any{"operations": sdk.BashOperations{Exec: func(command, cwd string, o sdk.BashExecOptions) (sdk.BashExecResult, error) {
			cmd := exec.Command("sh", "-c", strings.ReplaceAll(command, placeholder, secret))
			cmd.Dir = cwd
			out, err := cmd.CombinedOutput()
			if o.OnData != nil {
				o.OnData([]byte(strings.ReplaceAll(string(out), secret, placeholder)))
			}
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				return sdk.BashExecResult{}, err
			}
			logf("user_bash_exec", map[string]any{"code": code, "rawSecret": strings.Contains(string(out), secret)})
			return sdk.BashExecResult{ExitCode: &code}, nil
		}}}, nil
	})

	for _, name := range []string{"context", "context_with_system"} {
		name := name
		e.OnEvent(name, func(ctx sdk.Context, data map[string]any) (any, error) {
			fields := map[string]any{"keys": keys(data), "secret": has(data["messages"])}
			if sp, ok := data["systemPrompt"]; ok {
				fields["systemPromptType"] = fmt.Sprintf("%T", sp)
				fields["systemPromptSecret"] = has(sp)
			}
			logf(name, fields)
			if !active(name) {
				return nil, nil
			}
			out := map[string]any{"messages": replace(data["messages"], secret, placeholder)}
			if sp, ok := data["systemPrompt"]; ok {
				out["systemPrompt"] = replace(sp, secret, placeholder)
			}
			return out, nil
		})
	}

	e.OnEvent("before_provider_request", func(ctx sdk.Context, data map[string]any) (any, error) {
		payload := data["payload"]
		logf("before_provider_request", map[string]any{"keys": keys(data), "secret": has(payload), "payloadType": fmt.Sprintf("%T", payload)})
		if os.Getenv("PROBE_BLOCK") == "1" {
			return nil, fmt.Errorf("secrets boundary unavailable (probe)")
		}
		if os.Getenv("PROBE_BLOCK") == "abort" {
			ctx.Abort()
			return nil, fmt.Errorf("secrets boundary unavailable (probe, aborted)")
		}
		if os.Getenv("PROBE_BLOCK") == "empty" {
			if p, ok := payload.(map[string]any); ok {
				p["messages"] = []any{}
				return p, nil
			}
		}
		if os.Getenv("PROBE_BLOCK") == "invalid" {
			return "withheld", nil
		}
		if os.Getenv("PROBE_BLOCK") == "panic" {
			panic("secrets boundary panic (probe)")
		}
		if active("bpr") && has(payload) {
			return replace(payload, secret, placeholder), nil
		}
		return nil, nil
	})

	for _, ev := range []string{"session_before_compact", "session_compact", "session_before_tree"} {
		ev := ev
		e.OnEvent(ev, func(ctx sdk.Context, data map[string]any) (any, error) {
			logf(ev, map[string]any{"keys": keys(data)})
			return nil, nil
		})
	}

	e.Tool("probe_echo", "echo text back", sdk.Schema{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []any{"text"}}, func(ctx sdk.Context, p map[string]any) (any, error) {
		text, _ := p["text"].(string)
		logf("probe_echo", map[string]any{"secret": strings.Contains(text, secret), "placeholder": strings.Contains(text, placeholder), "sum": sum(text)})
		return "echo: " + text, nil
	})

	e.Command("probe-infer", "extension inference through host models", func(ctx sdk.Context, args string) error {
		prov, id := "mock", "mock-1"
		if strings.TrimSpace(args) == "probext" {
			prov, id = "probext", "probext-1"
		}
		model, err := ctx.ModelRegistry().Find(prov, id)
		if err != nil {
			logf("infer", map[string]any{"err": err.Error()})
			return err
		}
		res := ctx.ModelRegistry().Complete(model, map[string]any{
			"systemPrompt": "probe system " + secret,
			"messages":     []any{map[string]any{"role": "user", "content": "infer " + secret, "timestamp": time.Now().UnixMilli()}},
		}, map[string]any{})
		logf("infer", map[string]any{"stopReason": res["stopReason"], "error": res["errorMessage"]})
		return nil
	})

	e.Command("probe-auth", "credential inventory", func(ctx sdk.Context, args string) error {
		no := false
		rt, err := sdk.ModelRuntimeCreate(ctx, sdk.ModelRuntimeCreateOptions{RefreshOnCreate: &no})
		if err != nil {
			logf("auth_list", map[string]any{"err": err.Error()})
			return err
		}
		creds, err := rt.ListCredentials()
		logf("auth_list", map[string]any{"creds": creds, "err": fmt.Sprint(err)})
		for _, c := range creds {
			a, err := ctx.ModelRegistry().GetProviderAuth(c.ProviderID)
			auth, _ := a["auth"].(map[string]any)
			key, _ := auth["apiKey"].(string)
			logf("auth_get", map[string]any{"provider": c.ProviderID, "type": c.Type, "resultKeys": keys(a), "authKeys": keys(auth), "keyIsCommand": strings.HasPrefix(key, "!"), "keyIsSecret": key == secret, "keyLen": len(key), "err": fmt.Sprint(err)})
		}
		return nil
	})

	e.Command("probe-tree-sum", "navigate to the first user message with a branch summary", func(ctx sdk.Context, args string) error {
		entries, _ := ctx.GetEntries()
		target := ""
		for _, raw := range entries {
			var m map[string]any
			json.Unmarshal(raw, &m)
			if msg, ok := m["message"].(map[string]any); ok && msg["role"] == "user" {
				target, _ = m["id"].(string)
				break
			}
		}
		res, err := ctx.NavigateTree(target, map[string]any{"summarize": true})
		logf("command", map[string]any{"cmd": "tree-sum", "target": target, "res": res, "err": fmt.Sprint(err)})
		return err
	})

	e.Command("probe-compact", "compact", func(ctx sdk.Context, args string) error {
		ctx.Compact(map[string]any{})
		logf("command", map[string]any{"cmd": "compact"})
		return nil
	})

	return e
}
