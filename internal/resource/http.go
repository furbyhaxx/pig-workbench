package resource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html/charset"

	"github.com/furbyhaxx/pig-workbench/internal/htmlread"
)

const (
	httpDefaultMaxBytes = 4 << 20
	httpMaxRedirects    = 8
	httpTimeout         = 30 * time.Second
)

var (
	// ErrHTTPUnsupportedContent reports a response the static reader cannot render.
	ErrHTTPUnsupportedContent = errors.New("http content type is unsupported by the static reader")
	// ErrHTTPTooLarge reports a response above the configured byte ceiling.
	ErrHTTPTooLarge = errors.New("http response exceeds the byte limit")
)

// HTTPProvider serves static http(s) resources. It never executes scripts, sends
// credentials or follows a redirect to another scheme.
type HTTPProvider struct {
	// Secure selects the https scheme; the zero value serves http.
	Secure bool
	// MaxBytes bounds the response and rendered output; zero selects 4 MiB.
	MaxBytes int
	// Client overrides the bounded default client in tests.
	Client *http.Client
	// Backends routes URLs to contributed fetch backends before the builtin reader.
	Backends FetchRouter
}

// BackendFetch is a contributed fetch backend's answer for one URL.
type BackendFetch struct {
	Body           []byte
	ContentType    string
	FinalURI       string
	Redirects      []string
	Revision       string
	Handler        string
	Notices        []string
	Status         int
	UsedThirdParty bool
}

// ErrBackendDefinitive marks a backend answer that must not fall back to the builtin reader.
var ErrBackendDefinitive = errors.New("fetch backend reported a definitive origin answer")

// FetchRouter selects and calls a contributed fetch backend for one URL.
type FetchRouter interface {
	// Fetch returns matched=false when no enabled backend matches the URL. An error
	// wrapping ErrBackendDefinitive is final; any other error falls back with a notice.
	Fetch(ctx context.Context, uri string, limit int) (result BackendFetch, matched bool, err error)
}

// Scheme reports the URL scheme this provider owns.
func (p HTTPProvider) Scheme() string {
	if p.Secure {
		return "https"
	}
	return "http"
}

// Stat is unsupported: HTTP has no cheap, portable metadata contract here.
func (HTTPProvider) Stat(context.Context, string) (StatResult, error) {
	return StatResult{}, ErrUnsupported
}

// List is unsupported for a single web resource.
func (HTTPProvider) List(context.Context, string, string, int) (Listing, error) {
	return Listing{}, ErrUnsupported
}

