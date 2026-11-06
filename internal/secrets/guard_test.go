package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type failingGuard struct {
	delegate TextGuard
	failOn   string
	err      error
}

func (g failingGuard) Redact(text string) (string, error) {
	if text == g.failOn {
		return "", g.err
	}
	return g.delegate.Redact(text)
}

func (g failingGuard) Substitute(text string) (string, error) {
	if text == g.failOn {
		return "", g.err
	}
	return g.delegate.Substitute(text)
}

type neverCalledGuard struct {
	t *testing.T
}

func (g neverCalledGuard) Redact(string) (string, error) {
	g.t.Error("guard ran over content that must stay untouched")
	return "", errors.New("guard must not run")
}

func (g neverCalledGuard) Substitute(string) (string, error) {
	g.t.Error("guard ran over content that must stay untouched")
	return "", errors.New("guard must not run")
}

func newTestGuard(t *testing.T) (*Detector, *Vault, []byte) {
	t.Helper()
	master := bytes.Repeat([]byte{41}, 32)
	detector, vault := newTestDetector(t, master, testVaultPath(t), DetectionPolicy{})
	return detector, vault, mustHMACKey(t, master)
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func requireUnsupported(t *testing.T, out map[string]any, err error, path string) {
	t.Helper()
	var unsupported *ErrUnsupportedShape
	if !errors.As(err, &unsupported) || out != nil {
		t.Fatalf("result %v, err %v is not an unsupported-shape failure", out, err)
	}
	if unsupported.Path != path {
		t.Fatalf("rejected path = %q, want %q", unsupported.Path, path)
	}
}

func TestRedactContentProtectsTextAndPreservesBinary(t *testing.T) {
	detector, _, key := newTestGuard(t)
	secret := "content-guard-secret-value"
	placeholder := Placeholder(key, []byte(secret))
	if err := detector.Learn([]byte(secret), "token", "user"); err != nil {
		t.Fatal(err)
	}
	// The image data carries a token-shaped literal: a token regex must never see it.
	image := map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgo" + "ghp_" + strings.Repeat("Q", 36)}}
	content := []any{
		map[string]any{"type": "text", "text": "push with " + secret + " now", "cache_control": map[string]any{"type": "ephemeral"}},
		image,
	}
	out, err := RedactContent(detector, content)
	if err != nil {
		t.Fatal(err)
	}
	blocks, ok := out.([]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("content = %#v", out)
	}
	text := blocks[0].(map[string]any)
	if text["text"] != "push with "+placeholder+" now" {
		t.Fatalf("text block = %q", text["text"])
	}
	if !reflect.DeepEqual(text["cache_control"], map[string]any{"type": "ephemeral"}) {
		t.Fatalf("block metadata was altered: %#v", text)
	}
	if !reflect.DeepEqual(blocks[1], image) {
		t.Fatalf("binary block was corrupted: %#v", blocks[1])
	}
	if original := content[0].(map[string]any); original["text"] != "push with "+secret+" now" {
		t.Fatal("input content was mutated")
	}
	if textRaw, err := RedactContent(detector, "use "+secret); err != nil || textRaw != "use "+placeholder {
		t.Fatalf("string content = %v, err %v", textRaw, err)
	}
	if nilContent, err := RedactContent(detector, nil); err != nil || nilContent != nil {
		t.Fatalf("absent content = %v, err %v", nilContent, err)
	}
}

func TestRedactContentRejectsUncoveredShapes(t *testing.T) {
	guard := neverCalledGuard{t: t}
	cases := map[string]any{
		"number":          42,
		"string element":  []any{"plain"},
		"unknown block":   []any{map[string]any{"type": "mystery", "text": "x"}},
		"untyped block":   []any{map[string]any{"text": "x"}},
		"text not string": []any{map[string]any{"type": "text", "text": 7}},
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if out, err := RedactContent(guard, content); err == nil {
				t.Fatalf("accepted %#v as %#v", content, out)
			}
		})
	}
	if _, err := RedactContent(nil, "text"); err == nil {
		t.Fatal("accepted a nil guard")
	}
	// Binary-only content never reaches the guard at all.
	binary := []any{map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "ghp_" + strings.Repeat("Z", 36)}}}
	out, err := RedactContent(neverCalledGuard{t: t}, binary)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, binary) {
		t.Fatalf("binary content was altered: %#v", out)
	}
}

