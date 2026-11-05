package api

import (
	"encoding/json"
	"errors"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

// TestAuthoringSurfaceCompiles pins the documented authoring API against the
// staged SDK: a producer registers, serves and withdraws a contribution using
// only the symbols documented in docs/extension-services.md (SPEC-0001 §6).
func TestAuthoringSurfaceCompiles(t *testing.T) {
	producer := func(ext *sdk.Extension, ctx sdk.Context, endpoint Discovery) error {
		client, err := NewClient(ext)
		if err != nil {
			return err
		}
		defer client.Close()
		if client.Identity().Owner != ext.Name() || client.Identity().Instance == "" {
			t.Fatal("client identity does not name the extension")
		}
		stop, err := client.Serve(ext, func(_ sdk.Context, request ProviderRequest) (any, *Error) {
			switch request.Method {
			case "resource.execute":
				var params ResourceRequest
				if err := json.Unmarshal(request.Params, &params); err != nil {
					return nil, &Error{Code: "invalidRequest", Message: "bad resource request"}
				}
				complete := true
				return ResourceResult{
					CanonicalURI: params.URI,
					Kind:         "file",
					Revision:     "r1",
					Page: &TextPage{
						Text:              "alpha\n",
						ShownRanges:       []LineRange{{Start: 1, End: 1}},
						SelectionComplete: true,
						ResourceComplete:  false,
					},
					Complete: &complete,
				}, nil
			case "fetch":
				var params FetchRequest
				if err := json.Unmarshal(request.Params, &params); err != nil {
					return nil, &Error{Code: "invalidRequest", Message: "bad fetch request"}
				}
				body := "<html></html>"
				return FetchResult{Body: &body, FinalURL: params.URL, Status: 200, ContentType: "text/html"}, nil
			default:
				return nil, &Error{Code: "unsupported", Message: "method is not provided"}
			}
		})
		if err != nil {
			return err
		}
		defer stop()
		receipt, err := client.Register(ctx, endpoint, "resourceProvider", "kb", map[string]any{"schemes": []string{"kb"}})
		if err != nil {
			return err
		}
		if receipt.RegistrationID == "" || receipt.Kind != "resourceProvider" || receipt.Key != "kb" {
			t.Fatalf("receipt does not describe the registration: %+v", receipt)
		}
		var described struct {
			Registrations []struct {
				RegistrationID string `json:"registrationId"`
				Kind           string `json:"kind"`
				Key            string `json:"key"`
			} `json:"registrations"`
			NextCursor string `json:"nextCursor"`
		}
		if err := client.Call(ctx, endpoint, "describe", map[string]any{"kind": "resourceProvider"}, &described, nil); err != nil {
			return err
		}
		if err := client.Unregister(ctx, endpoint, receipt.RegistrationID); err != nil {
			return err
		}
		return nil
	}
	if producer == nil {
		t.Fatal("unreachable")
	}
}

// TestDocumentedWireNames pins the JSON names a producer and consumer depend on.
func TestDocumentedWireNames(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  []string
	}{
		{"discovery", Discovery{Major: 1, Minor: 0, WorkbenchVersion: "development", Instance: "wb", SessionID: "s", Generation: "g", Capabilities: []string{"resources"}, Limits: Limits{ControlBytes: ControlBytes, PageBytes: 51200, PageLines: 2000, InFlightPerOwner: 8}},
			[]string{`"major":1`, `"minor":0`, `"workbenchVersion"`, `"instance"`, `"sessionId"`, `"generation"`, `"capabilities"`, `"controlBytes"`, `"pageBytes"`, `"pageLines"`, `"inFlightPerOwner"`}},
		{"receipt", Receipt{RegistrationID: "r", DescriptorRevision: "d", Kind: "resourceProvider", Key: "kb", Idempotent: true},
			[]string{`"registrationId"`, `"descriptorRevision"`, `"kind"`, `"key"`, `"idempotent"`}},
		{"resource request", ResourceRequest{OperationID: "o", Operation: "read", RequestedURI: "kb://a", URI: "kb://a", Selector: &Selector{Kind: "lines", Ranges: []LineRange{{Start: 1, End: 2}}, Raw: true}, Cursor: "c", Limits: PageLimits{Bytes: 1, Lines: 2}, Project: &Project{ID: "prj", Slug: "app"}},
			[]string{`"operationId"`, `"operation"`, `"requestedUri"`, `"uri"`, `"selector"`, `"ranges"`, `"raw"`, `"cursor"`, `"limits"`, `"project"`, `"slug"`}},
		{"resource result", ResourceResult{CanonicalURI: "kb://a", Kind: "file", Revision: "r", Size: 3, Lines: 1, Entries: []ResourceEntry{{Name: "a", URI: "kb://a", Kind: "file", Target: "x"}}, NextCursor: "n"},
			[]string{`"canonicalUri"`, `"kind"`, `"revision"`, `"entries"`, `"target"`, `"nextCursor"`}},
		{"text page", TextPage{Text: "a", ShownRanges: []LineRange{{Start: 1, End: 1}}, TotalLines: new(int), SelectionComplete: true, ResourceComplete: false},
			[]string{`"text"`, `"shownRanges"`, `"totalLines"`, `"selectionComplete"`, `"resourceComplete"`}},
		{"fetch result", FetchResult{Body: new(string), Locate: "kb://a", FinalURL: "https://a", Redirects: []string{"https://b"}, Status: 200, ContentType: "text/html", Charset: "utf-8", Revision: "r", Handler: "fixture", UsedThirdParty: true, Notices: []string{"n"}},
			[]string{`"body"`, `"locate"`, `"finalUrl"`, `"redirects"`, `"status"`, `"contentType"`, `"charset"`, `"revision"`, `"handler"`, `"usedThirdParty"`, `"notices"`}},
		{"protocol error", Error{Code: "conflict", Message: "m", Retryable: true, Field: "f", Outcome: "notApplied"},
			[]string{`"code"`, `"message"`, `"retryable"`, `"field"`, `"outcome"`}},
	}
	for _, c := range cases {
		encoded, err := json.Marshal(c.value)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for _, want := range c.want {
			if !contains(string(encoded), want) {
				t.Errorf("%s lacks %s: %s", c.name, want, encoded)
			}
		}
	}
}

// TestTransportOutcomeClassification pins the retry guidance a consumer relies on.
func TestTransportOutcomeClassification(t *testing.T) {
	notApplied := &TransportError{RequestID: "r1", Outcome: "notApplied"}
	if errors.As(notApplied, new(*Error)) || notApplied.Outcome != "notApplied" {
		t.Fatal("a request that never emitted must be safe to retry")
	}
	unknown := &TransportError{RequestID: "r2", Outcome: "unknown", cause: ErrUnavailable}
	if !errors.Is(unknown, ErrUnavailable) || unknown.Outcome != "unknown" {
		t.Fatal("a lost emitted request must stay unknown")
	}
	for _, err := range []error{ErrUnavailable, ErrAmbiguous, ErrClosed} {
		if err.Error() == "" {
			t.Fatalf("%v has no guidance text", err)
		}
	}
}
