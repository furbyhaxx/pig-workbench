//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestLiveAskMultiAndPreviewThroughNativeDialogs(t *testing.T) {
	root := t.TempDir()
	form := map[string]any{"title": "Choices", "questions": []any{
		map[string]any{"id": "targets", "label": "Targets", "prompt": "Choose targets", "type": "multi", "options": []any{
			map[string]any{"value": "a", "label": "Alpha"}, map[string]any{"value": "b", "label": "Beta"},
		}},
		map[string]any{"id": "layout", "label": "Layout", "prompt": "Choose a layout", "type": "preview", "options": []any{
			map[string]any{"value": "compact", "label": "Compact", "preview": "Compact preview\nsecond line"},
		}},
	}}
	recorder := &requestRecorder{calls: []scriptedCall{{"ask_user", form}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user")
	fixture.send("ask", "prompt", "Ask the questions.")
	for index, wanted := range []string{"Alpha", "Beta", "Done selecting", "Compact", "Use this option"} {
		event := fixture.wait(func(event map[string]any) bool {
			return event["type"] == "extension_ui_request" && event["method"] == "select"
		})
		if index == 4 && fmt.Sprint(event["title"]) != "Compact preview\nsecond line" {
			t.Fatalf("preview text was not shown: %+v", event)
		}
		options, ok := event["options"].([]any)
		if !ok {
			t.Fatalf("missing select options: %+v", event)
		}
		choice := ""
		for _, option := range options {
			if value, ok := option.(string); ok && strings.Contains(value, wanted) {
				choice = value
				break
			}
		}
		if choice == "" {
			t.Fatalf("missing %q in %v", wanted, options)
		}
		response, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": event["id"], "value": choice})
		if _, err := fmt.Fprintf(fixture.stdin, "%s\n", response); err != nil {
			t.Fatal(err)
		}
	}
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	results := toolResults(t, recorder.bodies[len(recorder.bodies)-1])
	if len(results) != 1 {
		t.Fatalf("expected one questionnaire result, got %v", results)
	}
	var result struct {
		Answers map[string]struct {
			Values []string `json:"values"`
		} `json:"answers"`
	}
	if err := json.Unmarshal([]byte(results[0]), &result); err != nil {
		t.Fatalf("invalid questionnaire result %q: %v", results[0], err)
	}
	if !slices.Equal(result.Answers["targets"].Values, []string{"a", "b"}) || !slices.Equal(result.Answers["layout"].Values, []string{"compact"}) {
		t.Fatalf("wrong submitted answers: %+v", result.Answers)
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

func TestLiveAskFreeformAndSecretReturnTheResultShape(t *testing.T) {
	root := t.TempDir()
	secret := "ghp_" + strings.Repeat("Qw4eR5", 5)
	form := map[string]any{"title": "Types", "questions": []any{
		map[string]any{"id": "notes", "label": "Notes", "prompt": "Describe it", "type": "freeform"},
		map[string]any{"id": "token", "label": "Token", "prompt": "Your token", "type": "secret", "secret": map[string]any{"name": "ask-live-token", "scope": "global"}},
	}}
	recorder := &requestRecorder{calls: []scriptedCall{{"ask_user", form}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user")
	fixture.send("ask", "prompt", "Ask the typed questions.")
	for answered := 0; answered < 4; {
		event := fixture.wait(func(event map[string]any) bool {
			return event["type"] == "extension_ui_request" && (event["method"] == "input" || event["method"] == "select")
		})
		title := fmt.Sprint(event["title"])
		var value string
		switch {
		case event["method"] == "select":
			value = fmt.Sprint(event["options"].([]any)[0])
		case strings.Contains(title, "Token"):
			value = secret
		default:
			value = "FREEFORM-ANSWER"
		}
		response, _ := json.Marshal(map[string]any{"type": "extension_ui_response", "id": event["id"], "value": value})
		if _, err := fmt.Fprintf(fixture.stdin, "%s\n", response); err != nil {
			t.Fatal(err)
		}
		answered++
		if answered == 3 {
			break
		}
	}
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := strings.ReplaceAll(recorder.bodies[len(recorder.bodies)-1], `\"`, `"`)
	if !strings.Contains(last, "FREEFORM-ANSWER") || !regexp.MustCompile(`red-sec-[a-z2-7]{16}`).MatchString(last) {
		t.Fatalf("the result must carry the freeform text and a placeholder: %v", toolResults(t, recorder.bodies[len(recorder.bodies)-1]))
	}
	for _, body := range recorder.bodies {
		if strings.Contains(body, secret) {
			t.Fatal("the raw secret reached a provider request")
		}
	}
}

func TestLiveAskDialogTimeoutReportsTimedOutNeverADefaultAnswer(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[ask.remote]\ntimeout-ms = 300\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	form := map[string]any{"title": "Slow", "questions": []any{map[string]any{"id": "choice", "label": "Choice", "prompt": "Pick one", "type": "single", "options": []any{map[string]any{"value": "a", "label": "Option A"}}}}}
	recorder := &requestRecorder{calls: []scriptedCall{{"ask_user", form}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPC(t, root, "--model", "workbench-fixture/mock", "--tools", "ask_user")
	fixture.send("ask", "prompt", "Ask and nobody answers.")
	fixture.wait(func(event map[string]any) bool {
		return event["type"] == "extension_ui_request" && event["method"] == "select"
	})
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := strings.ReplaceAll(recorder.bodies[len(recorder.bodies)-1], `\"`, `"`)
	if !strings.Contains(last, "timedOut") || strings.Contains(last, `"answers":{"choice"`) {
		t.Fatalf("an elapsed dialog must report timedOut without an answer: %v", toolResults(t, recorder.bodies[len(recorder.bodies)-1]))
	}
}

func TestLiveAskInPrintModeIsUnavailable(t *testing.T) {
	root := t.TempDir()
	form := map[string]any{"questions": []any{map[string]any{"id": "choice", "label": "Choice", "prompt": "Pick one", "type": "single", "options": []any{map[string]any{"value": "a", "label": "Option A"}}}}}
	recorder := &requestRecorder{calls: []scriptedCall{{"ask_user", form}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	repo, _ := filepath.Abs("../..")
	cmd := exec.Command("pig", "--offline", "--no-session", "-e", repo, "--model", "workbench-fixture/mock", "--tools", "ask_user", "-p", "Ask the question.")
	cmd.Dir = root
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PIG_") && !strings.HasPrefix(v, "PI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+filepath.Join(root, "agent"), "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("print mode blocked on a dialog that cannot be shown")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) < 2 || !strings.Contains(recorder.bodies[len(recorder.bodies)-1], "interactionUnavailable") {
		t.Fatalf("print mode must return interactionUnavailable promptly, got %d requests", len(recorder.bodies))
	}
}
