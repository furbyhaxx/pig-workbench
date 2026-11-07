package resource

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const kbDescriptor = `{"label":"KB","summary":"Stored sources","schemes":["kb","knowledge"],"capabilities":["stat","read","list"],"selectorMode":"lines","backing":"virtual","scope":"user","contentKinds":["text","container"],"snapshots":{"stableRevision":true,"rangedRead":true,"compareAndSwap":false},"limits":{}}`

func TestParseProviderDescriptor(t *testing.T) {
	d, err := ParseProviderDescriptor([]byte(kbDescriptor))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Schemes) != 2 || !d.Has("list") || d.Has("write") {
		t.Fatalf("descriptor = %#v", d)
	}
	cases := map[string]string{
		"reserved scheme":    strings.Replace(kbDescriptor, `["kb","knowledge"]`, `["file"]`, 1),
		"retired scheme":     strings.Replace(kbDescriptor, `["kb","knowledge"]`, `["skills"]`, 1),
		"bad scheme":         strings.Replace(kbDescriptor, `["kb","knowledge"]`, `["KB"]`, 1),
		"no stat":            strings.Replace(kbDescriptor, `["stat","read","list"]`, `["read","list"]`, 1),
		"edit without write": strings.Replace(kbDescriptor, `["stat","read","list"]`, `["stat","read","list","edit"]`, 1),
		"write no mutations": strings.Replace(kbDescriptor, `["stat","read","list"]`, `["stat","read","list","write"]`, 1),
		"container no list":  strings.Replace(kbDescriptor, `["stat","read","list"]`, `["stat","read"]`, 1),
		"long summary":       strings.Replace(kbDescriptor, `"Stored sources"`, `"`+strings.Repeat("x", 121)+`"`, 1),
		"control summary":    strings.Replace(kbDescriptor, `"Stored sources"`, `"a\u001b[2Jb"`, 1),
		"unknown field":      strings.Replace(kbDescriptor, `"label":"KB"`, `"label":"KB","exec":"x"`, 1),
	}
	for name, payload := range cases {
		if _, err := ParseProviderDescriptor([]byte(payload)); !errors.Is(err, ErrInvalidDescriptor) {
			t.Errorf("%s: accepted or wrong error: %v", name, err)
		}
	}
}

func TestMutationDescriptorRequiresRevisionChecksAndRetention(t *testing.T) {
	valid := strings.Replace(kbDescriptor, `"capabilities":["stat","read","list"]`, `"capabilities":["stat","read","list","write","mkdir","copy","move","remove"],"mutations":{"revisionChecked":true,"retentionMs":60000,"durable":false}`, 1)
	valid = strings.Replace(valid, `"compareAndSwap":false`, `"compareAndSwap":true`, 1)
	if _, err := ParseProviderDescriptor([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{strings.Replace(valid, `"revisionChecked":true`, `"revisionChecked":false`, 1), strings.Replace(valid, `"retentionMs":60000`, `"retentionMs":0`, 1), strings.Replace(valid, `"compareAndSwap":true`, `"compareAndSwap":false`, 1), strings.Replace(valid, `"durable":false`, `"durable":false,"confirm":true`, 1)} {
		if _, err := ParseProviderDescriptor([]byte(payload)); !errors.Is(err, ErrInvalidDescriptor) {
			t.Fatalf("invalid mutation descriptor accepted: %s %v", payload, err)
		}
	}
}

func TestRegisterContributedIsAtomic(t *testing.T) {
	router := NewRouter()
	if err := router.RegisterBuiltin(FileProvider{}); err != nil {
		t.Fatal(err)
	}
	first := []Provider{schemeOnly("kb"), schemeOnly("knowledge")}
	if err := router.RegisterContributed(first); err != nil {
		t.Fatal(err)
	}
	if err := router.RegisterContributed([]Provider{schemeOnly("other"), schemeOnly("kb")}); err == nil {
		t.Fatal("colliding alias was accepted")
	}
	if strings.Contains(strings.Join(router.Schemes(), ","), "other") {
		t.Fatal("failed registration left a partial scheme behind")
	}
	router.UnregisterContributed([]string{"kb", "knowledge", "file"})
	if got := strings.Join(router.Schemes(), ","); got != "file" {
		t.Fatalf("schemes after unregister = %s", got)
	}
}

func TestRenderSpansValidatesProviderPages(t *testing.T) {
	budget := Budget{Bytes: 4096, Lines: 100}
	page, err := RenderSpans(context.Background(), "kb://a", "kb://a:2-3", []Range{{2, 3}}, []string{"two", "three"}, 5, false, budget)
	if err != nil || !strings.Contains(page.Content, "2:two\n3:three") || !page.SelectionComplete || page.TotalLines != 5 {
		t.Fatalf("page = %#v, %v", page, err)
	}
	if _, err := RenderSpans(context.Background(), "kb://a", "kb://a", []Range{{2, 3}}, []string{"only one"}, 0, false, budget); err == nil {
		t.Fatal("line count mismatch was accepted")
	}
	if _, err := RenderSpans(context.Background(), "kb://a", "kb://a", []Range{{3, 4}, {2, 2}}, []string{"a", "b", "c"}, 0, false, budget); err == nil {
		t.Fatal("unordered spans were accepted")
	}
}

type schemeOnly string

func (s schemeOnly) Scheme() string { return string(s) }
func (schemeOnly) Stat(context.Context, string) (StatResult, error) {
	return StatResult{}, ErrUnsupported
}
func (schemeOnly) List(context.Context, string, string, int) (Listing, error) {
	return Listing{}, ErrUnsupported
}
func (schemeOnly) Read(context.Context, ReadRequest) (ReadResult, error) {
	return ReadResult{}, ErrUnsupported
}

type stubBackends struct {
	answer  BackendFetch
	matched bool
	err     error
}

func (b stubBackends) Fetch(context.Context, string, int) (BackendFetch, bool, error) {
	return b.answer, b.matched, b.err
}

func TestHTTPBackendDefinitiveAnswerDoesNotFallBack(t *testing.T) {
	provider := HTTPProvider{Secure: true, Backends: stubBackends{matched: true, answer: BackendFetch{FinalURI: "https://example.test/x", Status: 410}}}
	_, err := provider.Read(context.Background(), ReadRequest{URI: "https://example.test/x", Requested: "https://example.test/x", Budget: Budget{Bytes: 4096, Lines: 100}})
	if !errors.Is(err, ErrBackendDefinitive) {
		t.Fatalf("410 from backend = %v; want definitive", err)
	}
	provider.Backends = stubBackends{matched: true, answer: BackendFetch{Body: []byte("plain"), ContentType: "text/plain", FinalURI: "ftp://example.test/x", Status: 200}}
	_, err = provider.Read(context.Background(), ReadRequest{URI: "https://example.test/x", Requested: "https://example.test/x", Budget: Budget{Bytes: 4096, Lines: 100}})
	if err == nil {
		t.Fatal("backend redirect outside http(s) fell through silently as success")
	}
}
