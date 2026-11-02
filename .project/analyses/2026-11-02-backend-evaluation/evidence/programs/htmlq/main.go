// Converts saved pages with readability→markdown and with markdown only, printing structure counts.
package main

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codeberg.org/readeck/go-readability/v2"
	"golang.org/x/net/html"
	md "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
)

var (
	fence    = regexp.MustCompile("(?m)^```")
	fenceLng = regexp.MustCompile("(?m)^```[a-zA-Z]")
	tableRow = regexp.MustCompile(`(?m)^\|.*\|\s*$`)
	heading  = regexp.MustCompile(`(?m)^#{1,6} `)
	relLink  = regexp.MustCompile(`\]\((/[^)]*|\.\.?/[^)]*)\)`)
	link     = regexp.MustCompile(`\]\([^)]+\)`)
)

func conv(html, base string) (string, error) {
	u, _ := url.Parse(base)
	c := converter.NewConverter(converter.WithPlugins(baseplugin(), commonmark.NewCommonmarkPlugin(),
		table.NewTablePlugin(table.WithCellPaddingBehavior(table.CellPaddingBehaviorNone), table.WithSkipEmptyRows(true))))
	return c.ConvertString(html, converter.WithDomain(u.Scheme+"://"+u.Host))
}

func baseplugin() converter.Plugin { return base.NewBasePlugin() }

func stats(s string) string {
	return fmt.Sprintf("bytes=%-7d headings=%-3d fences=%-3d fencesWithLang=%-3d tableRows=%-4d links=%-4d relLinks=%d",
		len(s), len(heading.FindAllString(s, -1)), len(fence.FindAllString(s, -1))/2, len(fenceLng.FindAllString(s, -1)),
		len(tableRow.FindAllString(s, -1)), len(link.FindAllString(s, -1)), len(relLink.FindAllString(s, -1)))
}

func main() {
	out := os.Args[1]
	for _, p := range os.Args[2:] {
		name := strings.TrimSuffix(filepath.Base(p), ".html")
		raw, _ := os.ReadFile(p)
		pageURL, _ := os.ReadFile(strings.TrimSuffix(p, ".html") + ".url")
		u, _ := url.Parse(strings.TrimSpace(string(pageURL)))
		t0 := time.Now()
		art, err := readability.FromReader(bytes.NewReader(raw), u)
		var rdHTML bytes.Buffer
		if err == nil && art.Node != nil {
			art.RenderHTML(&rdHTML)
		}
		rd, cerr := conv(rdHTML.String(), u.String())
		d1 := time.Since(t0)
		t1 := time.Now()
		full, ferr := conv(string(raw), u.String())
		d2 := time.Since(t1)
		_ = md.ConvertString
		doc, _ := html.Parse(bytes.NewReader(raw))
		t2 := time.Now()
		var mainHTML bytes.Buffer
		how := "readability-fallback"
		if n := pickMain(doc); n != nil {
			stripChrome(n)
			resolveLinks(n, u)
			markLayoutTables(n)
			html.Render(&mainHTML, n)
			how = "main:" + describe(n)
		} else if art.Node != nil {
			stripChrome(art.Node)
			resolveLinks(art.Node, u)
			markLayoutTables(art.Node)
			html.Render(&mainHTML, art.Node)
		}
		mm, merr := conv(mainHTML.String(), u.String())
		d3 := time.Since(t2)
		fmt.Printf("%-10s in=%-7d readability: title=%q err=%v convErr=%v %s %v\n", name, len(raw), art.Title(), err, cerr, stats(rd), d1.Round(time.Millisecond))
		fmt.Printf("%-10s %-7s full-page:   convErr=%v %s %v\n", name, "", ferr, stats(full), d2.Round(time.Millisecond))
		fmt.Printf("%-10s %-7s main-first:  via=%s convErr=%v %s %v\n", name, "", how, merr, stats(mm), d3.Round(time.Millisecond))
		os.WriteFile(filepath.Join(out, name+".main.md"), []byte(mm), 0o644)
		os.WriteFile(filepath.Join(out, name+".readability.md"), []byte("# "+art.Title()+"\n\n"+rd), 0o644)
		os.WriteFile(filepath.Join(out, name+".full.md"), []byte(full), 0o644)
	}
}

func attr(n *html.Node, k string) string {
	for _, a := range n.Attr {
		if a.Key == k {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, c string) bool {
	for _, f := range strings.Fields(attr(n, "class")) {
		if f == c {
			return true
		}
	}
	return false
}

func find(n *html.Node, pred func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && pred(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if r := find(c, pred); r != nil {
			return r
		}
	}
	return nil
}

// pickMain prefers explicit content containers, most specific first.
func pickMain(doc *html.Node) *html.Node {
	preds := []func(*html.Node) bool{
		func(n *html.Node) bool { return hasClass(n, "markdown-body") },
		func(n *html.Node) bool { return n.Data == "article" },
		func(n *html.Node) bool { return n.Data == "main" },
		func(n *html.Node) bool { return attr(n, "role") == "main" },
		func(n *html.Node) bool { return attr(n, "id") == "content" || attr(n, "id") == "main-content" },
	}
	for _, p := range preds {
		if n := find(doc, p); n != nil {
			return n
		}
	}
	return nil
}

func describe(n *html.Node) string {
	d := n.Data
	if id := attr(n, "id"); id != "" {
		d += "#" + id
	}
	if c := attr(n, "class"); c != "" {
		d += "." + strings.Fields(c)[0]
	}
	return d
}

var chrome = map[string]bool{"nav": true, "header": false, "footer": true, "aside": true, "script": true, "style": true, "noscript": true, "form": true, "button": true, "svg": true, "iframe": true}

func stripChrome(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && (chrome[c.Data] || attr(c, "role") == "navigation" || attr(c, "aria-hidden") == "true" || hasClass(c, "mw-editsection") || hasClass(c, "navbox") || attr(c, "hidden") != "" || hasClass(c, "vector-dropdown") || hasClass(c, "mw-indicators") || hasClass(c, "noprint")) {
			n.RemoveChild(c)
		} else {
			stripChrome(c)
		}
		c = next
	}
}

// resolveLinks makes href/src absolute against the page URL (fragments stay on the page) and drops permalink anchors.
func resolveLinks(n *html.Node, base *url.URL) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && c.Data == "a" {
			t := strings.TrimSpace(textOf(c))
			if t == "¶" || t == "#" || t == "§" || hasClass(c, "anchor") || hasClass(c, "headerlink") {
				n.RemoveChild(c)
				c = next
				continue
			}
		}
		if c.Type == html.ElementNode {
			for i, a := range c.Attr {
				if a.Key == "href" || a.Key == "src" {
					if r, err := base.Parse(a.Val); err == nil && (r.Scheme == "http" || r.Scheme == "https") {
						c.Attr[i].Val = r.String()
					} else if err == nil && r.Scheme != "mailto" {
						c.Attr[i].Val = ""
					}
				}
			}
			resolveLinks(c, base)
		}
		c = next
	}
}

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

// markLayoutTables flags tables without header cells as presentation so they convert as text.
func markLayoutTables(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "table" && find(n, func(x *html.Node) bool { return x.Data == "th" }) == nil {
		n.Attr = append(n.Attr, html.Attribute{Key: "role", Val: "presentation"})
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		markLayoutTables(c)
	}
}
