//go:build integration

package host

import (
	"encoding/json"
	"fmt"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLiveProviderRequestsNeverCarryAKnownSecret(t *testing.T) {
	root := t.TempDir()
	fileToken := "ghp_" + strings.Repeat("Ab3dE9", 5)
	promptToken := "ghp_" + strings.Repeat("Zy8xW7", 5)
	commandToken := "ghp_" + strings.Repeat("Qw4eR5", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("deploy with "+fileToken+"\nthen stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "emit.sh"), []byte("echo "+commandToken+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": "notes.txt"}},
		{"bash", map[string]any{"command": "sh emit.sh"}},
		{"bash", map[string]any{"command": "sh emit.sh; sleep 30", "background": true}},
	}}
	recorder.next = func(index int) *scriptedCall {
		recorder.mu.Lock()
		last := recorder.bodies[len(recorder.bodies)-1]
		recorder.mu.Unlock()
		id := regexp.MustCompile(`Job ([0-9a-f]{4}-[0-9a-f]+): running`).FindStringSubmatch(last)
		switch {
		case index == 3 && id != nil:
			return &scriptedCall{"jobs", map[string]any{"action": "output", "id": id[1]}}
		case index == 4 && id != nil:
			return &scriptedCall{"read", map[string]any{"path": "job://" + id[1] + "/log"}}
		}
		return nil
	}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash,jobs")
	fixture.send("script", "prompt", "My token is "+promptToken+", use it carefully.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 6 {
		t.Fatalf("got %d provider requests, want 6", len(recorder.bodies))
	}
	placeholder := regexp.MustCompile(`red-sec-[a-z2-7]{16}`)
	for i, body := range recorder.bodies {
		for name, secret := range map[string]string{"file": fileToken, "prompt": promptToken, "command output": commandToken} {
			if strings.Contains(body, secret) {
				t.Fatalf("request %d carries the %s secret", i, name)
			}
		}
	}
	last := recorder.bodies[len(recorder.bodies)-1]
	var request struct {
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(last), &request); err != nil {
		t.Fatal(err)
	}
	var results []string
	for _, message := range request.Messages {
		if message["role"] == "tool" {
			results = append(results, toolMessageText(message))
		}
	}
	if len(results) != 5 {
		t.Fatalf("got %d tool results, want 5: %q", len(results), results)
	}
	for _, index := range []int{3, 4} {
		if !placeholder.MatchString(results[index]) {
			t.Fatalf("result %d did not carry the job log as a placeholder: %s", index, results[index])
		}
	}
	if got := len(placeholder.FindAllString(last, -1)); got < 3 {
		t.Fatalf("the final request should carry placeholders for all three secrets, found %d", got)
	}
}

func TestLiveToolReceivesTheRealValueWhileTheTranscriptKeepsThePlaceholder(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Mn6pL2", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("key="+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	placeholderOf := func() string {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		match := regexp.MustCompile(`red-sec-[a-z2-7]{16}`).FindString(recorder.bodies[len(recorder.bodies)-1])
		return match
	}
	recorder.calls = []scriptedCall{{"read", map[string]any{"path": "notes.txt"}}}
	recorder.next = func(index int) *scriptedCall {
		if index == 1 {
			return &scriptedCall{"bash", map[string]any{"command": "printf %s " + placeholderOf() + " > copied.txt"}}
		}
		return nil
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "Copy the key.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	copied, err := os.ReadFile(filepath.Join(root, "copied.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != token {
		t.Fatalf("the shell received %q instead of the real value", copied)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, token) {
			t.Fatalf("request %d carries the real token", i)
		}
	}
}

func TestLiveBashReceivesAnInjectedEnvSecretAndItsOutputStaysRedacted(t *testing.T) {
	testInjectedEnvSecret(t, true)
}

func TestLiveEnvInjectionOffKeepsVaultSecretsOutOfJobs(t *testing.T) {
	testInjectedEnvSecret(t, false)
}

