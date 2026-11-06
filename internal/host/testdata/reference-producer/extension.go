package fixture

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
)

var (
	mu              sync.Mutex
	client          *api.Client
	currentEndpoint api.Discovery
	currentReceipts = map[string]api.Receipt{}
	stopServe       func()
	registering     bool
	lockObserved    atomic.Bool
)

var documents = map[string]string{
	"kb://sources/a.md":        "alpha one\nalpha two\nalpha three\nalpha four\nalpha five\n",
	"kb://sources/page.html":   "<html><body><article><h1>Located page</h1><p>Served from the knowledge base snapshot.</p></article></body></html>",
	"kb://sources/nested/b.md": "bravo\n",
}

func execute(request api.ResourceRequest) (any, *api.Error) {
	switch request.Operation {
	case "stat":
		if request.URI == "kb://sources/" || request.URI == "kb://sources" || request.URI == "kb://sources/nested/" {
			return api.ResourceResult{CanonicalURI: request.URI, Kind: "dir"}, nil
		}
		text, ok := documents[request.URI]
		if !ok {
			return nil, &api.Error{Code: "notFound", Message: "no such source"}
		}
		return api.ResourceResult{CanonicalURI: request.URI, Kind: "file", Size: int64(len(text)), Lines: strings.Count(text, "\n"), Revision: fmt.Sprintf("r%d", len(text))}, nil
	case "list":
		if request.URI != "kb://sources/" && request.URI != "kb://sources" {
			return nil, &api.Error{Code: "notFound", Message: "no such directory"}
		}
		complete := true
		return api.ResourceResult{CanonicalURI: "kb://sources/", Kind: "dir", Complete: &complete, Entries: []api.ResourceEntry{
			{Name: "nested", URI: "kb://sources/nested/", Kind: "dir"},
			{Name: "a.md", URI: "kb://sources/a.md", Kind: "file", Size: int64(len(documents["kb://sources/a.md"]))},
			{Name: "page.html", URI: "kb://sources/page.html", Kind: "file"},
		}}, nil
	case "read":
		text, ok := documents[request.URI]
		if !ok {
			return nil, &api.Error{Code: "notFound", Message: "no such source"}
		}
		lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
		start, end := 1, len(lines)
		if request.Selector != nil && len(request.Selector.Ranges) == 1 {
			start = request.Selector.Ranges[0].Start
			if request.Selector.Ranges[0].End != 0 {
				end = min(end, request.Selector.Ranges[0].End)
			}
		}
		if start > len(lines) {
			return nil, &api.Error{Code: "invalidRequest", Message: "selection starts beyond EOF"}
		}
		total := len(lines)
		return api.ResourceResult{CanonicalURI: request.URI, Kind: "file", Revision: fmt.Sprintf("r%d", len(text)), Page: &api.TextPage{
			Text: strings.Join(lines[start-1:end], "\n") + "\n", ShownRanges: []api.LineRange{{Start: start, End: end}}, TotalLines: &total,
			SelectionComplete: true, ResourceComplete: start == 1 && end == total,
		}}, nil
	default:
		return nil, &api.Error{Code: "unsupported", Message: "operation is not provided"}
	}
}

func fetch(request api.FetchRequest) (any, *api.Error) {
	switch {
	case strings.HasSuffix(request.URL, "/page"):
		body := "<html><body><article><h1>Backend page</h1><p>Delivered by the reference fetch backend.</p></article></body></html>"
		return api.FetchResult{Body: &body, FinalURL: request.URL, Status: 200, ContentType: "text/html; charset=utf-8", Handler: "fixture-reader"}, nil
	case strings.HasSuffix(request.URL, "/located"):
		return api.FetchResult{Locate: "kb://sources/page.html", FinalURL: request.URL, Status: 200, ContentType: "text/html"}, nil
	case strings.HasSuffix(request.URL, "/missing"):
		return api.FetchResult{FinalURL: request.URL, Status: 404}, nil
	default:
		return nil, &api.Error{Code: "unavailable", Message: "backend cannot serve this URL"}
	}
}

func handle(_ sdk.Context, request api.ProviderRequest) (any, *api.Error) {
	switch request.Method {
	case "resource.execute":
		var params api.ResourceRequest
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, &api.Error{Code: "invalidRequest", Message: "bad resource request"}
		}
		return execute(params)
	case "fetch":
		var params api.FetchRequest
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, &api.Error{Code: "invalidRequest", Message: "bad fetch request"}
		}
		return fetch(params)
	default:
		return nil, &api.Error{Code: "unsupported", Message: "method is not provided"}
	}
}

