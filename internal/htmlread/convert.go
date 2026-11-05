// Package htmlread converts a static HTML page into readable Markdown without
// executing scripts, fetching subresources or following redirects.
package htmlread

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
	converter "github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

const (
	// DefaultMaxInputBytes bounds the HTML handed to the parser.
	DefaultMaxInputBytes = 4 << 20
	// DefaultMaxOutputBytes bounds the complete Markdown handed back.
	DefaultMaxOutputBytes = 4 << 20

	maxDepth        = 512
	maxNodes        = 200_000
	minContentRunes = 40
	ctxCheckEvery   = 1024
)

const (
	// MethodMain reports content taken from a semantic container.
	MethodMain = "main"
	// MethodReadability reports content recovered by the readability fallback.
	MethodReadability = "readability"
	// MethodRedirect reports a page that carried no readable content and only a
	// redirect target; Markdown is empty and Redirect names the target.
	MethodRedirect = "redirect"
)

var (
	// ErrInvalidBaseURL reports a page URL that is not absolute http(s).
	ErrInvalidBaseURL = errors.New("invalid base URL")
	// ErrInvalidLimit reports a negative byte limit.
	ErrInvalidLimit = errors.New("invalid size limit")
	// ErrInputTooLarge reports HTML above the input limit.
	ErrInputTooLarge = errors.New("input exceeds size limit")
	// ErrOutputTooLarge reports Markdown above the output limit. Output is never truncated.
	ErrOutputTooLarge = errors.New("markdown exceeds size limit")
	// ErrTooComplex reports a document beyond the depth or node bounds.
	ErrTooComplex = errors.New("document exceeds structural limits")
	// ErrNoContent reports a page without meaningful readable content.
	ErrNoContent = errors.New("no readable content")
)

// Options bounds one conversion. A zero limit selects the package default and a
// negative limit is refused.
type Options struct {
	MaxInputBytes  int
	MaxOutputBytes int
}

// Document is the readable content of one page.
type Document struct {
	// Title is the readability title when the fallback ran, otherwise the
	// document title; it is empty when the page has neither.
	Title string
	// Markdown is the converted content.
	Markdown string
	// Method is one of MethodMain, MethodReadability or MethodRedirect.
	Method string
	// Redirect is the absolute target of a static meta refresh or a script-only
	// redirect. It is reported, never fetched or executed; following the target
	// across hops belongs to the caller's HTTP layer.
	Redirect string
}

// limits are the bounds applied to one conversion.
type limits struct {
	maxInput  int
	maxOutput int
	maxDepth  int
	maxNodes  int
	minRunes  int
}

func (o Options) limits() (limits, error) {
	if o.MaxInputBytes < 0 || o.MaxOutputBytes < 0 {
		return limits{}, fmt.Errorf("%w: limits must not be negative", ErrInvalidLimit)
	}
	l := limits{
		maxInput:  DefaultMaxInputBytes,
		maxOutput: DefaultMaxOutputBytes,
		maxDepth:  maxDepth,
		maxNodes:  maxNodes,
		minRunes:  minContentRunes,
	}
	if o.MaxInputBytes > 0 {
		l.maxInput = o.MaxInputBytes
	}
	if o.MaxOutputBytes > 0 {
		l.maxOutput = o.MaxOutputBytes
	}
	return l, nil
}

// Convert renders source as readable Markdown. Relative references resolve
// against baseURL, which must be the complete absolute page URL. Callers must
// not modify source during the call; it is never written to.
func Convert(ctx context.Context, source []byte, baseURL string, options Options) (Document, error) {
	l, err := options.limits()
	if err != nil {
		return Document{}, err
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	base, err := parseBaseURL(baseURL)
	if err != nil {
		return Document{}, err
	}
	if len(source) > l.maxInput {
		return Document{}, fmt.Errorf("%w: %d bytes exceed the %d byte limit", ErrInputTooLarge, len(source), l.maxInput)
	}
	return convert(ctx, source, base, l)
}

func parseBaseURL(raw string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidBaseURL, err)
	}
	if (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("%w: %q is not an absolute http(s) URL", ErrInvalidBaseURL, raw)
	}
	base.Fragment, base.RawFragment = "", ""
	return base, nil
}

func convert(ctx context.Context, source []byte, base *url.URL, l limits) (Document, error) {
	doc, err := html.Parse(&contextReader{ctx: ctx, reader: bytes.NewReader(source)})
	if err != nil {
		return Document{}, fmt.Errorf("parse HTML: %w", err)
	}
	state := &prepareState{ctx: ctx, base: base, limits: l}
	if err := state.prepare(doc, 0); err != nil {
		return Document{}, err
	}
	markLayoutTables(doc)

	node, method, extractedTitle := selectContent(doc, base)
	title := documentTitle(doc)
	if extractedTitle != "" {
		title = extractedTitle
	}
	redirect := state.target()

	if node == nil {
		return noContent(title, redirect, "no semantic container and readability found no article")
	}
	if text := visibleText(node); utf8.RuneCountInString(text) < l.minRunes {
		return noContent(title, redirect, fmt.Sprintf("%d characters of text", utf8.RuneCountInString(text)))
	}

	markdown, err := toMarkdown(ctx, node)
	if err != nil {
		return Document{}, fmt.Errorf("convert to markdown: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Document{}, err
	}
	if strings.TrimSpace(markdown) == "" {
		return noContent(title, redirect, "content converted to empty Markdown")
	}
	if len(markdown) > l.maxOutput {
		return Document{}, fmt.Errorf("%w: %d bytes exceed the %d byte limit", ErrOutputTooLarge, len(markdown), l.maxOutput)
	}
	return Document{Title: title, Markdown: markdown, Method: method, Redirect: redirect}, nil
}

// noContent reports a redirect when the page has one and an error otherwise.
func noContent(title, redirect, reason string) (Document, error) {
	if redirect != "" {
		return Document{Title: title, Method: MethodRedirect, Redirect: redirect}, nil
	}
	return Document{}, fmt.Errorf("%w: %s", ErrNoContent, reason)
}

// selectContent prefers semantic containers and falls back to readability.
func selectContent(doc *html.Node, base *url.URL) (*html.Node, string, string) {
	if main := pickMain(doc); main != nil {
		return main, MethodMain, ""
	}
	parser := readability.NewParser()
	article, err := parser.ParseDocument(doc, base)
	if err != nil || article.Node == nil {
		return nil, "", ""
	}
	return article.Node, MethodReadability, article.Title()
}

func toMarkdown(ctx context.Context, node *html.Node) (string, error) {
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(),
		// Dropped non-http references leave no href; the anchor text stays plain.
		commonmark.NewCommonmarkPlugin(commonmark.WithLinkEmptyHrefBehavior(commonmark.LinkBehaviorSkip)),
		table.NewTablePlugin(
			table.WithCellPaddingBehavior(table.CellPaddingBehaviorNone),
			table.WithSkipEmptyRows(true),
		),
	))
	out, err := conv.ConvertNode(node, converter.WithContext(ctx))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// contextReader makes parsing abandonable: the tokenizer reads in chunks and
// sees the cancellation between them.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