func testInjectedEnvSecret(t *testing.T, injection bool) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "Workbench.toml"), fmt.Appendf(nil, "[secrets]\nenv-injection = %t\n", injection), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	writeFixtureProvider(t, agent, "http://127.0.0.1:1")
	settings, err := config.New(config.Paths{User: filepath.Join(agent, "Workbench.toml"), Project: filepath.Join(root, "project", "Workbench.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	boundary := newSecretsBoundary(settings, agent, func() string { return "" }, func() bool { return false }, func() string { return root })
	guard, _, err := boundary.Guard()
	if err != nil {
		t.Fatal(err)
	}
	value := "injected-service-token-9f3a"
	if err := guard.Learn([]byte(value), "token", "user"); err != nil {
		t.Fatal(err)
	}
	placeholder, _ := guard.Redact(value)
	if err := boundary.vault.SetMetadata(secrets.Record{Placeholder: placeholder, Kind: "token", Source: "user", Scope: "global", Env: "SERVICE_TOKEN"}); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"bash", map[string]any{"command": `printf %s "$SERVICE_TOKEN" > got.txt; echo "token=$SERVICE_TOKEN"`}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "bash")
	fixture.send("script", "prompt", "Print the service token variable.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	got, err := os.ReadFile(filepath.Join(root, "got.txt"))
	if !injection {
		if err != nil || len(got) != 0 {
			t.Fatalf("with secrets.env-injection off the command saw %q (%v), want an unset variable", got, err)
		}
		return
	}
	if err != nil || string(got) != value {
		t.Fatalf("the command saw %q (%v), want the injected value", got, err)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, value) {
			t.Fatalf("request %d carries the injected secret", i)
		}
	}
	if !strings.Contains(recorder.bodies[len(recorder.bodies)-1], placeholder) {
		t.Fatal("the command output should reach the model as the placeholder")
	}
}

func TestLiveBrokenSecretsPolicySendsNothingToTheProvider(t *testing.T) {
	root := t.TempDir()
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "Secrets.toml"), []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, agent, server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	token := "ghp_" + strings.Repeat("Hj7kL3", 5)
	fixture.send("script", "prompt", "Token "+token)
	time.Sleep(3 * time.Second)
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, token) {
			t.Fatalf("request %d carried the token although the boundary was broken", i)
		}
	}
	if len(recorder.bodies) != 0 {
		t.Fatalf("%d provider requests were sent while the secrets boundary was unavailable", len(recorder.bodies))
	}
}

func TestLiveABackgroundBashCallDoesNotTriggerTheWithheldNotice(t *testing.T) {
	l := startLiveSession(t, shortRuntime(t), []scriptedCall{{"bash", map[string]any{"command": "sleep 30", "background": true}}}, "--tools", "read,write,bash")
	l.fixture.send("work", "prompt", "start a job")
	var notices []string
	l.fixture.wait(func(record map[string]any) bool {
		if record["type"] == "extension_ui_request" && record["method"] == "notify" {
			notices = append(notices, fmt.Sprint(record["message"]))
		}
		return record["type"] == "agent_end"
	})
	for _, notice := range notices {
		if strings.Contains(notice, "Secrets boundary unavailable") {
			t.Fatalf("a normal tool call must not trip the secrets boundary: %s", notice)
		}
	}
}

// TestLiveWriteReceivesTheRealValueFromAPlaceholder: substitution is not bash-specific.
func TestLiveWriteReceivesTheRealValueFromAPlaceholder(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Kd3sP8", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("key="+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	placeholderOf := func() string {
		recorder.mu.Lock()
		defer recorder.mu.Unlock()
		return regexp.MustCompile(`red-sec-[a-z2-7]{16}`).FindString(recorder.bodies[len(recorder.bodies)-1])
	}
	recorder.calls = []scriptedCall{{"read", map[string]any{"path": "notes.txt"}}}
	recorder.next = func(index int) *scriptedCall {
		if index == 1 {
			return &scriptedCall{"write", map[string]any{"path": "copied.txt", "content": "token=" + placeholderOf() + "\n"}}
		}
		return nil
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,write")
	fixture.send("script", "prompt", "Copy the key with write.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	copied, err := os.ReadFile(filepath.Join(root, "copied.txt"))
	if err != nil || string(copied) != "token="+token+"\n" {
		t.Fatalf("write received %q, %v", copied, err)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, token) {
			t.Fatalf("request %d carries the real token", i)
		}
	}
}

// neverCarries fails when any provider request body holds the real token.
func neverCarries(t *testing.T, recorder *requestRecorder, token string) {
	t.Helper()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	for i, body := range recorder.bodies {
		if strings.Contains(body, token) {
			t.Fatalf("request %d carries the real token", i)
		}
	}
}

func lastPlaceholder(recorder *requestRecorder) string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return regexp.MustCompile(`red-sec-[a-z2-7]{16}`).FindString(recorder.bodies[len(recorder.bodies)-1])
}

