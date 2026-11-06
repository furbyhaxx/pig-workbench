//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestLiveAskUsesNativeRPCAndReturnsUserChoice(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	var messages string
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad JSON", 400)
			return
		}
		mu.Lock()
		index := requests
		requests++
		body, _ := json.Marshal(request["messages"])
		messages = string(body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		send := func(delta any, reason any) {
			data, _ := json.Marshal(map[string]any{"id": "ask-fixture", "object": "chat.completion.chunk", "created": 0, "model": "mock", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		if index == 0 {
			args, _ := json.Marshal(map[string]any{"title": "Decision", "questions": []any{map[string]any{"id": "choice", "label": "Choice", "prompt": "Select the fixture answer", "type": "single", "options": []any{map[string]any{"value": "chosen", "label": "Fixture option"}}}}})
			send(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "ask-call", "type": "function", "function": map[string]any{"name": "ask_user", "arguments": string(args)}}}}, nil)
			send(map[string]any{}, "tool_calls")
		} else {
			send(map[string]any{"role": "assistant", "content": "answered"}, nil)
			send(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user")
	fixture.send("ask", "prompt", "Ask the scripted question.")
	event := fixture.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "select"
	})
	options, ok := event["options"].([]any)
	if !ok || len(options) == 0 {
		t.Fatalf("missing native choices: %+v", event)
	}
	var chosen string
	for _, option := range options {
		if value, ok := option.(string); ok && strings.Contains(value, "Fixture option") {
			chosen = value
		}
	}
	if chosen == "" {
		t.Fatalf("missing fixture option: %+v", options)
	}
	response, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": event["id"], "value": chosen})
	if _, err := fmt.Fprintf(fixture.stdin, "%s\n", response); err != nil {
		t.Fatal(err)
	}
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(messages, `chosen`) || !strings.Contains(messages, `answers`) {
		t.Fatalf("user answer did not reach model context: %s", messages)
	}
}

func TestLiveUnansweredAskIsPendingAfterRestartAndNeverAnsweredByDefault(t *testing.T) {
	root := t.TempDir()
	form := map[string]any{"title": "Left open", "questions": []any{map[string]any{"id": "choice", "label": "Choice", "prompt": "Pick one", "type": "single", "options": []any{map[string]any{"value": "a", "label": "Option A"}}}}}
	recorder := &requestRecorder{calls: []scriptedCall{{"ask_user", form}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)

	first := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user")
	first.send("ask", "prompt", "Ask the question.")
	first.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "select"
	})
	// The process ends with the dialog unanswered, as a crash or reload would leave it.
	first.cancel()
	<-first.done
	first.exitErr = nil

	second := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user", "--continue")
	notice := second.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), "left unanswered")
	})
	if !strings.Contains(fmt.Sprint(notice["message"]), "Left open") || !strings.Contains(fmt.Sprint(notice["message"]), "Nothing was answered for you") {
		t.Fatalf("the pending notice must name the form and say nothing was answered: %v", notice["message"])
	}
	second.send("show", "prompt", "/workbench ask show")
	second.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), "Pick one")
	})
	second.send("discard", "prompt", "/workbench ask discard")
	second.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), "Discarded")
	})
	second.close()

	third := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user", "--continue")
	third.send("again", "prompt", "/workbench ask show")
	third.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "notify" && strings.Contains(fmt.Sprint(event["message"]), "No unanswered")
	})
}
