package resource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const httpArticle = `<!doctype html>
<html><head><title>Reading architecture</title></head>
<body><nav>chrome</nav><article>
<h1>Reading architecture</h1>
<p>The reader extracts the main content of a page and resolves every reference against the complete page URL.</p>
<h2>Installation</h2>
<pre><code class="language-go">package main
</code></pre>
<p>Continue in the <a href="../guide.html">guide</a>.</p>
</article><footer>footer</footer></body></html>`

func httpTarget(t *testing.T, raw, selector string) Target {
	t.Helper()
	target, err := ParsePath("", raw+selector, "")
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func httpRead(t *testing.T, provider HTTPProvider, raw, selector string, budget Budget) ReadResult {
	t.Helper()
	target := httpTarget(t, raw, selector)
	result, err := provider.Read(context.Background(), ReadRequest{URI: target.URI, Requested: target.Requested, Selector: target.Selection, Budget: budget})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHTTPProviderRendersReadableAndRawHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, httpArticle)
	}))
	defer server.Close()
	readable := httpRead(t, HTTPProvider{}, server.URL+"/docs/intro/page.html", "", Budget{Bytes: 8192, Lines: 100})
	if readable.Facts.Handler != "builtin-html-main" {
		t.Fatalf("handler = %q", readable.Facts.Handler)
	}
	for _, want := range []string{"Reading architecture", "Installation", server.URL + "/docs/guide.html"} {
		if !strings.Contains(readable.Content, want) {
			t.Fatalf("readable view lacks %q:\n%s", want, readable.Content)
		}
	}
	if strings.Contains(readable.Content, "footer") || strings.Contains(readable.Content, "chrome") {
		t.Fatalf("readable view kept page chrome:\n%s", readable.Content)
	}
	raw := httpRead(t, HTTPProvider{}, server.URL+"/docs/intro/page.html", ":raw", Budget{Bytes: 8192, Lines: 100})
	if raw.Facts.Handler != "builtin-html-raw" || !strings.Contains(raw.Content, "<article>") || !strings.HasPrefix(raw.Content, "<!doctype html>") {
		t.Fatalf("raw view = %q, handler %q", raw.Content, raw.Facts.Handler)
	}
	paged := httpRead(t, HTTPProvider{}, server.URL+"/docs/intro/page.html", ":2-3", Budget{Bytes: 1024, Lines: 20})
	if paged.Content == "" || paged.Facts.TotalLines == 0 || !strings.Contains(paged.Content, "2:") {
		t.Fatalf("selector read = %#v", paged.Facts)
	}
}

func TestHTTPProviderTracksRedirectsAndRejectsCycles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old":
			http.Redirect(w, r, "/new", http.StatusFound)
		case "/new":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "moved content\n")
		case "/a":
			http.Redirect(w, r, "/b", http.StatusFound)
		case "/b":
			http.Redirect(w, r, "/a", http.StatusFound)
		case "/escape":
			w.Header().Set("Location", "file:///etc/passwd")
			w.WriteHeader(http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result := httpRead(t, HTTPProvider{}, server.URL+"/old", "", Budget{Bytes: 4096, Lines: 50})
	if !strings.Contains(result.Content, "moved content") || result.Facts.CanonicalURI != server.URL+"/new" {
		t.Fatalf("redirect = %#v", result)
	}
	if len(result.Facts.Notices) == 0 || !strings.Contains(result.Facts.Notices[0], "followed HTTP redirects") {
		t.Fatalf("redirect notice = %#v", result.Facts.Notices)
	}
	for _, path := range []string{"/a", "/escape"} {
		target := httpTarget(t, server.URL+path, "")
		if _, err := (HTTPProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: target.Requested, Selector: target.Selection, Budget: Budget{Bytes: 4096, Lines: 50}}); err == nil {
			t.Fatalf("%s: unsafe redirect was accepted", path)
		}
	}
}

func TestHTTPProviderLimitsDecodesAndRefusesUnsupportedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/large":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, strings.Repeat("x", 256))
		case "/binary":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{0x89, 'P', 'N', 'G'})
		case "/latin1":
			w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
			w.Write([]byte("caf\xe9\n"))
		case "/status":
			http.Error(w, "gone", http.StatusGone)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target := httpTarget(t, server.URL+"/large", "")
	if _, err := (HTTPProvider{MaxBytes: 32}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: target.Requested, Selector: target.Selection, Budget: Budget{Bytes: 4096, Lines: 50}}); !errors.Is(err, ErrHTTPTooLarge) {
		t.Fatalf("large response = %v; want ErrHTTPTooLarge", err)
	}
	for _, path := range []string{"/binary", "/status"} {
		target = httpTarget(t, server.URL+path, "")
		if _, err := (HTTPProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: target.Requested, Selector: target.Selection, Budget: Budget{Bytes: 4096, Lines: 50}}); err == nil {
			t.Fatalf("%s: accepted", path)
		}
	}
	decoded := httpRead(t, HTTPProvider{}, server.URL+"/latin1", "", Budget{Bytes: 4096, Lines: 50})
	if !strings.Contains(decoded.Content, "café") {
		t.Fatalf("charset decoding = %q", decoded.Content)
	}
}

func TestHTTPProviderRefusesCredentialsForeignSchemesAndUnnormalizedOrigins(t *testing.T) {
	for _, raw := range []string{"http://user:pass@example.com/", "https://example.com/"} {
		target, err := ParsePath("", raw, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (HTTPProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: target.Requested, Selector: target.Selection, Budget: Budget{Bytes: 4096, Lines: 50}}); err == nil {
			t.Fatalf("%q was accepted", raw)
		}
	}
	// ParsePath normalizes an origin to "/"; a caller that bypasses it is still refused before any request.
	if _, err := (HTTPProvider{}).Read(context.Background(), ReadRequest{URI: "http://example.com", Requested: "http://example.com", Budget: Budget{Bytes: 4096, Lines: 50}}); err == nil {
		t.Fatal("an unnormalized origin was accepted")
	}
}

func TestHTTPProviderReportsStaticRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><meta http-equiv="refresh" content="0; url=/new/home"></head><body></body></html>`)
	}))
	defer server.Close()
	result := httpRead(t, HTTPProvider{}, server.URL+"/moved", "", Budget{Bytes: 4096, Lines: 50})
	if result.Facts.Handler != "builtin-html-redirect" || !strings.Contains(result.Content, "Static redirect reported") || !strings.Contains(strings.Join(result.Facts.Notices, " "), "static redirect") {
		t.Fatalf("static redirect = %#v", result)
	}
}

func TestHTTPOriginWithoutAPathReadsItsRootDocument(t *testing.T) {
	var requested string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "root document\n")
	}))
	defer server.Close()
	target, err := ParsePath(t.TempDir(), server.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	if target.URI != server.URL+"/" {
		t.Fatalf("target = %+v", target)
	}
	result := httpRead(t, HTTPProvider{}, server.URL, "", Budget{Bytes: 8192, Lines: 100})
	if requested != "/" || !strings.Contains(result.Content, "root document") {
		t.Fatalf("requested %q, content %q", requested, result.Content)
	}
	if query, err := ParsePath(t.TempDir(), "https://example.test?q=1", ""); err != nil || query.URI != "https://example.test/?q=1" {
		t.Fatalf("query on a bare origin = %+v, %v", query, err)
	}
	if bare, err := ParsePath(t.TempDir(), "skill://name", ""); err != nil || bare.URI != "skill://name" {
		t.Fatalf("other schemes must keep an empty path: %+v, %v", bare, err)
	}
}