func TestRedactMessagesProtectsTextAndToolArguments(t *testing.T) {
	detector, _, key := newTestGuard(t)
	secret := "message-guard-secret-value"
	placeholder := Placeholder(key, []byte(secret))
	if err := detector.Learn([]byte(secret), "token", "user"); err != nil {
		t.Fatal(err)
	}
	arguments := `{"command":"echo \"` + secret + `\"\nnext","count":42,"nested":{"path":"` + secret + `"}}`
	messages := []any{
		map[string]any{"role": "system", "content": "steering " + secret},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "prompt " + secret}}},
		map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
			"id": "call-1", "type": "function", "index": 0,
			"function": map[string]any{"name": "bash", "arguments": arguments},
		}}},
		map[string]any{"role": "tool", "name": "bash", "tool_call_id": "call-1", "content": "token " + secret},
	}
	out, err := RedactMessages(detector, messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustJSON(t, out), secret) {
		t.Fatalf("secret survived message redaction: %s", mustJSON(t, out))
	}
	redacted := out
	if redacted[0].(map[string]any)["role"] != "system" || redacted[3].(map[string]any)["role"] != "tool" {
		t.Fatal("role was altered")
	}
	if redacted[3].(map[string]any)["tool_call_id"] != "call-1" || redacted[3].(map[string]any)["name"] != "bash" {
		t.Fatal("tool result identity was altered")
	}
	call := redacted[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
	if call["id"] != "call-1" || call["function"].(map[string]any)["name"] != "bash" {
		t.Fatal("tool call identity was altered")
	}
	redactedArguments := call["function"].(map[string]any)["arguments"].(string)
	if strings.Contains(redactedArguments, secret) || !strings.Contains(redactedArguments, `"count":42`) {
		t.Fatalf("arguments = %s", redactedArguments)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(redactedArguments), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["command"] != "echo \""+placeholder+"\"\nnext" {
		t.Fatalf("escaped argument value = %q", decoded["command"])
	}
	if decoded["nested"].(map[string]any)["path"] != placeholder {
		t.Fatalf("nested argument value = %q", decoded["nested"])
	}
	if original := messages[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"]; original != arguments {
		t.Fatal("input arguments were mutated")
	}
	if got, err := RedactMessages(detector, nil); err != nil || len(got) != 0 {
		t.Fatalf("absent messages = %v, err %v", got, err)
	}
}

func TestSubstituteArgumentsTouchesValuesNotKeys(t *testing.T) {
	detector, vault, key := newTestGuard(t)
	value := "argument-substitute-value"
	placeholder := Placeholder(key, []byte(value))
	if err := vault.Put(Record{
		Placeholder: placeholder, Kind: "token", Source: "user", Scope: "global",
	}, []byte(value)); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{
		placeholder: "key-must-survive",
		"command":   "curl -H 'x: " + placeholder + "'",
		"count":     3,
		"nested":    []any{map[string]any{"path": placeholder, "flag": true}},
	}
	out, err := SubstituteArguments(detector, args)
	if err != nil {
		t.Fatal(err)
	}
	if out["command"] != "curl -H 'x: "+value+"'" {
		t.Fatalf("command = %q", out["command"])
	}
	if out[placeholder] != "key-must-survive" {
		t.Fatalf("placeholder key was rewritten: %#v", out)
	}
	if _, ok := out[value]; ok {
		t.Fatal("a value key was invented")
	}
	if out["count"] != 3 {
		t.Fatalf("number value = %#v", out["count"])
	}
	nested := out["nested"].([]any)[0].(map[string]any)
	if nested["path"] != value || nested["flag"] != true {
		t.Fatalf("nested argument = %#v", nested)
	}
	if args["command"] != "curl -H 'x: "+placeholder+"'" {
		t.Fatal("input arguments were mutated")
	}
	empty, err := SubstituteArguments(detector, nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty arguments = %#v, err %v", empty, err)
	}
}

func TestRedactProviderPayloadWithholdsOutputOnLateFailure(t *testing.T) {
	detector, vault, key := newTestGuard(t)
	// A token shape, so the first field is detected and persisted as "detected".
	secret := "ghp_" + strings.Repeat("7", 36)
	placeholder := Placeholder(key, []byte(secret))
	guard := failingGuard{delegate: detector, failOn: "third field", err: errors.New("detector echoed " + secret)}
	payload := map[string]any{
		"model":  "fixture-model",
		"stream": true,
		"messages": []any{
			map[string]any{"role": "system", "content": "ok " + secret},
			map[string]any{"role": "user", "content": secret},
			map[string]any{"role": "user", "content": "third field"},
		},
	}
	before := mustJSON(t, payload)
	out, err := RedactProviderPayload(guard, payload)
	var failed *ErrGuardFailed
	if !errors.As(err, &failed) || out != nil {
		t.Fatalf("result %v, err %v is not a withheld failure", out, err)
	}
	if failed.Path != "messages[2].content" {
		t.Fatalf("failed path = %q", failed.Path)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "detector echoed") {
		t.Fatalf("guard error leaked a diagnostic: %v", err)
	}
	if after := mustJSON(t, payload); after != before {
		t.Fatalf("input payload was mutated:\n%s\n%s", before, after)
	}
	// Detection legitimately persisted the earlier field before the failure;
	// the payload is still withheld.
	learned, ok := mustLoad(t, vault)[placeholder]
	if !ok || learned.Source != "detected" {
		t.Fatalf("earlier field was not learned: %#v, present %v", learned, ok)
	}
}

func TestRedactProviderPayloadCapturedOpenAIShape(t *testing.T) {
	detector, _, key := newTestGuard(t)
	secret := "captured-openai-secret-value"
	placeholder := Placeholder(key, []byte(secret))
	if err := detector.Learn([]byte(secret), "token", "user"); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"model":                 "fixture-model",
		"stream":                true,
		"stream_options":        map[string]any{"include_usage": true},
		"temperature":           0.2,
		"max_completion_tokens": 512,
		"prompt_cache_key":      "session-1",
		// Unknown top-level keys are provider metadata: declared limit, copied verbatim.
		"provider_options": map[string]any{"gateway": map[string]any{"only": []any{"fixture"}}},
		"messages": []any{
			map[string]any{"role": "system", "content": "You are a fixture. Key: " + secret},
			map[string]any{"role": "user", "content": "use " + secret},
			map[string]any{"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": "call-0", "type": "function", "index": 0,
				"function": map[string]any{"name": "bash", "arguments": `{"command":"echo ` + secret + `"}`},
			}}},
			map[string]any{"role": "tool", "name": "bash", "tool_call_id": "call-0", "content": []any{map[string]any{"type": "text", "text": "ran " + secret}}},
		},
		"tools": []any{map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "bash",
				"description": "run a command using " + secret,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string", "description": "shell command for " + secret},
						"mode":    map[string]any{"type": "string", "enum": []any{"user", "read"}, "default": "read"},
						// A parameter literally named description is a schema object, not prose.
						"description": map[string]any{"type": "string", "description": "free text " + secret},
					},
					"required": []any{"command"},
				},
			},
		}},
	}
	before := mustJSON(t, payload)
	out, err := RedactProviderPayload(detector, payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded := mustJSON(t, out)
	if strings.Contains(encoded, secret) {
		t.Fatalf("secret survived the provider payload: %s", encoded)
	}
	for _, structural := range []string{`"model":"fixture-model"`, `"prompt_cache_key":"session-1"`, `"max_completion_tokens":512`, `"temperature":0.2`, `"include_usage":true`, `"provider_options":{"gateway":{"only":["fixture"]}}`} {
		if !strings.Contains(encoded, structural) {
			t.Fatalf("structural key %s was altered: %s", structural, encoded)
		}
	}
	tool := out["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != "bash" {
		t.Fatalf("tool name was altered: %#v", tool["name"])
	}
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	mode := properties["mode"].(map[string]any)
	if !reflect.DeepEqual(mode["enum"], []any{"user", "read"}) || mode["default"] != "read" {
		t.Fatalf("schema contract values were altered: %#v", mode)
	}
	if !reflect.DeepEqual(parameters["required"], []any{"command"}) {
		t.Fatalf("required list was altered: %#v", parameters["required"])
	}
	if !strings.Contains(properties["command"].(map[string]any)["description"].(string), placeholder) {
		t.Fatalf("parameter description was not redacted: %#v", properties["command"])
	}
	named := properties["description"].(map[string]any)
	if named["type"] != "string" || !strings.Contains(named["description"].(string), placeholder) {
		t.Fatalf("parameter named description was not preserved and redacted: %#v", named)
	}
	messages := out["messages"].([]any)
	for index, want := range []string{"system", "user", "assistant", "tool"} {
		if messages[index].(map[string]any)["role"] != want {
			t.Fatalf("role %d = %#v", index, messages[index].(map[string]any)["role"])
		}
	}
	arguments := messages[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"].(string)
	if arguments != `{"command":"echo `+placeholder+`"}` {
		t.Fatalf("tool call arguments = %s", arguments)
	}
	if mustJSON(t, payload) != before {
		t.Fatal("input payload was mutated")
	}
	// The returned payload shares nothing with the input.
	messages[0].(map[string]any)["content"] = "local edit"
	if payload["messages"].([]any)[0].(map[string]any)["content"] == "local edit" {
		t.Fatal("output aliases the input payload")
	}
}

func TestRedactProviderPayloadDeclaredAnthropicAndGeminiShapes(t *testing.T) {
	guard, _, key := newTestGuard(t)
	secret := "declared-provider-secret-value"
	placeholder := Placeholder(key, []byte(secret))
	if err := guard.Learn([]byte(secret), "token", "user"); err != nil {
		t.Fatal(err)
	}
	anthropic := map[string]any{
		"model":      "fixture-anthropic",
		"max_tokens": 256,
		"system":     []any{map[string]any{"type": "text", "text": "system " + secret}},
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo" + secret}},
				map[string]any{"type": "text", "text": "text " + secret},
				map[string]any{"type": "tool_use", "id": "tu-1", "name": "bash", "input": map[string]any{"command": secret}},
			},
		}},
		"tools": []any{map[string]any{"name": "bash", "description": "runs " + secret, "input_schema": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string", "description": "cmd " + secret}}}}},
	}
	gemini := map[string]any{
		"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "part " + secret}, map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "iVBORw0KGgo" + secret}}, map[string]any{"functionCall": map[string]any{"name": "bash", "args": map[string]any{"command": secret}}}}}},
		"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "instruction " + secret}}},
		"generationConfig":  map[string]any{"temperature": 0.1, "stopSequences": []any{secret + "-stop"}},
	}
	anthropicOut, err := RedactProviderPayload(guard, anthropic)
	if err != nil {
		t.Fatal(err)
	}
	blocks := anthropicOut["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if !reflect.DeepEqual(blocks[0].(map[string]any)["source"], map[string]any{"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo" + secret}) {
		t.Fatalf("inline image data was altered: %#v", blocks[0])
	}
	if blocks[1].(map[string]any)["text"] != "text "+placeholder {
		t.Fatalf("anthropic text block = %#v", blocks[1])
	}
	if blocks[2].(map[string]any)["input"].(map[string]any)["command"] != placeholder {
		t.Fatalf("tool_use input = %#v", blocks[2])
	}
	if strings.Contains(mustJSON(t, anthropicOut["system"]), secret) || strings.Contains(mustJSON(t, anthropicOut["tools"]), secret) {
		t.Fatalf("anthropic system or tools kept the secret: %s", mustJSON(t, anthropicOut))
	}
	geminiOut, err := RedactProviderPayload(guard, gemini)
	if err != nil {
		t.Fatal(err)
	}
	parts := geminiOut["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if parts[0].(map[string]any)["text"] != "part "+placeholder {
		t.Fatalf("gemini text part = %#v", parts[0])
	}
	if !reflect.DeepEqual(parts[1].(map[string]any)["inlineData"], map[string]any{"mimeType": "image/png", "data": "iVBORw0KGgo" + secret}) {
		t.Fatalf("gemini inline data was altered: %#v", parts[1])
	}
	if parts[2].(map[string]any)["functionCall"].(map[string]any)["args"].(map[string]any)["command"] != placeholder {
		t.Fatalf("gemini functionCall args = %#v", parts[2])
	}
	if strings.Contains(mustJSON(t, geminiOut["systemInstruction"]), secret) {
		t.Fatalf("gemini instruction kept the secret: %s", mustJSON(t, geminiOut["systemInstruction"]))
	}
	// generationConfig is provider metadata, even when it holds a string.
	if !reflect.DeepEqual(geminiOut["generationConfig"], gemini["generationConfig"]) {
		t.Fatalf("generationConfig was altered: %#v", geminiOut["generationConfig"])
	}
}