// Read fetches one URL and renders either readable Markdown or the raw document.
func (p HTTPProvider) Read(ctx context.Context, request ReadRequest) (ReadResult, error) {
	if err := p.validateURI(request.URI); err != nil {
		return ReadResult{}, err
	}
	limit := p.MaxBytes
	if limit <= 0 {
		limit = httpDefaultMaxBytes
	}
	fetched, err := p.fetchRouted(ctx, request.URI, limit)
	if err != nil {
		return ReadResult{}, err
	}
	mediaType, _, err := mime.ParseMediaType(fetched.contentType)
	if err != nil || mediaType == "" {
		mediaType = http.DetectContentType(fetched.body)
		mediaType, _, _ = mime.ParseMediaType(mediaType)
	}
	decoded, err := decodeHTTPBody(fetched.body, fetched.contentType, limit)
	if err != nil {
		return ReadResult{}, err
	}
	if !utf8.Valid(decoded) {
		return ReadResult{}, errors.New("http response is not valid UTF-8 after charset decoding")
	}
	target := Target{Requested: request.Requested, URI: fetched.finalURI, Scheme: p.Scheme()}
	digest := sha256.Sum256(fetched.body)
	revision := hex.EncodeToString(digest[:])
	if fetched.revision != "" {
		revision = fetched.revision
	}
	basePage := Page{
		RequestedURI:    request.Requested,
		CanonicalURI:    fetched.finalURI,
		Revision:        revision,
		ContentType:     fetched.contentType,
		Notices:         fetched.notices,
		CoordinateSpace: "rendered-lines",
	}
	var content []byte
	var header string
	switch {
	case isHTMLMediaType(mediaType) && request.Selector.Raw:
		basePage.Handler = fetched.handlerPrefix + "builtin-html-raw"
		content = decoded
	case isHTMLMediaType(mediaType):
		document, err := htmlread.Convert(ctx, decoded, fetched.finalURI, htmlread.Options{MaxInputBytes: limit, MaxOutputBytes: limit})
		if err != nil {
			return ReadResult{}, fmt.Errorf("render HTML: %w", err)
		}
		basePage.Handler = fetched.handlerPrefix + "builtin-html-" + document.Method
		if document.Redirect != "" {
			basePage.Notices = append(basePage.Notices, "page reports a static redirect to "+displayText(document.Redirect))
		}
		content = []byte(document.Markdown)
		if document.Method == htmlread.MethodRedirect || strings.TrimSpace(document.Markdown) == "" {
			content = []byte("Static redirect reported: " + displayText(document.Redirect) + "\nRead that URL explicitly; it was not fetched automatically.\n")
		}
		header = "[" + displayText(fetched.finalURI) + "]\nHTML " + document.Method + " view; no hashline anchors.\n"
	case isTextMediaType(mediaType):
		basePage.Handler = fetched.handlerPrefix + "builtin-text"
		content = decoded
		header = "[" + displayText(fetched.finalURI) + "]\nText resource; no hashline anchors.\n"
	default:
		return ReadResult{}, fmt.Errorf("%w: %s", ErrHTTPUnsupportedContent, mediaType)
	}
	if request.Selector.Raw {
		header = ""
	}
	page, err := pageMemory(target, content, request.Selector, request.Budget, header)
	if err != nil {
		return ReadResult{}, err
	}
	page.RequestedURI = request.Requested
	page.CanonicalURI = fetched.finalURI
	page.Revision = basePage.Revision
	page.Handler = basePage.Handler
	page.ContentType = basePage.ContentType
	page.Notices = basePage.Notices
	return ReadResult{Content: page.Content, Facts: page}, nil
}

func (p HTTPProvider) validateURI(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid http URL")
	}
	if parsed.Scheme != p.Scheme() {
		return fmt.Errorf("http provider cannot read scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("http URL requires a host")
	}
	if parsed.User != nil {
		return errors.New("http URL must not contain credentials")
	}
	if parsed.Path == "" {
		return errors.New("http URL requires an explicit / path")
	}
	if parsed.Fragment != "" {
		return errors.New("http URL must not contain a fragment after selector parsing")
	}
	return nil
}

type httpResult struct {
	body          []byte
	contentType   string
	finalURI      string
	notices       []string
	revision      string
	handlerPrefix string
}

// fetchRouted asks a matching contributed backend first and falls back to the
// builtin reader on unavailability or backend failure, never on a definitive answer.
func (p HTTPProvider) fetchRouted(ctx context.Context, raw string, limit int) (httpResult, error) {
	if p.Backends == nil {
		return p.fetch(ctx, raw, limit)
	}
	answer, matched, err := p.Backends.Fetch(ctx, raw, limit)
	if !matched {
		return p.fetch(ctx, raw, limit)
	}
	if err == nil {
		err = p.validateBackend(answer, limit)
	}
	if err != nil {
		if errors.Is(err, ErrBackendDefinitive) {
			return httpResult{}, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return httpResult{}, ctxErr
		}
		fallback, fetchErr := p.fetch(ctx, raw, limit)
		if fetchErr != nil {
			return httpResult{}, fetchErr
		}
		fallback.notices = append([]string{"fetch backend failed (" + displayText(err.Error()) + "); used the builtin reader"}, fallback.notices...)
		return fallback, nil
	}
	notices := append([]string(nil), answer.Notices...)
	if answer.UsedThirdParty {
		notices = append(notices, "a third-party service saw this URL")
	}
	if answer.FinalURI != raw {
		notices = append(notices, "backend followed redirects to "+displayText(answer.FinalURI))
	}
	return httpResult{body: answer.Body, contentType: answer.ContentType, finalURI: answer.FinalURI, notices: notices, revision: answer.Revision, handlerPrefix: answer.Handler + ":"}, nil
}

