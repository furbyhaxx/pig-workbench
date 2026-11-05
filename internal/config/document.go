package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// MaxDocumentBytes bounds the parsing of one settings document.
const MaxDocumentBytes = 1048576

// Error reports configuration location and remediation without source values.
type Error struct {
	Path   string
	Key    string
	Line   int
	Column int
	Reason string
}

// Error formats a source-safe diagnostic.
func (e *Error) Error() string {
	where := e.Path
	if where == "" {
		where = "runtime settings"
	}
	if e.Line > 0 {
		where += fmt.Sprintf(":%d:%d", e.Line, e.Column)
	}
	if e.Key != "" {
		where += " (" + e.Key + ")"
	}
	return where + ": " + e.Reason
}

type document struct {
	canonical string
	raw       []byte
	path      string
	revision  string
	tree      map[string]any
	values    map[string]any
	warnings  []string
	ast       *tomledit.Document
}

func readDocument(path string, definitions map[string]Definition) (*document, error) {
	return readDocumentAs(path, definitions, false)
}

// readDocumentAs parses one settings file, restricting which origins it may carry.
func readDocumentAs(path string, definitions map[string]Definition, secrets bool) (*document, error) {
	if path == "" {
		return parseDocumentAs(path, nil, definitions, secrets)
	}
	canonical, err := resolveTarget(path)
	if err != nil {
		return nil, err
	}
	f, _, err := fileio.OpenRegular(canonical)
	if errors.Is(err, os.ErrNotExist) {
		doc, err := parseDocumentAs(path, nil, definitions, secrets)
		if err == nil {
			doc.canonical = canonical
		}
		return doc, err
	}
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read configuration: %w", err)
	}
	after, err := resolveTarget(path)
	if err != nil {
		return nil, err
	}
	if after != canonical {
		return nil, errors.New("configuration target changed while reading; reload")
	}
	doc, err := parseDocumentAs(path, data, definitions, secrets)
	if err == nil {
		doc.canonical = canonical
	}
	return doc, err
}

func parseDocument(path string, data []byte, definitions map[string]Definition) (*document, error) {
	return parseDocumentAs(path, data, definitions, false)
}

func parseDocumentAs(path string, data []byte, definitions map[string]Definition, secrets bool) (*document, error) {
	if len(data) > MaxDocumentBytes {
		return nil, &Error{Path: path, Reason: "settings file exceeds 1 MiB; reduce its size"}
	}
	tree := make(map[string]any)
	if _, err := toml.Decode(string(data), &tree); err != nil {
		problem := &Error{Path: path, Reason: "invalid TOML syntax or duplicate definitions; fix the document before reloading"}
		var parse toml.ParseError
		if errors.As(err, &parse) {
			problem.Line = parse.Position.Line
			problem.Column = parse.Position.Col
		}
		return nil, problem
	}
	if version, ok := tree["version"]; ok {
		if number, ok := version.(int64); !ok || number != 1 {
			return nil, &Error{Path: path, Key: "version", Reason: "expected supported integer version 1"}
		}
	}
	ast, err := tomledit.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, &Error{Path: path, Reason: "round-trip parser rejected the document; use TOML 1.0 syntax"}
	}
	digest := sha256.Sum256(data)
	result := &document{raw: bytes.Clone(data), path: path, revision: hex.EncodeToString(digest[:]), tree: tree, values: make(map[string]any), ast: ast}
	var walk func([]string, any) error
	walk = func(parts []string, value any) error {
		key := parser.Key(parts).String()
		if key == "version" {
			return nil
		}
		if definition, ok := definitions[key]; ok {
			if err := definition.checkOrigin(path, secrets); err != nil {
				return err
			}
			normalized, err := definition.validate(value)
			if err != nil {
				line := 0
				if entry := ast.First(parts...); entry != nil && entry.KeyValue != nil {
					line = entry.Line
				}
				return &Error{Path: path, Key: key, Line: line, Column: 1, Reason: err.Error() + "; correct this setting"}
			}
			result.values[key] = normalized
			return nil
		}
		if table, ok := value.(map[string]any); ok && len(table) > 0 {
			keys := make([]string, 0, len(table))
			for k := range table {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if err := walk(append(append([]string(nil), parts...), k), table[k]); err != nil {
					return err
				}
			}
			return nil
		}
		if len(parts) > 0 {
			result.warnings = append(result.warnings, "unknown setting "+key+" is preserved but inactive")
		}
		return nil
	}
	if err := walk(nil, tree); err != nil {
		return nil, err
	}
	return result, nil
}