func TestRedactProviderPayloadRejectsUncoveredShapes(t *testing.T) {
	guard := neverCalledGuard{t: t}
	cases := map[string]struct {
		payload map[string]any
		path    string
	}{
		"no text position":      {map[string]any{"prompt": "secret"}, "payload"},
		"messages not a list":   {map[string]any{"messages": "secret"}, "messages"},
		"content not a string":  {map[string]any{"messages": []any{map[string]any{"role": "user", "content": 42}}}, "messages[0].content"},
		"unknown block":         {map[string]any{"messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "mystery", "text": "x"}}}}}, "messages[0].content[0]"},
		"unparsed arguments":    {map[string]any{"messages": []any{map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"function": map[string]any{"name": "bash", "arguments": "not json"}}}}}}, "messages[0].tool_calls[0].function.arguments"},
		"instructions not text": {map[string]any{"instructions": 42}, "instructions"},
		"gemini unknown part":   {map[string]any{"contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"audio": "x"}}}}}, "contents[0].parts[0]"},
		"tools arguments type":  {map[string]any{"tools": []any{map[string]any{"function": map[string]any{"name": "bash", "arguments": 42}}}}, "tools[0].function.arguments"},
		"system not text":       {map[string]any{"system": 42}, "system"},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := RedactProviderPayload(guard, testCase.payload)
			requireUnsupported(t, out, err, testCase.path)
		})
	}
	if out, err := RedactProviderPayload(nil, map[string]any{"messages": []any{}}); err == nil || out != nil {
		t.Fatalf("accepted a nil guard: %v, %v", out, err)
	}
}