func (p HTTPProvider) validateBackend(answer BackendFetch, limit int) error {
	if len(answer.Body) > limit {
		return fmt.Errorf("%w: backend body exceeds %d bytes", ErrHTTPTooLarge, limit)
	}
	if len(answer.Redirects) > httpMaxRedirects {
		return fmt.Errorf("backend reported more than %d redirects", httpMaxRedirects)
	}
	seen := map[string]bool{}
	for _, hop := range append(append([]string(nil), answer.Redirects...), answer.FinalURI) {
		parsed, err := url.Parse(hop)
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return errors.New("backend reported a redirect outside http(s) or with credentials")
		}
		if seen[hop] && hop != answer.FinalURI {
			return errors.New("backend reported a redirect cycle")
		}
		seen[hop] = true
	}
	if answer.Status >= 400 && answer.Status < 500 {
		return fmt.Errorf("%w: http status %d %s", ErrBackendDefinitive, answer.Status, http.StatusText(answer.Status))
	}
	if answer.Status < 200 || answer.Status > 299 {
		return fmt.Errorf("backend reported http status %d", answer.Status)
	}
	return nil
}

func (p HTTPProvider) fetch(ctx context.Context, raw string, limit int) (httpResult, error) {
	client := p.Client
	if client == nil {
		clone := *http.DefaultClient
		clone.Timeout = httpTimeout
		client = &clone
	}
	clientCopy := *client
	previous := client.CheckRedirect
	clientCopy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= httpMaxRedirects {
			return fmt.Errorf("http redirect limit of %d exceeded", httpMaxRedirects)
		}
		if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
			return errors.New("http redirect left the http(s) schemes")
		}
		if request.URL.User != nil {
			return errors.New("http redirect contains credentials")
		}
		for _, prior := range via {
			if prior.URL.String() == request.URL.String() {
				return errors.New("http redirect cycle detected")
			}
		}
		request.Header.Del("Authorization")
		request.Header.Del("Cookie")
		if previous != nil {
			return previous(request, via)
		}
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return httpResult{}, errors.New("invalid http request")
	}
	request.Header.Set("Accept", "text/html, application/xhtml+xml, text/plain, text/markdown, application/json, application/xml;q=0.9, text/*;q=0.8")
	request.Header.Set("User-Agent", "pig-workbench/0.1 static-reader")
	response, err := clientCopy.Do(request)
	if err != nil {
		return httpResult{}, fmt.Errorf("fetch URL: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return httpResult{}, fmt.Errorf("http status %d %s", response.StatusCode, http.StatusText(response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	if err != nil {
		return httpResult{}, fmt.Errorf("read http response: %w", err)
	}
	if len(body) > limit {
		return httpResult{}, fmt.Errorf("%w: response exceeds %d bytes", ErrHTTPTooLarge, limit)
	}
	notices := make([]string, 0, 1)
	finalURI := response.Request.URL.String()
	if finalURI != raw {
		notices = append(notices, "followed HTTP redirects to "+displayText(finalURI))
	}
	return httpResult{body: body, contentType: response.Header.Get("Content-Type"), finalURI: finalURI, notices: notices}, nil
}

func decodeHTTPBody(body []byte, contentType string, limit int) ([]byte, error) {
	reader, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return nil, fmt.Errorf("decode response charset: %w", err)
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("decode response body: %w", err)
	}
	if len(decoded) > limit {
		return nil, fmt.Errorf("%w: decoded response exceeds %d bytes", ErrHTTPTooLarge, limit)
	}
	return decoded, nil
}

