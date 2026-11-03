package resource

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// Budget bounds the complete model-visible result, including its framing.
type Budget struct {
	Bytes int
	Lines int
}

// Page contains exact source coverage and honest completeness facts.
type Page struct {
	RequestedURI      string  `json:"requestedUri"`
	CanonicalURI      string  `json:"canonicalUri"`
	Revision          string  `json:"revision"`
	CoordinateSpace   string  `json:"coordinateSpace"`
	ShownRanges       []Range `json:"shownRanges"`
	FullyShownRanges  []Range `json:"fullyShownRanges"`
	TotalLines        int     `json:"totalLines"`
	SelectionComplete bool    `json:"selectionComplete"`
	ResourceComplete  bool    `json:"resourceComplete"`
	Continuation      string  `json:"continuation,omitempty"`
	Truncation        string  `json:"truncation,omitempty"`
	Content           string  `json:"-"`
}

// ReadFile pages a regular UTF-8 file with constant source-size-independent memory.
func ReadFile(ctx context.Context, target Target, budget Budget) (Page, error) {
	page := Page{RequestedURI: target.Requested, CoordinateSpace: "source-lines", ShownRanges: []Range{}, FullyShownRanges: []Range{}}
	if target.Scheme != "file" {
		return page, fmt.Errorf("file reader cannot read scheme %q", target.Scheme)
	}
	canonical, err := filepath.EvalSymlinks(target.Path)
	if err != nil {
		return page, fmt.Errorf("resolve file: %w", err)
	}
	f, opened, err := fileio.OpenRegular(canonical)
	if err != nil {
		return page, err
	}
	defer f.Close()
	digest, total, err := inspect(ctx, f)
	if err != nil {
		return page, err
	}
	page.CanonicalURI = FileURI(canonical)
	page.Revision = hex.EncodeToString(digest[:])
	page.TotalLines = total
	ranges, err := target.Selection.Resolve(total, true)
	if err != nil {
		return page, err
	}
	if budget.Bytes <= 0 || budget.Lines <= 0 {
		return page, errors.New("read budget must be positive")
	}
	header := "[" + displayText(page.CanonicalURI) + "]\n"
	if target.Selection.Raw {
		header = ""
	}
	const footerBytes = 256
	available := budget.Bytes - len(header) - footerBytes
	lineBudget := budget.Lines - 3
	if available < 1 || lineBudget < 1 {
		return page, errors.New("read budget too small for source identity and continuation")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return page, err
	}
	h := sha256.New()
	reader := bufio.NewReaderSize(io.TeeReader(f, h), 64*1024)
	var body strings.Builder
	body.Grow(min(available, 64*1024))
	line := 1
	rangeIndex := 0
	next := 0
	exhausted := false
	selectedCount := 0
	shownCount := 0
	for _, r := range ranges {
		selectedCount += r.End - r.Start + 1
	}
	for {
		if err := ctx.Err(); err != nil {
			return page, err
		}
		for rangeIndex < len(ranges) && line > ranges[rangeIndex].End {
			rangeIndex++
		}
		selected := rangeIndex < len(ranges) && line >= ranges[rangeIndex].Start
		var prefix string
		if selected && !target.Selection.Raw {
			prefix = strconv.Itoa(line) + ":"
		}
		var kept []byte
		whole := true
		had := false
		for {
			if err := ctx.Err(); err != nil {
				return page, err
			}
			chunk, e := reader.ReadSlice('\n')
			had = had || len(chunk) > 0
			if selected && !exhausted {
				room := available - body.Len() - len(prefix) - len(kept)
				take := min(max(room, 0), len(chunk))
				kept = append(kept, chunk[:take]...)
				if take < len(chunk) {
					whole = false
				}
			}
			if errors.Is(e, bufio.ErrBufferFull) {
				continue
			}
			if e != nil && !errors.Is(e, io.EOF) {
				return page, e
			}
			if selected && had {
				if exhausted {
					if next == 0 {
						next = line
					}
				} else {
					if lineBudget <= 0 || available-body.Len() <= len(prefix) {
						exhausted = true
						next = line
						page.Truncation = "budget"
					} else {
						if !utf8.Valid(kept) {
							for len(kept) > 0 && !utf8.Valid(kept) {
								kept = kept[:len(kept)-1]
							}
							whole = false
						}
						if !target.Selection.Raw {
							kept = bytes.TrimSuffix(kept, []byte{'\n'})
							kept = bytes.TrimSuffix(kept, []byte{'\r'})
						}
						extra := 0
						if !target.Selection.Raw {
							extra = 1
						}
						if body.Len()+len(prefix)+len(kept)+extra > available {
							if len(kept) > 0 {
								kept = kept[:len(kept)-1]
								for !utf8.Valid(kept) {
									kept = kept[:len(kept)-1]
								}
							}
							whole = false
						}
						body.WriteString(prefix)
						body.Write(kept)
						if !target.Selection.Raw {
							body.WriteByte('\n')
						}
						appendSpan(&page.ShownRanges, line)
						if whole {
							appendSpan(&page.FullyShownRanges, line)
							shownCount++
						} else {
							next = line
							exhausted = true
							page.Truncation = "partialLine"
						}
						lineBudget--
					}
				}
			}
			if errors.Is(e, io.EOF) {
				goto finished
			}
			break
		}
		line++
	}
finished:
	after, err := f.Stat()
	if err != nil {
		return page, err
	}
	if !bytes.Equal(h.Sum(nil), digest[:]) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return Page{}, errors.New("staleRevision: file changed while reading; retry")
	}
	page.SelectionComplete = shownCount == selectedCount
	page.ResourceComplete = page.SelectionComplete && len(ranges) == 1 && ranges[0].Start == 1 && ranges[0].End == total
	if total == 0 {
		page.ResourceComplete = true
		page.SelectionComplete = true
	}
	footer := ""
	if !page.SelectionComplete {
		suffix := continuationSelection(ranges, next, target.Selection.Raw)
		page.Continuation = page.CanonicalURI + suffix
		footer = "\n… incomplete; continue with selector " + suffix + " on the same resource"
		if page.Truncation == "partialLine" {
			footer += " (partial line repeated; increase byte budget to progress)"
		}
	}
	page.Content = header + body.String() + footer
	if len(page.Content) > budget.Bytes {
		return Page{}, errors.New("internal read framing exceeded byte budget")
	}
	return page, nil
}