// TestLiveEditSubstitutesAPlaceholderIntoTheFileAndKeepsTheTokenFromTheProvider covers edit, the
// tool with the largest model-written payload.
func TestLiveEditSubstitutesAPlaceholderIntoTheFileAndKeepsTheTokenFromTheProvider(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Ed7tQ4", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("key="+token+"\nname=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "notes.txt"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[edit]\nmode = 'replace'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder.next = func(index int) *scriptedCall {
		if index == 1 {
			return &scriptedCall{"edit", map[string]any{"path": "notes.txt", "edits": []any{map[string]any{"oldText": "name=old", "newText": "copy=" + lastPlaceholder(recorder)}}}}
		}
		return nil
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,edit")
	fixture.send("script", "prompt", "Copy the key with edit.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	got, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil || string(got) != "key="+token+"\ncopy="+token+"\n" {
		t.Fatalf("edit wrote %q, %v", got, err)
	}
	neverCarries(t, recorder, token)
}

// TestLiveHTTPBodyWithATokenIsRedactedBeforeTheProvider covers the http(s) reader path.
func TestLiveHTTPBodyWithATokenIsRedactedBeforeTheProvider(t *testing.T) {
	token := "ghp_" + strings.Repeat("Ht9sR2", 5)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><html><head><title>T</title></head><body><article><h1>Keys</h1><p>Use %s for deploys and keep this paragraph long enough to read as an article body.</p></article></body></html>", token)
	}))
	defer page.Close()
	root := t.TempDir()
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": page.URL + "/keys"}}, {"read", map[string]any{"path": page.URL + "/keys:raw"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("script", "prompt", "Read the page.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	neverCarries(t, recorder, token)
	if lastPlaceholder(recorder) == "" {
		t.Fatal("the page content reached the model without its placeholder")
	}
}

// TestLiveCodemodeChildResultsAreRedactedBeforeTheProvider: a script that prints what its nested
// read returned must not leak the file's secret.
func TestLiveCodemodeChildResultsAreRedactedBeforeTheProvider(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Cm5wN8", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("key="+token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"codemode", map[string]any{"code": "const r = await tools.read({path:'notes.txt'}); text(JSON.stringify(r));"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,codemode")
	fixture.send("script", "prompt", "Run the script.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	neverCarries(t, recorder, token)
	if lastPlaceholder(recorder) == "" {
		t.Fatal("the child result never reached the model, so the test proved nothing")
	}
}