func isHTMLMediaType(mediaType string) bool {
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

func isTextMediaType(mediaType string) bool {
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/x-yaml", "application/yaml", "application/toml", "application/javascript":
		return true
	}
	return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
}

type memoryLine struct {
	start, end, next int
}

func splitMemoryLines(source []byte) []memoryLine {
	var lines []memoryLine
	for start := 0; start < len(source); {
		index := bytes.IndexByte(source[start:], '\n')
		end := len(source)
		next := len(source)
		if index >= 0 {
			end = start + index
			next = end + 1
		}
		lines = append(lines, memoryLine{start: start, end: end, next: next})
		start = next
	}
	return lines
}

func pageMemory(target Target, source []byte, selection Selection, budget Budget, header string) (Page, error) {
	page := Page{
		RequestedURI: target.Requested, CanonicalURI: target.URI,
		CoordinateSpace: "rendered-lines", ShownRanges: []Range{}, FullyShownRanges: []Range{},
	}
	lines := splitMemoryLines(source)
	page.TotalLines = len(lines)
	ranges, err := selection.Resolve(len(lines), true)
	if err != nil {
		return page, err
	}
	if budget.Bytes <= 0 || budget.Lines <= 0 {
		return page, errors.New("read budget must be positive")
	}
	const footerBytes = 256
	available := budget.Bytes - len(header) - footerBytes
	lineBudget := budget.Lines - strings.Count(header, "\n") - 2
	if available < 1 || lineBudget < 1 {
		return page, errors.New("read budget too small for source identity and continuation")
	}
	var body strings.Builder
	selected, shown := 0, 0
	for _, span := range ranges {
		selected += span.End - span.Start + 1
	}
	next := 0
	for _, span := range ranges {
		for line := span.Start; line <= span.End; line++ {
			record := lines[line-1]
			text := source[record.start:record.end]
			prefix := ""
			if !selection.Raw {
				if line == 1 {
					text = bytes.TrimPrefix(text, []byte{0xef, 0xbb, 0xbf})
				}
				text = bytes.TrimSuffix(text, []byte{'\r'})
				prefix = strconv.Itoa(line) + ":"
			}
			extra := 0
			if !selection.Raw {
				extra = 1
			}
			room := available - body.Len() - len(prefix) - extra
			if lineBudget <= 0 || room < 0 {
				next = line
				page.Truncation = "budget"
				break
			}
			if len(text) > room {
				text = text[:room]
				for len(text) > 0 && !utf8.Valid(text) {
					text = text[:len(text)-1]
				}
				if len(text) == 0 {
					next = line
					page.Truncation = "partialLine"
					break
				}
				body.WriteString(prefix)
				body.Write(text)
				appendSpan(&page.ShownRanges, line)
				next = line
				page.Truncation = "partialLine"
				break
			}
			body.WriteString(prefix)
			body.Write(text)
			if !selection.Raw {
				body.WriteByte('\n')
			} else if record.next > record.end {
				body.WriteByte('\n')
			}
			appendSpan(&page.ShownRanges, line)
			appendSpan(&page.FullyShownRanges, line)
			shown++
			lineBudget--
		}
		if next != 0 {
			break
		}
	}
	page.SelectionComplete = shown == selected
	page.ResourceComplete = page.SelectionComplete && len(ranges) == 1 && ranges[0].Start == 1 && ranges[0].End == len(lines)
	if len(lines) == 0 {
		page.SelectionComplete, page.ResourceComplete = true, true
	}
	footer := ""
	if !page.SelectionComplete {
		suffix := continuationSelection(ranges, next, selection.Raw)
		page.Continuation = page.CanonicalURI + suffix
		footer = "\n… incomplete; continue with selector " + suffix + " on the same resource"
	}
	page.Content = header + body.String() + footer
	if len(page.Content) > budget.Bytes {
		return Page{}, errors.New("internal read framing exceeded byte budget")
	}
	return page, nil
}