// ParseValue parses exactly one TOML setting value, not a document fragment.
func ParseValue(input string) (any, error) {
	if len(input) > MaxDocumentBytes {
		return nil, errors.New("setting value exceeds the configuration size limit")
	}
	var values map[string]any
	if _, err := toml.Decode("value = "+input, &values); err != nil {
		return nil, errors.New("invalid TOML value; use quoted strings, booleans, integers or arrays")
	}
	value, ok := values["value"]
	if !ok || len(values) != 1 {
		return nil, errors.New("expected one TOML value, not additional settings")
	}
	return value, nil
}

func encodeValue(value any) (parser.Value, error) {
	var buffer bytes.Buffer
	if err := toml.NewEncoder(&buffer).Encode(map[string]any{"value": value}); err != nil {
		return parser.Value{}, errors.New("cannot encode setting value")
	}
	_, encoded, ok := strings.Cut(buffer.String(), " = ")
	if !ok {
		return parser.Value{}, errors.New("TOML encoder did not produce a scalar assignment")
	}
	return parser.ParseValue(strings.TrimSpace(encoded))
}

func editDocument(doc *document, edits []Edit) ([]byte, error) {
	for _, edit := range edits {
		parts := strings.Split(edit.Key, ".")
		entry := doc.ast.First(parts...)
		if edit.Reset {
			if entry != nil {
				comments := append(parser.Comments(nil), entry.Block...)
				if entry.Value.Trailer != "" {
					comments = append(comments, parser.CleanTrailer(entry.Value.Trailer))
				}
				if len(comments) > 0 {
					for i, item := range entry.Section.Items {
						if item == entry.KeyValue {
							entry.Section.Items[i] = comments
							break
						}
					}
				}
				entry.Remove()
			}
			continue
		}
		value, err := encodeValue(edit.Value)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			if entry.KeyValue == nil {
				return nil, errors.New("cannot replace a settings table with a scalar")
			}
			entry.Value.X = value.X
			continue
		}
		inserted := false
		for i := len(parts) - 1; i > 0; i-- {
			parent := doc.ast.First(parts[:i]...)
			if parent == nil || parent.KeyValue == nil {
				continue
			}
			if inline, ok := parent.Value.X.(parser.Inline); ok {
				parent.Value.X = append(inline, &parser.KeyValue{Name: parser.Key(parts[i:]), Value: value})
				inserted = true
				break
			}
		}
		if inserted {
			continue
		}
		table := doc.ast.Global
		if len(parts) > 1 {
			name := parser.Key(parts[:len(parts)-1])
			table = nil
			for _, section := range doc.ast.Sections {
				if section.TableName().Equals(name) {
					table = section
					break
				}
			}
			if table == nil {
				table = &tomledit.Section{Heading: &parser.Heading{Name: name}}
				doc.ast.Sections = append(doc.ast.Sections, table)
			}
		}
		table.Items = append(table.Items, &parser.KeyValue{Name: parser.Key{parts[len(parts)-1]}, Value: value})
	}
	if doc.ast.First("version") == nil {
		value, err := encodeValue(int64(1))
		if err != nil {
			return nil, err
		}
		doc.ast.Global.Items = append([]parser.Item{&parser.KeyValue{Name: parser.Key{"version"}, Value: value}}, doc.ast.Global.Items...)
	}
	var output bytes.Buffer
	if err := tomledit.Format(&output, doc.ast); err != nil {
		return nil, errors.New("round-trip settings edit failed; original file is unchanged")
	}
	return output.Bytes(), nil
}
