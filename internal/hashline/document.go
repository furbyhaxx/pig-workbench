package hashline

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// OpKind identifies a canonical patch operation or register-paste target.
type OpKind string

// Canonical patch operation kinds.
const (
	PUT_RANGE         OpKind = "PUT_RANGE"
	PUT_INSERT_BEFORE OpKind = "PUT_INSERT_BEFORE"
	PUT_INSERT_AFTER  OpKind = "PUT_INSERT_AFTER"
	PUT_BLOCK         OpKind = "PUT_BLOCK"
	PUT_BLOCK_AFTER   OpKind = "PUT_BLOCK_AFTER"
	CUT_RANGE         OpKind = "CUT_RANGE"
	CUT_BLOCK         OpKind = "CUT_BLOCK"
	PUT_FROM_REGISTER OpKind = "PUT_FROM_REGISTER"
	REM               OpKind = "REM"
	MV                OpKind = "MV"
)

// Op addresses the original source; Target is set only for register pastes.
// Line is one-based except for PUT_INSERT_AFTER at EOF; End is inclusive for ranges.
type Op struct {
	Kind     OpKind
	Target   OpKind
	Line     int
	End      int
	Block    bool
	Register string
	Payload  []string
	Dest     string
}

// InsertAfterEOF reports whether the operation targets the explicit EOF gap.
func (o Op) InsertAfterEOF() bool {
	return o.Kind == PUT_INSERT_AFTER && o.Line == 0
}

// Section groups operations addressed to the same tagged path.
type Section struct {
	Path   string
	Tag    string
	Quoted bool
	Ops    []Op
}

// Header renders the section's tagged path.
func (s Section) Header() string {
	return Header(s.Path, s.Tag)
}

// Header renders a tagged section header, quoting the path when the grammar
// requires it. Every renderer of an editable handle must use it so a path
// containing spaces or '#' stays parseable.
func Header(path, tag string) string {
	if needsQuotes(path) || strings.Contains(path, "#") {
		path = strconv.Quote(path)
	}
	return "[" + path + "#" + tag + "]"
}

// Document is an ordered batch of tagged sections.
// NoFinalNewline preserves the absence of a terminal LF in the patch text.
type Document struct {
	Sections       []Section
	NoFinalNewline bool
}

// Paths returns distinct section paths in first-appearance order.
func (d Document) Paths() []string {
	var paths []string
	seen := make(map[string]bool, len(d.Sections))
	for _, section := range d.Sections {
		if !seen[section.Path] {
			seen[section.Path] = true
			paths = append(paths, section.Path)
		}
	}
	return paths
}

// ParseError identifies an invalid patch line and explains the problem.
type ParseError struct {
	Line    int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

func needsQuotes(value string) bool {
	return value == "" || strings.ContainsRune(value, '"') || strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0
}