func inspect(ctx context.Context, r io.Reader) ([32]byte, int, error) {
	h := sha256.New()
	buffer := make([]byte, 64*1024)
	pending := make([]byte, 0, 4)
	lines := 0
	last := byte(0)
	any := false
	for {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, 0, err
		}
		n, err := r.Read(buffer)
		if n > 0 {
			data := buffer[:n]
			h.Write(data)
			lines += bytes.Count(data, []byte{'\n'})
			last = data[n-1]
			any = true
			if bytes.IndexByte(data, 0) >= 0 {
				return [32]byte{}, 0, errors.New("binary source contains NUL; choose a binary adapter")
			}
			if len(pending) > 0 {
				data = append(pending, data...)
				pending = nil
			}
			for len(data) > 0 {
				if !utf8.FullRune(data) {
					pending = append(pending, data...)
					break
				}
				rune, size := utf8.DecodeRune(data)
				if rune == utf8.RuneError && size == 1 {
					return [32]byte{}, 0, errors.New("source is not valid UTF-8")
				}
				data = data[size:]
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return [32]byte{}, 0, err
		}
	}
	if len(pending) > 0 {
		return [32]byte{}, 0, errors.New("source ends with incomplete UTF-8")
	}
	if any && last != '\n' {
		lines++
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest, lines, nil
}

func appendSpan(spans *[]Range, line int) {
	if len(*spans) > 0 && (*spans)[len(*spans)-1].End == line-1 {
		(*spans)[len(*spans)-1].End = line
		return
	}
	*spans = append(*spans, Range{Start: line, End: line})
}
func continuationSelection(ranges []Range, next int, raw bool) string {
	var parts []string
	for _, r := range ranges {
		if r.End >= next {
			parts = append(parts, fmt.Sprintf("%d-%d", max(next, r.Start), r.End))
		}
	}
	result := ":" + strings.Join(parts, ",")
	if raw {
		result += ":raw"
	}
	return result
}

func displayText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r >= 127 && r <= 159 {
			return -1
		}
		return r
	}, s)
}