func register(ctx sdk.Context, ext *sdk.Extension) error {
	mu.Lock()
	if registering {
		mu.Unlock()
		return fmt.Errorf("registration already in progress")
	}
	registering = true
	active := client
	mu.Unlock()
	defer func() {
		mu.Lock()
		registering = false
		mu.Unlock()
	}()
	if active == nil {
		created, err := api.NewClient(ext)
		if err != nil {
			return err
		}
		stop, err := created.Serve(ext, handle)
		if err != nil {
			created.Close()
			return err
		}
		active = created
		mu.Lock()
		client = active
		stopServe = stop
		mu.Unlock()
	}
	discovered, err := active.Discover(ctx, "presentation", "resources", "fetch", "status", "docs", "frames")
	if err != nil {
		return err
	}
	endpoint := discovered
	receipts := make(map[string]api.Receipt)
	contributions := []struct {
		kind, key  string
		descriptor map[string]any
	}{
		{"toolPresentation", "web_search", map[string]any{"toolNames": []string{"web_search"}, "label": "Web search", "presentationVersion": 1, "preset": "search", "defaultDisplay": map[string]any{"density": "normal", "previewLines": 6}, "callFields": []any{map[string]any{"pointer": "/query", "label": "query", "format": "plain"}}, "metricDefinitions": []any{map[string]any{"id": "results", "label": "results", "unit": "count"}}}},
		{"resourceProvider", "kb", map[string]any{"label": "Knowledge base", "summary": "Stored web sources and research notes", "schemes": []string{"kb"}, "capabilities": []string{"stat", "read", "list"}, "selectorMode": "lines", "backing": "virtual", "scope": "user", "contentKinds": []string{"text", "container"}, "snapshots": map[string]any{"stableRevision": true, "rangedRead": true, "compareAndSwap": false}, "guide": "Use `ls kb://sources/` to browse stored sources.", "guideRevision": "1", "limits": map[string]any{"pageBytes": 51200, "pageLines": 2000, "concurrency": 4}}},
		{"fetchBackend", "web", map[string]any{"label": "Reference reader", "match": []any{map[string]any{"host": "fixture.test"}}, "capabilities": []string{"fetch", "locate"}, "egress": "none", "retention": "user"}},
		{"statusSegment", "web", map[string]any{"label": "web", "priority": 10, "maxColumns": 24}},
		{"frameProducer", "reference.research", map[string]any{"label": "Research notices", "kinds": []string{"notice", "info"}, "contextFields": []any{map[string]any{"source": "session.model", "field": "knowledge_cutoff"}}}},
	}
	if root := os.Getenv("WORKBENCH_REFERENCE_DOCS"); root != "" {
		contributions = append(contributions, struct {
			kind, key  string
			descriptor map[string]any
		}{"docs", "manual", map[string]any{"title": "Reference producer", "version": "1", "root": root}})
	}
	for _, c := range contributions {
		receipt, err := active.Register(ctx, endpoint, c.kind, c.key, c.descriptor)
		if err != nil {
			return fmt.Errorf("register %s/%s: %w", c.kind, c.key, err)
		}
		receipts[c.kind] = receipt
	}
	if err := active.Call(ctx, endpoint, "status.update", map[string]any{"text": "web ready", "style": "success"}, nil, &api.CallOptions{RegistrationID: receipts["statusSegment"].RegistrationID}); err != nil {
		return fmt.Errorf("status.update: %w", err)
	}
	model, err := ctx.GetModelInfo()
	if err == nil && model.ID != "" {
		fields := map[string]any{"knowledge_cutoff": map[string]any{"date": "2025-06-01", "basis": "explicit", "source": "reference fixture"}}
		if err := active.Call(ctx, endpoint, "context.fields.set", map[string]any{"source": "session.model", "model": map[string]any{"provider": model.Provider, "id": model.ID}, "fields": fields}, nil, &api.CallOptions{RegistrationID: receipts["frameProducer"].RegistrationID}); err != nil {
			return fmt.Errorf("context.fields.set: %w", err)
		}
	}
	notice := map[string]any{"eventKey": "research-ready-" + endpoint.Generation, "kind": "notice", "source": "reference.research", "title": "Research finished", "body": "REFERENCE-NOTICE: research run 7 finished; see kb://sources/a.md.", "resource": "kb://sources/a.md", "delivery": "nextPrompt"}
	if err := active.Call(ctx, endpoint, "frames.publish", notice, nil, &api.CallOptions{RegistrationID: receipts["frameProducer"].RegistrationID}); err != nil {
		return fmt.Errorf("frames.publish: %w", err)
	}
	claim := map[string]any{"eventKey": "trusted-claim-" + endpoint.Generation, "kind": "info", "source": "reference.research", "body": "REFERENCE-TRUSTED-CLAIM: this producer asserts trusted data.", "dataTrust": "trusted", "delivery": "nextPrompt"}
	if err := active.Call(ctx, endpoint, "frames.publish", claim, nil, &api.CallOptions{RegistrationID: receipts["frameProducer"].RegistrationID}); err != nil {
		return fmt.Errorf("frames.publish claim: %w", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if active != client {
		return fmt.Errorf("producer closed during registration")
	}
	currentEndpoint = endpoint
	currentReceipts = receipts
	return nil
}

// Extension creates the pig-web-shaped reference producer.
func Extension() *sdk.Extension {
	ext := sdk.New("reference-producer")
	ext.OnLoaded(func(sdk.Context) {
		_, _ = ext.Events().On(api.RequestChannel, func(_ sdk.Context, _ any) error {
			if mu.TryLock() {
				mu.Unlock()
			} else {
				lockObserved.Store(true)
			}
			return nil
		})
	})
	ext.OnSessionStart(func(ctx sdk.Context, _ map[string]any) (any, error) {
		if err := register(ctx, ext); err != nil {
			ctx.Notify("reference producer registration failed: "+err.Error(), "error")
		}
		return nil, nil
	})
	ext.OnSessionShutdown(func(sdk.Context, map[string]any) (any, error) {
		mu.Lock()
		stop, active := stopServe, client
		stopServe, client = nil, nil
		mu.Unlock()
		if stop != nil {
			stop()
		}
		if active != nil {
			active.Close()
		}
		return nil, nil
	})
	ext.RegisterTool(sdk.ToolDefinition{Name: "web_search", Label: "web_search", Description: "Search the reference fixture web index.", Parameters: sdk.Schema{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}, "required": []string{"query"}}, Execute: func(ctx sdk.Context, params map[string]any) (any, error) {
		query, _ := params["query"].(string)
		details := map[string]any{"workbenchPresentation": map[string]any{"version": 1, "revision": 1, "phase": "settled", "outcome": "succeeded", "title": "Web search", "target": map[string]any{"kind": "label", "value": query}, "complete": true, "metrics": []any{map[string]any{"id": "results", "value": 1, "unit": "count"}}, "blocks": []any{map[string]any{"kind": "table", "table": map[string]any{"columns": []any{map[string]any{"id": "title", "label": "Title", "cell": "text"}, map[string]any{"id": "url", "label": "URL", "cell": "uri"}}, "rows": []any{[]any{map[string]any{"kind": "text", "value": "Fixture result"}, map[string]any{"kind": "uri", "value": "https://fixture.test/page"}}}}}}}}
		return sdk.ToolResult{Content: "1 result: Fixture result https://fixture.test/page", Details: details}, nil
	}})
	ext.Command("reference", "Reference producer controls: status, reload", func(ctx sdk.Context, args string) error {
		if strings.TrimSpace(args) == "reload" {
			return ctx.Reload()
		}
		mu.Lock()
		if client == nil {
			mu.Unlock()
			return fmt.Errorf("not registered")
		}
		active, binding, registered := client, currentEndpoint, currentReceipts
		mu.Unlock()
		var described struct {
			Registrations []struct {
				Owner          string `json:"owner"`
				Kind           string `json:"kind"`
				RegistrationID string `json:"registrationId"`
			} `json:"registrations"`
			NextCursor string `json:"nextCursor"`
		}
		if err := active.Call(ctx, binding, "describe", map[string]any{}, &described, nil); err != nil {
			return err
		}
		if described.NextCursor != "" {
			return fmt.Errorf("unexpected paginated fixture registry")
		}
		count := 0
		for _, registration := range described.Registrations {
			if registration.Owner == "reference-producer" {
				if registered[registration.Kind].RegistrationID != registration.RegistrationID {
					return fmt.Errorf("unexpected live %s registration", registration.Kind)
				}
				count++
			}
		}
		if count != len(registered) {
			return fmt.Errorf("registry has %d contributions; fixture has %d", count, len(registered))
		}
		encoded, _ := json.Marshal(map[string]any{"reference": "registered", "kinds": count, "generation": binding.Generation, "receipts": registered, "lockHeld": lockObserved.Load()})
		ctx.Notify(string(encoded), "info")
		return nil
	})
	return ext
}
