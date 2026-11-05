package htmlread

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

const pageURL = "https://example.com/docs/intro/page.html"

const articlePage = `<!doctype html>
<html>
<head>
<title>Reading architecture — Example docs</title>
<style>body { margin: 0; }</style>
<script>console.log("analytics beacon")</script>
</head>
<body>
<nav><a href="/">Home</a> <a href="/docs">Docs</a> <a href="/login">Sign in</a></nav>
<article class="markdown-body">
<h1>Reading architecture</h1>
<p>The reader extracts the main content of a page. It resolves every reference against the complete page URL, so links keep working outside the browser.</p>
<h2>Installation</h2>
<pre><code class="language-go">package main

func main() {
	println("hi")
}
</code></pre>
<table>
<thead><tr><th>Flag</th><th>Meaning</th></tr></thead>
<tbody><tr><td>-race</td><td>enables the race detector</td></tr></tbody>
</table>
<p>Continue in the <a href="../guide.html">guide</a>.</p>
<p><img src="images/architecture.png" alt="Architecture diagram"></p>
<table>
<tr><td>Left cell text</td><td>Right cell text</td></tr>
</table>
</article>
<footer><p>Footer text</p></footer>
</body>
</html>`

const fallbackPage = `<!doctype html>
<html>
<head><title>Fallback story</title></head>
<body>
<div id="wrapper">
<div class="post">
<h1>Fallback story</h1>
<p>Readability needs enough prose before it accepts a candidate block as the article. This paragraph exists so the fallback has a realistic amount of text to weigh against the surrounding chrome, which on real pages is often heavier than the article itself.</p>
<p>A second paragraph keeps the candidate score high enough to be selected. It also checks that paragraph boundaries survive the conversion into Markdown, instead of collapsing into one long line of running text.</p>
<p>A third paragraph rounds out the fixture. Headings, paragraphs and link references should all be present in the converted Markdown, and the surrounding page furniture should not be, which is the whole reason the fallback runs on a chrome-stripped tree.</p>
</div>
</div>
</body>
</html>`

const redirectPage = `<!doctype html>
<html>
<head>
<title>Redirect…</title>
<script>window.location.replace("/new/place.html")</script>
</head>
<body>
<p>This page has moved.</p>
<p><a href="/new/place.html">Continue</a></p>
</body>
</html>`

const refreshPage = `<!doctype html>
<html>
<head>
<title>Moved</title>
<meta http-equiv="refresh" content="0; url=/new/home">
</head>
<body></body>
</html>`

func fixture(t *testing.T, source string) Document {
	t.Helper()
	doc, err := Convert(context.Background(), []byte(source), pageURL, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return doc
}

func requireContains(t *testing.T, markdown string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}
}

func requireAbsent(t *testing.T, markdown string, unwanted ...string) {
	t.Helper()
	for _, absent := range unwanted {
		if strings.Contains(markdown, absent) {
			t.Errorf("markdown contains %q:\n%s", absent, markdown)
		}
	}
}

func TestArticleStructureSurvives(t *testing.T) {
	doc := fixture(t, articlePage)
	if doc.Method != MethodMain {
		t.Fatalf("method = %q, want %q", doc.Method, MethodMain)
	}
	if doc.Title != "Reading architecture — Example docs" {
		t.Errorf("title = %q", doc.Title)
	}
	requireContains(t, doc.Markdown,
		"# Reading architecture",
		"## Installation",
		"```go",
		"|Flag|Meaning|",
		"|---|---|",
		"|-race|enables the race detector|",
		"](https://example.com/docs/guide.html)",
		"![Architecture diagram](https://example.com/docs/intro/images/architecture.png)",
		"Left cell text",
	)
	requireAbsent(t, doc.Markdown, "Sign in", "Footer text", "analytics beacon", "margin: 0")
	if separators := strings.Count(doc.Markdown, "|---|---|"); separators != 1 {
		t.Errorf("table separators = %d, want only the data table:\n%s", separators, doc.Markdown)
	}
	if layout := strings.Count(doc.Markdown, "|Left cell text|"); layout != 0 {
		t.Errorf("layout table converted as data table:\n%s", doc.Markdown)
	}
}

func TestReadabilityFallback(t *testing.T) {
	doc := fixture(t, fallbackPage)
	if doc.Method != MethodReadability {
		t.Fatalf("method = %q, want %q", doc.Method, MethodReadability)
	}
	if doc.Title != "Fallback story" {
		t.Errorf("title = %q", doc.Title)
	}
	// Readability folds a heading that repeats the title into the title itself,
	// so the heading is reported through Document.Title instead of the Markdown.
	requireContains(t, doc.Markdown, "A second paragraph keeps the candidate score high enough", "A third paragraph rounds out the fixture")
}

func TestScriptRedirectReported(t *testing.T) {
	doc := fixture(t, redirectPage)
	if doc.Method != MethodRedirect || doc.Markdown != "" {
		t.Fatalf("document = %+v, want redirect without markdown", doc)
	}
	if doc.Redirect != "https://example.com/new/place.html" {
		t.Errorf("redirect = %q", doc.Redirect)
	}
	if doc.Title != "Redirect…" {
		t.Errorf("title = %q", doc.Title)
	}
}