// TestLiveAnotherExtensionsFetchBackendBodyIsRedactedBeforeTheProvider: a body served by a
// contributed fetch backend takes the same tool-result boundary as the builtin reader.
func TestLiveAnotherExtensionsFetchBackendBodyIsRedactedBeforeTheProvider(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Fb2kM6", 5)
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "https://fixture.test/secret-page"}}, {"read", map[string]any{"path": "https://fixture.test/secret-page:raw"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock", "--tools", "read")
	fixture.send("script", "prompt", "Read the page.")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	neverCarries(t, recorder, token)
	recorder.mu.Lock()
	last := recorder.bodies[len(recorder.bodies)-1]
	recorder.mu.Unlock()
	if !strings.Contains(last, "Deploy keys") || lastPlaceholder(recorder) == "" {
		t.Fatalf("the backend page did not reach the model with a placeholder:\n%s", last[max(0, len(last)-1800):])
	}
}

// TestLiveCompactionRequestCarriesThePlaceholderNotTheSecret captures the summarization request
// a real /compact sends: it holds the read's content and must hold only its placeholder.
func TestLiveCompactionRequestCarriesThePlaceholderNotTheSecret(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Cp4vB7", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("deploy key "+token+"\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "notes.txt"}}}}
	server := recorder.serve(t)
	defer server.Close()
	agent := filepath.Join(root, "agent")
	writeFixtureProvider(t, agent, server.URL)
	if err := os.WriteFile(filepath.Join(agent, "settings.json"), []byte(`{"compaction":{"enabled":true,"keepRecentTokens":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	for i, prompt := range []string{"read the notes", "again", "once more"} {
		fixture.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	}
	recorder.mu.Lock()
	before := len(recorder.bodies)
	recorder.mu.Unlock()
	fixture.sendRaw("compact", "compact", nil)
	fixture.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == "compact" })
	recorder.mu.Lock()
	after := len(recorder.bodies)
	var request string
	if after > before {
		request = recorder.bodies[before]
	}
	recorder.mu.Unlock()
	if request == "" {
		t.Fatal("the compaction made no provider request")
	}
	neverCarries(t, recorder, token)
	if !strings.Contains(request, "red-sec-") {
		t.Fatalf("the compaction request did not carry the read's content, so the test proved nothing:\n%.1500s", request)
	}
}

// TestLiveBranchSummaryRequestCarriesThePlaceholderNotTheSecret navigates the session tree with
// summarization. PiG's branch summary omits tool results, so the secret enters through a user
// message and the request must carry only its placeholder.
func TestLiveBranchSummaryRequestCarriesThePlaceholderNotTheSecret(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Bs9wD2", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("deploy key "+token+"\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"read", map[string]any{"path": "notes.txt"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, []string{"reference-producer"}, "--model", "workbench-fixture/mock", "--tools", "read")
	for i, prompt := range []string{"read the notes", "the deploy key is " + token, "once more"} {
		fixture.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	}
	fixture.sendRaw("forks", "get_fork_messages", nil)
	listed := fixture.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == "forks" })
	messages, _ := listed["data"].(map[string]any)["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("need user entries to leave a branch: %+v", listed)
	}
	target, _ := messages[0].(map[string]any)["entryId"].(string)
	recorder.mu.Lock()
	before := len(recorder.bodies)
	recorder.mu.Unlock()
	fixture.send("nav", "prompt", "/reference navigate "+target)
	fixture.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == "nav" })
	deadline := time.Now().Add(20 * time.Second)
	var request string
	for time.Now().Before(deadline) && request == "" {
		recorder.mu.Lock()
		for _, body := range recorder.bodies[before:] {
			if strings.Contains(body, "red-sec-") {
				request = body
				break
			}
		}
		recorder.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	if request == "" {
		t.Fatal("the branch summarization request carried no placeholder, so the test proved nothing")
	}
	neverCarries(t, recorder, token)
}

// TestLiveForkKeepsTheSessionFrameNonce forks a session and asserts every frame the model receives,
// before and after the fork, carries the same nonce (SPEC-0002 §6).
func TestLiveForkKeepsTheSessionFrameNonce(t *testing.T) {
	root := t.TempDir()
	recorder := &requestRecorder{}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read")
	for i, prompt := range []string{"one", "two", "three"} {
		fixture.send(fmt.Sprintf("p%d", i), "prompt", prompt)
		fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	}
	fixture.sendRaw("forks", "get_fork_messages", nil)
	listed := fixture.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == "forks" })
	messages, _ := listed["data"].(map[string]any)["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("need user entries to fork at: %+v", listed)
	}
	target, _ := messages[1].(map[string]any)["entryId"].(string)
	fixture.sendRaw("fork", "fork", map[string]any{"entryId": target})
	fixture.wait(func(event map[string]any) bool { return event["type"] == "response" && event["id"] == "fork" })
	fixture.send("after", "prompt", "four")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_settled" })
	nonces := make(map[string]int)
	recorder.mu.Lock()
	for _, body := range recorder.bodies {
		for _, match := range regexp.MustCompile(`n=\\"([0-9A-F]{8})\\"`).FindAllStringSubmatch(body, -1) {
			nonces[match[1]]++
		}
	}
	recorder.mu.Unlock()
	if len(nonces) != 1 {
		t.Fatalf("frames carry %d distinct nonces across a fork, want one: %v", len(nonces), nonces)
	}
}

// An env file outside the cwd chain is learned before the call that names it runs, so the
// result of that very call already carries placeholders (SPEC-0013 §2).
func TestLiveReferencedEnvFileOutsideTheCwdChainIsRedacted(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	readValue := "plainvalue-" + strings.Repeat("r3Ad", 6)
	shellValue := "plainvalue-" + strings.Repeat("sH3l", 6)
	if err := os.WriteFile(filepath.Join(outside, ".env"), []byte("API_KEY="+readValue+"\nMODE=fast\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "deploy.env"), []byte("export DEPLOY_TOKEN='"+shellValue+"'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recorder := &requestRecorder{calls: []scriptedCall{
		{"read", map[string]any{"path": filepath.Join(outside, ".env")}},
		{"bash", map[string]any{"command": "cat " + filepath.Join(outside, "deploy.env")}},
	}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	fixture := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,bash")
	fixture.send("script", "prompt", "read the env files")
	fixture.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.bodies) != 3 {
		t.Fatalf("got %d provider requests, want 3", len(recorder.bodies))
	}
	for i, body := range recorder.bodies {
		if strings.Contains(body, readValue) || strings.Contains(body, shellValue) {
			t.Fatalf("request %d carries an env-file value in plaintext", i)
		}
	}
	last := recorder.bodies[len(recorder.bodies)-1]
	if !strings.Contains(last, "API_KEY=red-sec-") || !strings.Contains(last, "MODE=fast") {
		t.Fatalf("the read result should show a placeholder for API_KEY and keep MODE: %s", last)
	}
	if !strings.Contains(last, "DEPLOY_TOKEN='red-sec-") {
		t.Fatalf("the bash result should show a placeholder for DEPLOY_TOKEN: %s", last)
	}
}