func TestStructuralIdentitySurvivesShortValueOverlap(t *testing.T) {
	detector, vault, key := newTestGuard(t)
	role := "user"
	placeholder := Placeholder(key, []byte(role))
	if err := vault.Put(Record{
		Placeholder: placeholder, Kind: "token", Source: "user", Scope: "global",
	}, []byte(role)); err != nil {
		t.Fatal(err)
	}
	name := "read"
	namePlaceholder := Placeholder(key, []byte(name))
	if err := vault.Put(Record{
		Placeholder: namePlaceholder, Kind: "token", Source: "user", Scope: "global",
	}, []byte(name)); err != nil {
		t.Fatal(err)
	}
	messages := []any{map[string]any{"role": role, "content": "the user asked"}}
	out, err := RedactMessages(detector, messages)
	if err != nil {
		t.Fatal(err)
	}
	message := out[0].(map[string]any)
	if message["role"] != role {
		t.Fatalf("protocol role was rewritten: %#v", message["role"])
	}
	if message["content"] != "the "+placeholder+" asked" {
		t.Fatalf("content = %#v", message["content"])
	}
	payload, err := RedactProviderPayload(detector, map[string]any{
		"messages": messages,
		"tools":    []any{map[string]any{"type": "function", "function": map[string]any{"name": name}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := payload["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if tool["name"] != name {
		t.Fatalf("tool name was rewritten: %#v", tool["name"])
	}
	if payload["messages"].([]any)[0].(map[string]any)["role"] != role {
		t.Fatal("payload role was rewritten")
	}
	// Arguments are user data: their keys are never substituted, their values are.
	substituted, err := SubstituteArguments(detector, map[string]any{"path": namePlaceholder})
	if err != nil || substituted["path"] != name {
		t.Fatalf("argument = %#v, err %v", substituted, err)
	}
}

func TestRedactMessagesHandlesPiGNativeToolCallAndCustomMessages(t *testing.T) {
	guard, _, key := newTestGuard(t)
	secret := "native-shape-secret-value"
	placeholder := Placeholder(key, []byte(secret))
	if err := guard.Learn([]byte(secret), "token", "user"); err != nil {
		t.Fatal(err)
	}
	messages := []any{
		map[string]any{"role": "system", "content": "", "sections": map[string]any{"cwd": "<cwd>\n/work\n</cwd>"}},
		map[string]any{"role": "custom", "customType": "pig-workbench.context", "content": "note " + secret, "display": true},
		map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "toolCall", "id": "call-1", "name": "bash", "arguments": map[string]any{"command": "echo " + secret, "background": true}},
		}, "stopReason": "toolUse"},
		map[string]any{"role": "toolResult", "toolCallId": "call-1", "content": []any{map[string]any{"type": "text", "text": "out " + secret}}, "details": map[string]any{"cursor": float64(0)}},
	}
	out, err := RedactMessages(guard, messages)
	if err != nil {
		t.Fatalf("PiG's own message shapes must be accepted: %v", err)
	}
	encoded := mustJSON(t, out)
	if strings.Contains(encoded, secret) {
		t.Fatalf("the secret survived: %s", encoded)
	}
	call := out[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if call["arguments"].(map[string]any)["command"] != "echo "+placeholder || call["name"] != "bash" || call["arguments"].(map[string]any)["background"] != true {
		t.Fatalf("toolCall arguments must be redacted by value and keep their shape: %#v", call)
	}
}