func TestMetaRefreshReported(t *testing.T) {
	doc := fixture(t, refreshPage)
	if doc.Method != MethodRedirect || doc.Markdown != "" {
		t.Fatalf("document = %+v, want redirect without markdown", doc)
	}
	if doc.Redirect != "https://example.com/new/home" {
		t.Errorf("redirect = %q", doc.Redirect)
	}
}

func TestMetaRefreshWithContentKeepsContent(t *testing.T) {
	source := strings.Replace(articlePage, "<title>", `<meta http-equiv="refresh" content="300; url=/docs/intro/page.html"><title>`, 1)
	doc := fixture(t, source)
	if doc.Method != MethodMain {
		t.Fatalf("method = %q, want %q", doc.Method, MethodMain)
	}
	if doc.Redirect != pageURL {
		t.Errorf("redirect = %q, want %q", doc.Redirect, pageURL)
	}
	requireContains(t, doc.Markdown, "# Reading architecture")
}

func TestNonHTTPReferencesDropped(t *testing.T) {
	source := strings.Replace(articlePage, `<p>Continue in the <a href="../guide.html">guide</a>.</p>`,
		`<p>Continue in the <a href="../guide.html">guide</a> or <a href="javascript:alert(1)">toggle</a>.</p>`, 1)
	doc := fixture(t, source)
	requireContains(t, doc.Markdown, "or toggle.")
	requireAbsent(t, doc.Markdown, "javascript:", "]()")
}

func TestCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Convert(ctx, []byte(articlePage), pageURL, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestCanceledWhileParsing(t *testing.T) {
	source := []byte(strings.Repeat("<div><p>text</p></div>", 20_000))
	ctx := &countingContext{Context: context.Background(), remaining: 3}
	if _, err := Convert(ctx, source, pageURL, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestDepthLimit(t *testing.T) {
	base, err := parseBaseURL(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("<html><body><article>" + strings.Repeat("<div>", 8) + "deep text here, long enough to pass the text check comfortably" + strings.Repeat("</div>", 8) + "</article></body></html>")
	l, err := Options{}.limits()
	if err != nil {
		t.Fatal(err)
	}
	l.maxDepth = 4
	if _, err := convert(context.Background(), source, base, l); !errors.Is(err, ErrTooComplex) {
		t.Fatalf("err = %v, want ErrTooComplex", err)
	}
}

func TestNodeLimit(t *testing.T) {
	base, err := parseBaseURL(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("<html><body><article>" + strings.Repeat("<p>paragraph text</p>", 20) + "</article></body></html>")
	l, err := Options{}.limits()
	if err != nil {
		t.Fatal(err)
	}
	l.maxNodes = 10
	if _, err := convert(context.Background(), source, base, l); !errors.Is(err, ErrTooComplex) {
		t.Fatalf("err = %v, want ErrTooComplex", err)
	}
}

func TestLimits(t *testing.T) {
	cases := []struct {
		name    string
		options Options
		source  string
		want    error
	}{
		{name: "input", options: Options{MaxInputBytes: 100}, source: articlePage, want: ErrInputTooLarge},
		{name: "output", options: Options{MaxOutputBytes: 100}, source: articlePage, want: ErrOutputTooLarge},
		{name: "negative input", options: Options{MaxInputBytes: -1}, source: articlePage, want: ErrInvalidLimit},
		{name: "negative output", options: Options{MaxOutputBytes: -1}, source: articlePage, want: ErrInvalidLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Convert(context.Background(), []byte(tc.source), pageURL, tc.options); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestInvalidBaseURL(t *testing.T) {
	for _, base := range []string{"", "not a url", "/docs/page.html", "ftp://example.com/x", "javascript:alert(1)", "//example.com/x"} {
		if _, err := Convert(context.Background(), []byte(articlePage), base, Options{}); !errors.Is(err, ErrInvalidBaseURL) {
			t.Errorf("base %q: err = %v, want ErrInvalidBaseURL", base, err)
		}
	}
}

func TestNoContent(t *testing.T) {
	if _, err := Convert(context.Background(), []byte("<html><body><nav><a href=\"/\">Home</a></nav></body></html>"), pageURL, Options{}); !errors.Is(err, ErrNoContent) {
		t.Fatalf("err = %v, want ErrNoContent", err)
	}
}

func TestSourceNotMutated(t *testing.T) {
	source := []byte(articlePage)
	original := bytes.Clone(source)
	if _, err := Convert(context.Background(), source, pageURL, Options{}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(source, original) {
		t.Fatal("caller source was mutated")
	}
}

// countingContext reports cancellation once its budget of Err calls is spent,
// which lets a test cancel at a precise point of the pipeline.
type countingContext struct {
	context.Context
	remaining int
}

func (c *countingContext) Err() error {
	if c.remaining <= 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}
