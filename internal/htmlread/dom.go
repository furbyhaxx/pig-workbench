package htmlread

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// chrome lists elements that hold page furniture rather than readable content.
var chrome = map[string]bool{
	"nav":      true,
	"footer":   true,
	"aside":    true,
	"script":   true,
	"style":    true,
	"noscript": true,
	"form":     true,
	"button":   true,
	"svg":      true,
	"iframe":   true,
}

// chromeClasses lists content-furniture classes observed on real pages.
var chromeClasses = []string{"mw-editsection", "navbox", "vector-dropdown", "mw-indicators", "noprint"}

var (
	scriptLocationAssign = regexp.MustCompile(`(?i)\blocation\s*(?:\.\s*href\s*)?=\s*["']([^"']+)["']`)
	scriptLocationCall   = regexp.MustCompile(`(?i)\blocation\s*\.\s*(?:replace|assign)\s*\(\s*["']([^"']+)["']`)
)

// prepareState carries the bounds, the page URL and the redirect candidates
// through one pre-order pass over the parsed document.
type prepareState struct {
	ctx    context.Context
	base   *url.URL
	limits limits
	nodes  int
	meta   string
	script string
}

// prepare strips chrome and permalinks, resolves references against the page
// URL, notes redirect candidates and enforces the structural bounds.
func (s *prepareState) prepare(n *html.Node, depth int) error {
	if depth > s.limits.maxDepth {
		return fmt.Errorf("%w: element nesting deeper than %d levels", ErrTooComplex, s.limits.maxDepth)
	}
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		s.nodes++
		if s.nodes > s.limits.maxNodes {
			return fmt.Errorf("%w: more than %d elements", ErrTooComplex, s.limits.maxNodes)
		}
		if s.nodes%ctxCheckEvery == 0 {
			if err := s.ctx.Err(); err != nil {
				return err
			}
		}
		if c.Type == html.ElementNode {
			s.observeRedirect(c)
			switch {
			case isChrome(c):
				n.RemoveChild(c)
				c = next
				continue
			case isPermalink(c):
				n.RemoveChild(c)
				c = next
				continue
			}
			resolveRefs(c, s.base)
		}
		if err := s.prepare(c, depth+1); err != nil {
			return err
		}
		c = next
	}
	return nil
}

// observeRedirect records the first static meta refresh and the first script
// location assignment. Nothing is executed or fetched.
func (s *prepareState) observeRedirect(n *html.Node) {
	switch n.Data {
	case "meta":
		if s.meta == "" && strings.EqualFold(attr(n, "http-equiv"), "refresh") {
			s.meta = refreshTarget(attr(n, "content"))
		}
	case "script":
		if s.script == "" {
			text := textOf(n)
			if m := scriptLocationAssign.FindStringSubmatch(text); m != nil {
				s.script = m[1]
			} else if m := scriptLocationCall.FindStringSubmatch(text); m != nil {
				s.script = m[1]
			}
		}
	}
}

// target returns the absolute redirect destination, preferring the static
// meta refresh over a script assignment.
func (s *prepareState) target() string {
	for _, raw := range []string{s.meta, s.script} {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := s.base.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		return u.String()
	}
	return ""
}

// refreshTarget extracts the url from a meta refresh content value.
func refreshTarget(content string) string {
	i := strings.Index(strings.ToLower(content), "url=")
	if i < 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(content[i+len("url="):]), `"'`)
}

// isChrome reports page furniture that must not reach the Markdown.
func isChrome(n *html.Node) bool {
	if chrome[n.Data] {
		return true
	}
	if attr(n, "role") == "navigation" || attr(n, "aria-hidden") == "true" || hasAttr(n, "hidden") {
		return true
	}
	for _, class := range chromeClasses {
		if hasClass(n, class) {
			return true
		}
	}
	return false
}

// isPermalink reports heading permalinks, whose anchors carry no content.
func isPermalink(n *html.Node) bool {
	if n.Data != "a" {
		return false
	}
	if hasClass(n, "anchor") || hasClass(n, "headerlink") {
		return true
	}
	switch strings.TrimSpace(textOf(n)) {
	case "¶", "#", "§":
		return true
	}
	return false
}

// resolveRefs makes href and src absolute against the complete page URL.
// Non-http schemes are dropped so no script target survives into the Markdown.
func resolveRefs(n *html.Node, base *url.URL) {
	for i, a := range n.Attr {
		if a.Key != "href" && a.Key != "src" {
			continue
		}
		ref := strings.TrimSpace(a.Val)
		if ref == "" {
			continue
		}
		u, err := base.Parse(ref)
		if err != nil {
			n.Attr[i].Val = ""
			continue
		}
		switch u.Scheme {
		case "http", "https":
			n.Attr[i].Val = u.String()
		case "mailto":
		default:
			n.Attr[i].Val = ""
		}
	}
}

// markLayoutTables flags headerless tables as presentation so the converter
// renders them as text instead of a one-column Markdown table.
func markLayoutTables(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		markLayoutTables(c)
		if c.Data != "table" || attr(c, "role") != "" {
			continue
		}
		if findElement(c, func(x *html.Node) bool { return x.Data == "th" }) == nil {
			c.Attr = append(c.Attr, html.Attribute{Key: "role", Val: "presentation"})
		}
	}
}

// pickMain returns the first semantic content container, most specific first.
func pickMain(doc *html.Node) *html.Node {
	predicates := []func(*html.Node) bool{
		func(n *html.Node) bool { return hasClass(n, "markdown-body") },
		func(n *html.Node) bool { return n.Data == "article" },
		func(n *html.Node) bool { return n.Data == "main" },
		func(n *html.Node) bool { return attr(n, "role") == "main" },
		func(n *html.Node) bool { return attr(n, "id") == "content" || attr(n, "id") == "main-content" },
	}
	for _, predicate := range predicates {
		if n := findElement(doc, predicate); n != nil {
			return n
		}
	}
	return nil
}

func findElement(n *html.Node, predicate func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && predicate(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElement(c, predicate); found != nil {
			return found
		}
	}
	return nil
}

func documentTitle(doc *html.Node) string {
	title := findElement(doc, func(n *html.Node) bool { return n.Data == "title" })
	if title == nil {
		return ""
	}
	return collapse(textOf(title))
}

// visibleText returns the collapsed text content of a subtree.
func visibleText(n *html.Node) string { return collapse(textOf(n)) }

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(textOf(c))
	}
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasClass(n *html.Node, class string) bool {
	for _, field := range strings.Fields(attr(n, "class")) {
		if field == class {
			return true
		}
	}
	return false
}
