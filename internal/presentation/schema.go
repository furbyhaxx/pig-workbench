// Package presentation strictly decodes SPEC-0003 §3 semantic presentation snapshots.
//
// A producer places a snapshot in tool Details under workbenchPresentation; resolvers,
// renderers and lifecycle tracking consume the decoded values. Decode performs no I/O,
// holds no SDK types, keeps no state and never infers a fact the producer omitted.
package presentation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// Version is the only supported snapshot schema version.
	Version = 1

	// MaxPayloadBytes is the SPEC-0003 maximum presentation payload size.
	MaxPayloadBytes = 64 * 1024

	// MaxBlocks is the SPEC-0003 maximum block count per snapshot.
	MaxBlocks = 32

	// MaxMetrics is the SPEC-0003 maximum metric count per snapshot.
	MaxMetrics = 16

	// MaxTableRows is the SPEC-0003 maximum row count per table block.
	MaxTableRows = 100

	// MaxDepth is the SPEC-0003 maximum snapshot nesting depth, counting the decoded snapshot as 1.
	MaxDepth = 4

	maxIdentifierBytes = 128
)

var (
	identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
	currencyPattern   = regexp.MustCompile(`^[A-Z]{3}$`)
	mediaTypePattern  = regexp.MustCompile(`^[A-Za-z0-9!#$&^_.+-]{1,127}/[A-Za-z0-9!#$&^_.+-]{1,127}$`)
	rawMessageType    = reflect.TypeOf(json.RawMessage{})
)

// ErrInvalid reports a payload that does not satisfy the SPEC-0003 §3 contract.
var ErrInvalid = errors.New("invalid presentation snapshot")

func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// nestedf prefixes a nested decode error with its field path.
func nestedf(field string, err error) error {
	return invalidf("%s: %s", field, strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": "))
}

// Phase is the lifecycle phase a snapshot reports.
type Phase string

// The phases a snapshot may report.
const (
	PhasePreparing Phase = "preparing"
	PhaseRunning   Phase = "running"
	PhaseSettled   Phase = "settled"
)

// Outcome is the terminal result of a settled call.
type Outcome string

// The terminal outcomes a settled snapshot may report; unknown is never success.
const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
	OutcomeTimedOut  Outcome = "timedOut"
	OutcomeDenied    Outcome = "denied"
	OutcomeUnknown   Outcome = "unknown"
)

// Effects reports whether a call changed state.
type Effects string

// The side-effect states a snapshot may report.
const (
	EffectsNone      Effects = "none"
	EffectsCommitted Effects = "committed"
	EffectsPartial   Effects = "partial"
	EffectsUnknown   Effects = "unknown"
)

// TargetKind is the kind of structured target a snapshot names.
type TargetKind string

// The target kinds a snapshot may report.
const (
	TargetPath  TargetKind = "path"
	TargetURI   TargetKind = "uri"
	TargetLabel TargetKind = "label"
)

// MetricUnit is a metric's unit: one of the known units or an ISO 4217 currency code.
type MetricUnit string

// The units SPEC-0003 §3 names; any other value must be a three-letter uppercase currency code.
const (
	UnitCount        MetricUnit = "count"
	UnitBytes        MetricUnit = "bytes"
	UnitLines        MetricUnit = "lines"
	UnitMilliseconds MetricUnit = "ms"
	UnitPercent      MetricUnit = "percent"
)

// BlockKind is the kind of one presentation block.
type BlockKind string

// The block kinds SPEC-0003 §3 names.
const (
	BlockText     BlockKind = "text"
	BlockCode     BlockKind = "code"
	BlockDiff     BlockKind = "diff"
	BlockTable    BlockKind = "table"
	BlockImageRef BlockKind = "imageRef"
	BlockNotice   BlockKind = "notice"
	BlockChildren BlockKind = "children"
)

// TextRole is the semantic emphasis of a text block.
type TextRole string

// The roles a text block may declare; empty means no declared role.
const (
	TextRolePlain   TextRole = "plain"
	TextRoleInfo    TextRole = "info"
	TextRoleWarning TextRole = "warning"
	TextRoleError   TextRole = "error"
)

// Severity is a notice's declared level.
type Severity string

// The severities a notice may declare.
const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// DiffOperation is the file operation a diff block describes.
type DiffOperation string

// The file operations a diff block may describe; other covers operations the set does not name.
const (
	DiffCreate DiffOperation = "create"
	DiffUpdate DiffOperation = "update"
	DiffDelete DiffOperation = "delete"
	DiffMove   DiffOperation = "move"
	DiffCopy   DiffOperation = "copy"
	DiffOther  DiffOperation = "other"
)

// HunkLineKind classifies one diff hunk line.
type HunkLineKind string

// The hunk line kinds a diff block may carry.
const (
	HunkContext HunkLineKind = "context"
	HunkAdd     HunkLineKind = "add"
	HunkRemove  HunkLineKind = "remove"
)

// CellKind is the type of one table cell.
type CellKind string

// The cell kinds SPEC-0003 §3 names.
const (
	CellText     CellKind = "text"
	CellCount    CellKind = "count"
	CellBytes    CellKind = "bytes"
	CellDuration CellKind = "duration"
	CellPath     CellKind = "path"
	CellURI      CellKind = "uri"
	CellLabel    CellKind = "label"
)

// Snapshot is one decoded presentation snapshot; absent fields stay absent.
type Snapshot struct {
	Version      int                `json:"version"`
	Revision     int64              `json:"revision"`
	Phase        Phase              `json:"phase"`
	Outcome      *Outcome           `json:"outcome,omitempty"`
	Title        string             `json:"title,omitempty"`
	Target       *Target            `json:"target,omitempty"`
	Summary      string             `json:"summary,omitempty"`
	Complete     *bool              `json:"complete,omitempty"`
	Effects      *Effects           `json:"effects,omitempty"`
	Metrics      []Metric           `json:"metrics,omitempty"`
	Blocks       []Block            `json:"blocks,omitempty"`
	Continuation *Continuation      `json:"continuation,omitempty"`
	Error        *PresentationError `json:"error,omitempty"`
	Provenance   *Provenance        `json:"provenance,omitempty"`
	ParentCallID string             `json:"parentCallId,omitempty"`
}

// Target is the structured subject of a call; Value retains the original spelling.
type Target struct {
	Kind  TargetKind `json:"kind"`
	Value string     `json:"value"`
}

// Metric is one typed known fact about a call.
type Metric struct {
	ID    string     `json:"id"`
	Value float64    `json:"value"`
	Unit  MetricUnit `json:"unit"`
}

// Continuation explains omitted content and how to reach it.
type Continuation struct {
	URI         string `json:"uri,omitempty"`
	Cursor      string `json:"cursor,omitempty"`
	Explanation string `json:"explanation"`
}

// PresentationError is a stable code with an actionable message.
type PresentationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Provenance names the resource revision and source a snapshot derives from.
type Provenance struct {
	Revision string            `json:"revision,omitempty"`
	Source   *SourceCoordinate `json:"source,omitempty"`
	Adapter  string            `json:"adapter,omitempty"`
	View     string            `json:"view,omitempty"`
}

// SourceCoordinate locates a snapshot inside a resource.
type SourceCoordinate struct {
	URI     string `json:"uri"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"endLine,omitempty"`
}

// Block is one ordered presentation block; exactly one payload matches Kind.
type Block struct {
	Kind     BlockKind
	Text     *TextBlock
	Code     *CodeBlock
	Diff     *DiffBlock
	Table    *TableBlock
	Image    *ImageRefBlock
	Notice   *NoticeBlock
	Children *ChildrenBlock
}

// TextBlock is bounded plain text with an optional semantic role.
type TextBlock struct {
	Text string
	Role TextRole
}

// CodeBlock is bounded source text with an optional language.
type CodeBlock struct {
	Text     string
	Language string
}

// NoticeBlock is a bounded message with a declared severity.
type NoticeBlock struct {
	Text     string
	Severity Severity
}

// DiffBlock is a file operation with before/after identities and bounded hunks.
type DiffBlock struct {
	Operation DiffOperation `json:"operation"`
	Before    *FileIdentity `json:"before,omitempty"`
	After     *FileIdentity `json:"after,omitempty"`
	Hunks     []Hunk        `json:"hunks,omitempty"`
}

// FileIdentity names one side of a file operation.
type FileIdentity struct {
	Path     string `json:"path"`
	Revision string `json:"revision,omitempty"`
}

// Hunk is one bounded diff hunk.
type Hunk struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []HunkLine `json:"lines"`
}

// HunkLine is one classified diff line.
type HunkLine struct {
	Kind HunkLineKind `json:"kind"`
	Text string       `json:"text"`
}

// TableBlock is columns plus typed rows.
type TableBlock struct {
	Columns []Column `json:"columns"`
	Rows    [][]Cell `json:"rows"`
}

// Column declares a table column and the cell kind its rows carry.
type Column struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Cell  CellKind `json:"cell"`
}

// ImageRefBlock references an existing content block or resource; it never embeds image data.
type ImageRefBlock struct {
	Resource  string `json:"resource,omitempty"`
	BlockID   string `json:"blockId,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Alt       string `json:"alt,omitempty"`
}

// ChildrenBlock lists child call IDs and the child snapshots retained for replay.
type ChildrenBlock struct {
	ChildCallIDs []string        `json:"childCallIds"`
	Retained     []RetainedChild `json:"retained,omitempty"`
}

// RetainedChild is one child call's retained presentation snapshot.
type RetainedChild struct {
	CallID   string    `json:"callId"`
	Snapshot *Snapshot `json:"snapshot"`
}

// Cell is one typed table cell; Text holds text, label, path and uri cells, Number holds
// count, bytes and duration cells.
type Cell struct {
	Kind   CellKind
	Text   string
	Number float64
}

type encodedBlock struct {
	Kind     BlockKind      `json:"kind"`
	Text     string         `json:"text,omitempty"`
	Role     TextRole       `json:"role,omitempty"`
	Language string         `json:"language,omitempty"`
	Severity Severity       `json:"severity,omitempty"`
	Diff     *DiffBlock     `json:"diff,omitempty"`
	Table    *TableBlock    `json:"table,omitempty"`
	Image    *ImageRefBlock `json:"image,omitempty"`
	Children *ChildrenBlock `json:"children,omitempty"`
}

// MarshalJSON writes the flat SPEC-0003 §3 block shape for the payload that matches Kind.
func (b Block) MarshalJSON() ([]byte, error) {
	encoded := encodedBlock{Kind: b.Kind}
	switch b.Kind {
	case BlockText:
		if b.Text == nil {
			return nil, errors.New("presentation: text block without payload")
		}
		encoded.Text, encoded.Role = b.Text.Text, b.Text.Role
	case BlockCode:
		if b.Code == nil {
			return nil, errors.New("presentation: code block without payload")
		}
		encoded.Text, encoded.Language = b.Code.Text, b.Code.Language
	case BlockNotice:
		if b.Notice == nil {
			return nil, errors.New("presentation: notice block without payload")
		}
		encoded.Text, encoded.Severity = b.Notice.Text, b.Notice.Severity
	case BlockDiff:
		if b.Diff == nil {
			return nil, errors.New("presentation: diff block without payload")
		}
		encoded.Diff = b.Diff
	case BlockTable:
		if b.Table == nil {
			return nil, errors.New("presentation: table block without payload")
		}
		encoded.Table = b.Table
	case BlockImageRef:
		if b.Image == nil {
			return nil, errors.New("presentation: imageRef block without payload")
		}
		encoded.Image = b.Image
	case BlockChildren:
		if b.Children == nil {
			return nil, errors.New("presentation: children block without payload")
		}
		encoded.Children = b.Children
	default:
		return nil, fmt.Errorf("presentation: unsupported block kind %q", string(b.Kind))
	}
	return json.Marshal(encoded)
}

type encodedCell struct {
	Kind  CellKind `json:"kind"`
	Value any      `json:"value"`
}

// MarshalJSON writes the {kind, value} shape SPEC-0003 §3 uses for typed cells.
func (c Cell) MarshalJSON() ([]byte, error) {
	var value any
	switch c.Kind {
	case CellText, CellLabel, CellPath, CellURI:
		value = c.Text
	case CellCount, CellBytes, CellDuration:
		value = c.Number
	default:
		return nil, fmt.Errorf("presentation: unsupported cell kind %q", string(c.Kind))
	}
	return json.Marshal(encodedCell{Kind: c.Kind, Value: value})
}

// Decode strictly decodes one presentation snapshot.
//
// It accepts the snapshot object itself or any details object that carries the snapshot at
// the default detailsPath /workbenchPresentation (SPEC-0003 §2). Unknown fields, duplicate
// keys, null values, unknown kinds, control characters and every SPEC-0003 §3 bound are
// rejected; the result shares no memory with data.
func Decode(data []byte) (Snapshot, error) {
	if len(data) > MaxPayloadBytes {
		return Snapshot{}, invalidf("snapshot: exceeds %d bytes", MaxPayloadBytes)
	}
	if !utf8.Valid(data) {
		return Snapshot{}, invalidf("snapshot: invalid UTF-8")
	}
	if err := checkRawBytes(data); err != nil {
		return Snapshot{}, err
	}
	data, err := unwrapDetails(data)
	if err != nil {
		return Snapshot{}, err
	}
	members := json.NewDecoder(bytes.NewReader(data))
	members.UseNumber()
	if err := checkJSONValue(members, reflect.TypeOf(wireSnapshot{}), "snapshot"); err != nil {
		return Snapshot{}, invalidf("decode: %s", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire wireSnapshot
	if err := decoder.Decode(&wire); err != nil {
		return Snapshot{}, invalidf("decode: %s", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Snapshot{}, invalidf("decode: unexpected trailing JSON")
	}
	return wire.convert(1)
}

// unwrapDetails returns the snapshot carried at the default detailsPath.
func unwrapDetails(data []byte) ([]byte, error) {
	var details map[string]json.RawMessage
	if err := json.Unmarshal(data, &details); err != nil {
		return data, nil
	}
	snapshot, ok := details["workbenchPresentation"]
	if !ok {
		return data, nil
	}
	if len(snapshot) == 0 || snapshot[0] != '{' {
		return nil, invalidf("workbenchPresentation: must be an object")
	}
	return snapshot, nil
}

// checkRawBytes rejects raw control bytes, which JSON only carries in escaped form.
func checkRawBytes(data []byte) error {
	for _, b := range data {
		if (b < 0x20 && b != '\n' && b != '\r' && b != '\t') || b == 0x7f {
			return invalidf("snapshot: raw control characters are not allowed")
		}
	}
	return nil
}

// checkJSONValue rejects unknown and duplicate object members and null values, which
// encoding/json would otherwise accept silently.
func checkJSONValue(decoder *json.Decoder, typ reflect.Type, path string) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == rawMessageType {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token == nil {
			return fmt.Errorf("%s: null is not allowed", path)
		}
		if _, isDelim := token.(json.Delim); isDelim {
			return fmt.Errorf("%s: must be a scalar", path)
		}
		return nil
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("%s: null is not allowed", path)
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		if typ.Kind() != reflect.Struct {
			return fmt.Errorf("%s: unexpected object", path)
		}
		fields := make(map[string]reflect.Type, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			fields[name] = field.Type
		}
		seen := make(map[string]bool, len(fields))
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name := nameToken.(string)
			fieldPath := fmt.Sprintf("%s.%q", path, displayName(name))
			fieldType, ok := fields[name]
			if !ok {
				return fmt.Errorf("%s: unknown field", fieldPath)
			}
			if seen[name] {
				return fmt.Errorf("%s: duplicate field", fieldPath)
			}
			seen[name] = true
			if err := checkJSONValue(decoder, fieldType, fieldPath); err != nil {
				return err
			}
		}
	case '[':
		if typ.Kind() != reflect.Slice {
			return fmt.Errorf("%s: unexpected array", path)
		}
		for i := 0; decoder.More(); i++ {
			if err := checkJSONValue(decoder, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s: unexpected delimiter", path)
	}
	_, err = decoder.Token()
	return err
}

func displayName(name string) string {
	runes := []rune(name)
	if len(runes) > 64 {
		return string(append(runes[:64], '…'))
	}
	return name
}

func validateText(field, value string, multiline bool) error {
	if !utf8.ValidString(value) {
		return invalidf("%s: invalid UTF-8", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return invalidf("%s: control characters are not allowed", field)
		}
	}
	return nil
}

func validateOpaqueID(field, value string) error {
	if value == "" {
		return invalidf("%s: is required", field)
	}
	if len(value) > maxIdentifierBytes {
		return invalidf("%s: exceeds %d bytes", field, maxIdentifierBytes)
	}
	if err := validateText(field, value, false); err != nil {
		return err
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return invalidf("%s: must not contain whitespace", field)
	}
	return nil
}

func validateURI(field, value string) error {
	if err := validateText(field, value, false); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return invalidf("%s: must be an absolute URI", field)
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return invalidf("%s: must not contain whitespace", field)
	}
	return nil
}

func (u MetricUnit) valid() bool {
	switch u {
	case UnitCount, UnitBytes, UnitLines, UnitMilliseconds, UnitPercent:
		return true
	}
	return currencyPattern.MatchString(string(u))
}

// integralUnit reports units that count discrete items and reject fractions.
func (u MetricUnit) integralUnit() bool {
	return u == UnitCount || u == UnitBytes || u == UnitLines
}

func (k CellKind) valid() bool {
	switch k {
	case CellText, CellCount, CellBytes, CellDuration, CellPath, CellURI, CellLabel:
		return true
	}
	return false
}

type wireSnapshot struct {
	Version      *int              `json:"version"`
	Revision     *int64            `json:"revision"`
	Phase        string            `json:"phase"`
	Outcome      *string           `json:"outcome"`
	Title        string            `json:"title"`
	Target       *wireTarget       `json:"target"`
	Summary      string            `json:"summary"`
	Complete     *bool             `json:"complete"`
	Effects      *string           `json:"effects"`
	Metrics      []wireMetric      `json:"metrics"`
	Blocks       []wireBlock       `json:"blocks"`
	Continuation *wireContinuation `json:"continuation"`
	Error        *wireError        `json:"error"`
	Provenance   *wireProvenance   `json:"provenance"`
	ParentCallID string            `json:"parentCallId"`
}

func (w wireSnapshot) convert(depth int) (Snapshot, error) {
	if w.Version == nil {
		return Snapshot{}, invalidf("version: is required")
	}
	if *w.Version != Version {
		return Snapshot{}, invalidf("version: must be %d", Version)
	}
	if w.Revision == nil {
		return Snapshot{}, invalidf("revision: is required")
	}
	if *w.Revision < 0 {
		return Snapshot{}, invalidf("revision: must not be negative")
	}
	phase := Phase(w.Phase)
	switch phase {
	case PhasePreparing, PhaseRunning, PhaseSettled:
	default:
		return Snapshot{}, invalidf("phase: must be preparing, running or settled")
	}
	snapshot := Snapshot{Version: *w.Version, Revision: *w.Revision, Phase: phase}
	if w.Outcome == nil {
		if phase == PhaseSettled {
			return Snapshot{}, invalidf("outcome: is required when phase is settled")
		}
	} else {
		if phase != PhaseSettled {
			return Snapshot{}, invalidf("outcome: is only allowed when phase is settled")
		}
		outcome := Outcome(*w.Outcome)
		switch outcome {
		case OutcomeSucceeded, OutcomeFailed, OutcomeCancelled, OutcomeTimedOut, OutcomeDenied, OutcomeUnknown:
		default:
			return Snapshot{}, invalidf("outcome: must be succeeded, failed, cancelled, timedOut, denied or unknown")
		}
		snapshot.Outcome = &outcome
	}
	if err := validateText("title", w.Title, false); err != nil {
		return Snapshot{}, err
	}
	if err := validateText("summary", w.Summary, true); err != nil {
		return Snapshot{}, err
	}
	snapshot.Title, snapshot.Summary = w.Title, w.Summary
	if w.Target != nil {
		target, err := w.Target.convert("target")
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Target = &target
	}
	if w.Complete != nil {
		complete := *w.Complete
		snapshot.Complete = &complete
	}
	if w.Effects != nil {
		effects := Effects(*w.Effects)
		switch effects {
		case EffectsNone, EffectsCommitted, EffectsPartial, EffectsUnknown:
		default:
			return Snapshot{}, invalidf("effects: must be none, committed, partial or unknown")
		}
		snapshot.Effects = &effects
	}
	if len(w.Metrics) > MaxMetrics {
		return Snapshot{}, invalidf("metrics: exceeds %d entries", MaxMetrics)
	}
	metricIDs := make(map[string]struct{}, len(w.Metrics))
	for i, wire := range w.Metrics {
		field := fmt.Sprintf("metrics[%d]", i)
		metric, err := wire.convert(field)
		if err != nil {
			return Snapshot{}, err
		}
		if _, duplicate := metricIDs[metric.ID]; duplicate {
			return Snapshot{}, invalidf("%s.id: duplicate metric id", field)
		}
		metricIDs[metric.ID] = struct{}{}
		snapshot.Metrics = append(snapshot.Metrics, metric)
	}
	if len(w.Blocks) > MaxBlocks {
		return Snapshot{}, invalidf("blocks: exceeds %d entries", MaxBlocks)
	}
	for i, wire := range w.Blocks {
		block, err := wire.convert(fmt.Sprintf("blocks[%d]", i), depth)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Blocks = append(snapshot.Blocks, block)
	}
	if w.Continuation != nil {
		continuation, err := w.Continuation.convert("continuation")
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Continuation = &continuation
	}
	if w.Error != nil {
		failure, err := w.Error.convert("error")
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Error = &failure
	}
	if w.Provenance != nil {
		provenance, err := w.Provenance.convert("provenance")
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Provenance = &provenance
	}
	if w.ParentCallID != "" {
		if err := validateOpaqueID("parentCallId", w.ParentCallID); err != nil {
			return Snapshot{}, err
		}
		snapshot.ParentCallID = w.ParentCallID
	}
	return snapshot, nil
}

type wireTarget struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func (w wireTarget) convert(field string) (Target, error) {
	kind := TargetKind(w.Kind)
	switch kind {
	case TargetPath, TargetURI, TargetLabel:
	default:
		return Target{}, invalidf("%s.kind: must be path, uri or label", field)
	}
	if w.Value == "" {
		return Target{}, invalidf("%s.value: is required", field)
	}
	if kind == TargetURI {
		if err := validateURI(field+".value", w.Value); err != nil {
			return Target{}, err
		}
	} else if err := validateText(field+".value", w.Value, false); err != nil {
		return Target{}, err
	}
	return Target{Kind: kind, Value: w.Value}, nil
}

type wireMetric struct {
	ID    string   `json:"id"`
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
}

func (w wireMetric) convert(field string) (Metric, error) {
	if !identifierPattern.MatchString(w.ID) {
		return Metric{}, invalidf("%s.id: must match [A-Za-z][A-Za-z0-9._-]{0,63}", field)
	}
	if w.Value == nil {
		return Metric{}, invalidf("%s.value: is required", field)
	}
	if *w.Value < 0 {
		return Metric{}, invalidf("%s.value: must not be negative", field)
	}
	unit := MetricUnit(w.Unit)
	if !unit.valid() {
		return Metric{}, invalidf("%s.unit: must be count, bytes, lines, ms, percent or a three-letter currency code", field)
	}
	if unit.integralUnit() && *w.Value != math.Trunc(*w.Value) {
		return Metric{}, invalidf("%s.value: must be a whole number for %s", field, unit)
	}
	return Metric{ID: w.ID, Value: *w.Value, Unit: unit}, nil
}

type wireBlock struct {
	Kind     string        `json:"kind"`
	Text     string        `json:"text"`
	Role     string        `json:"role"`
	Language string        `json:"language"`
	Severity string        `json:"severity"`
	Diff     *wireDiff     `json:"diff"`
	Table    *wireTable    `json:"table"`
	Image    *wireImageRef `json:"image"`
	Children *wireChildren `json:"children"`
}

func (w wireBlock) convert(field string, depth int) (Block, error) {
	kind := BlockKind(w.Kind)
	switch kind {
	case BlockText:
		if err := w.exclusive(field, "text", "role"); err != nil {
			return Block{}, err
		}
		text, err := w.text(field)
		if err != nil {
			return Block{}, err
		}
		role := TextRole(w.Role)
		switch role {
		case "", TextRolePlain, TextRoleInfo, TextRoleWarning, TextRoleError:
		default:
			return Block{}, invalidf("%s.role: must be plain, info, warning or error", field)
		}
		return Block{Kind: kind, Text: &TextBlock{Text: text, Role: role}}, nil
	case BlockCode:
		if err := w.exclusive(field, "text", "language"); err != nil {
			return Block{}, err
		}
		text, err := w.text(field)
		if err != nil {
			return Block{}, err
		}
		if w.Language != "" && !identifierPattern.MatchString(w.Language) {
			return Block{}, invalidf("%s.language: must match [A-Za-z][A-Za-z0-9._-]{0,63}", field)
		}
		return Block{Kind: kind, Code: &CodeBlock{Text: text, Language: w.Language}}, nil
	case BlockNotice:
		if err := w.exclusive(field, "text", "severity"); err != nil {
			return Block{}, err
		}
		text, err := w.text(field)
		if err != nil {
			return Block{}, err
		}
		severity := Severity(w.Severity)
		switch severity {
		case SeverityInfo, SeverityWarning, SeverityError:
		default:
			return Block{}, invalidf("%s.severity: must be info, warning or error", field)
		}
		return Block{Kind: kind, Notice: &NoticeBlock{Text: text, Severity: severity}}, nil
	case BlockDiff:
		if err := w.exclusive(field, "diff"); err != nil {
			return Block{}, err
		}
		if w.Diff == nil {
			return Block{}, invalidf("%s.diff: is required", field)
		}
		diff, err := w.Diff.convert(field + ".diff")
		if err != nil {
			return Block{}, err
		}
		return Block{Kind: kind, Diff: &diff}, nil
	case BlockTable:
		if err := w.exclusive(field, "table"); err != nil {
			return Block{}, err
		}
		if w.Table == nil {
			return Block{}, invalidf("%s.table: is required", field)
		}
		table, err := w.Table.convert(field + ".table")
		if err != nil {
			return Block{}, err
		}
		return Block{Kind: kind, Table: &table}, nil
	case BlockImageRef:
		if err := w.exclusive(field, "image"); err != nil {
			return Block{}, err
		}
		if w.Image == nil {
			return Block{}, invalidf("%s.image: is required", field)
		}
		image, err := w.Image.convert(field + ".image")
		if err != nil {
			return Block{}, err
		}
		return Block{Kind: kind, Image: &image}, nil
	case BlockChildren:
		if err := w.exclusive(field, "children"); err != nil {
			return Block{}, err
		}
		if w.Children == nil {
			return Block{}, invalidf("%s.children: is required", field)
		}
		children, err := w.Children.convert(field+".children", depth)
		if err != nil {
			return Block{}, err
		}
		return Block{Kind: kind, Children: &children}, nil
	default:
		return Block{}, invalidf("%s.kind: must be text, code, diff, table, imageRef, notice or children", field)
	}
}

func (w wireBlock) text(field string) (string, error) {
	if w.Text == "" {
		return "", invalidf("%s.text: is required", field)
	}
	if err := validateText(field+".text", w.Text, true); err != nil {
		return "", err
	}
	return w.Text, nil
}

// exclusive rejects members that do not belong to the block's kind.
func (w wireBlock) exclusive(field string, allowed ...string) error {
	for _, member := range []struct {
		name  string
		found bool
	}{
		{"text", w.Text != ""},
		{"role", w.Role != ""},
		{"language", w.Language != ""},
		{"severity", w.Severity != ""},
		{"diff", w.Diff != nil},
		{"table", w.Table != nil},
		{"image", w.Image != nil},
		{"children", w.Children != nil},
	} {
		if member.found && !slices.Contains(allowed, member.name) {
			return invalidf("%s.%s: is not allowed for %s blocks", field, member.name, w.Kind)
		}
	}
	return nil
}

type wireDiff struct {
	Operation string         `json:"operation"`
	Before    *wireIdentity  `json:"before"`
	After     *wireIdentity  `json:"after"`
	Hunks     []wireHunkSpec `json:"hunks"`
}

type wireIdentity struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
}

type wireHunkSpec struct {
	OldStart int        `json:"oldStart"`
	OldLines int        `json:"oldLines"`
	NewStart int        `json:"newStart"`
	NewLines int        `json:"newLines"`
	Lines    []wireLine `json:"lines"`
}

type wireLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func (w wireDiff) convert(field string) (DiffBlock, error) {
	operation := DiffOperation(w.Operation)
	switch operation {
	case DiffCreate, DiffUpdate, DiffDelete, DiffMove, DiffCopy, DiffOther:
	default:
		return DiffBlock{}, invalidf("%s.operation: must be create, update, delete, move, copy or other", field)
	}
	diff := DiffBlock{Operation: operation}
	if w.Before != nil {
		before, err := w.Before.convert(field + ".before")
		if err != nil {
			return DiffBlock{}, err
		}
		diff.Before = &before
	}
	if w.After != nil {
		after, err := w.After.convert(field + ".after")
		if err != nil {
			return DiffBlock{}, err
		}
		diff.After = &after
	}
	if diff.Before == nil && diff.After == nil {
		return DiffBlock{}, invalidf("%s: at least one of before or after is required", field)
	}
	for i, wire := range w.Hunks {
		hunk, err := wire.convert(fmt.Sprintf("%s.hunks[%d]", field, i))
		if err != nil {
			return DiffBlock{}, err
		}
		diff.Hunks = append(diff.Hunks, hunk)
	}
	return diff, nil
}

func (w wireIdentity) convert(field string) (FileIdentity, error) {
	if w.Path == "" {
		return FileIdentity{}, invalidf("%s.path: is required", field)
	}
	if err := validateText(field+".path", w.Path, false); err != nil {
		return FileIdentity{}, err
	}
	if w.Revision != "" {
		if err := validateOpaqueID(field+".revision", w.Revision); err != nil {
			return FileIdentity{}, err
		}
	}
	return FileIdentity(w), nil
}

func (w wireHunkSpec) convert(field string) (Hunk, error) {
	for _, counter := range []struct {
		name  string
		value int
	}{
		{"oldStart", w.OldStart}, {"oldLines", w.OldLines},
		{"newStart", w.NewStart}, {"newLines", w.NewLines},
	} {
		if counter.value < 0 {
			return Hunk{}, invalidf("%s.%s: must not be negative", field, counter.name)
		}
	}
	if len(w.Lines) == 0 {
		return Hunk{}, invalidf("%s.lines: at least one line is required", field)
	}
	hunk := Hunk{OldStart: w.OldStart, OldLines: w.OldLines, NewStart: w.NewStart, NewLines: w.NewLines}
	for i, wire := range w.Lines {
		kind := HunkLineKind(wire.Kind)
		switch kind {
		case HunkContext, HunkAdd, HunkRemove:
		default:
			return Hunk{}, invalidf("%s.lines[%d].kind: must be context, add or remove", field, i)
		}
		if err := validateText(fmt.Sprintf("%s.lines[%d].text", field, i), wire.Text, true); err != nil {
			return Hunk{}, err
		}
		hunk.Lines = append(hunk.Lines, HunkLine{Kind: kind, Text: wire.Text})
	}
	return hunk, nil
}

type wireTable struct {
	Columns []wireColumn `json:"columns"`
	Rows    [][]wireCell `json:"rows"`
}

type wireColumn struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Cell  string `json:"cell"`
}

func (w wireTable) convert(field string) (TableBlock, error) {
	if len(w.Columns) == 0 {
		return TableBlock{}, invalidf("%s.columns: at least one column is required", field)
	}
	if len(w.Rows) > MaxTableRows {
		return TableBlock{}, invalidf("%s.rows: exceeds %d rows", field, MaxTableRows)
	}
	table := TableBlock{}
	columnIDs := make(map[string]struct{}, len(w.Columns))
	for i, wire := range w.Columns {
		columnField := fmt.Sprintf("%s.columns[%d]", field, i)
		if !identifierPattern.MatchString(wire.ID) {
			return TableBlock{}, invalidf("%s.id: must match [A-Za-z][A-Za-z0-9._-]{0,63}", columnField)
		}
		if _, duplicate := columnIDs[wire.ID]; duplicate {
			return TableBlock{}, invalidf("%s.id: duplicate column id", columnField)
		}
		columnIDs[wire.ID] = struct{}{}
		if wire.Label == "" {
			return TableBlock{}, invalidf("%s.label: is required", columnField)
		}
		if err := validateText(columnField+".label", wire.Label, false); err != nil {
			return TableBlock{}, err
		}
		kind := CellKind(wire.Cell)
		if !kind.valid() {
			return TableBlock{}, invalidf("%s.cell: must be text, count, bytes, duration, path, uri or label", columnField)
		}
		table.Columns = append(table.Columns, Column{ID: wire.ID, Label: wire.Label, Cell: kind})
	}
	for i, row := range w.Rows {
		if len(row) != len(w.Columns) {
			return TableBlock{}, invalidf("%s.rows[%d]: must have %d cells", field, i, len(w.Columns))
		}
		cells := make([]Cell, 0, len(row))
		for j, wire := range row {
			cellField := fmt.Sprintf("%s.rows[%d][%d]", field, i, j)
			cell, err := wire.convert(cellField)
			if err != nil {
				return TableBlock{}, err
			}
			if cell.Kind != table.Columns[j].Cell {
				return TableBlock{}, invalidf("%s.kind: must match column %q", cellField, table.Columns[j].ID)
			}
			cells = append(cells, cell)
		}
		table.Rows = append(table.Rows, cells)
	}
	return table, nil
}

type wireCell struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

func (w wireCell) convert(field string) (Cell, error) {
	kind := CellKind(w.Kind)
	if !kind.valid() {
		return Cell{}, invalidf("%s.kind: must be text, count, bytes, duration, path, uri or label", field)
	}
	if len(w.Value) == 0 {
		return Cell{}, invalidf("%s.value: is required", field)
	}
	switch kind {
	case CellText, CellLabel, CellPath, CellURI:
		var text string
		if err := json.Unmarshal(w.Value, &text); err != nil {
			return Cell{}, invalidf("%s.value: must be a string", field)
		}
		if err := validateText(field+".value", text, true); err != nil {
			return Cell{}, err
		}
		switch kind {
		case CellPath:
			if text == "" {
				return Cell{}, invalidf("%s.value: is required", field)
			}
		case CellURI:
			if err := validateURI(field+".value", text); err != nil {
				return Cell{}, err
			}
		}
		return Cell{Kind: kind, Text: text}, nil
	default:
		var number float64
		if err := json.Unmarshal(w.Value, &number); err != nil {
			return Cell{}, invalidf("%s.value: must be a number", field)
		}
		if number < 0 {
			return Cell{}, invalidf("%s.value: must not be negative", field)
		}
		if (kind == CellCount || kind == CellBytes) && number != math.Trunc(number) {
			return Cell{}, invalidf("%s.value: must be a whole number for %s cells", field, kind)
		}
		return Cell{Kind: kind, Number: number}, nil
	}
}

type wireImageRef struct {
	Resource  string `json:"resource"`
	BlockID   string `json:"blockId"`
	MediaType string `json:"mediaType"`
	Alt       string `json:"alt"`
}

func (w wireImageRef) convert(field string) (ImageRefBlock, error) {
	if w.Resource == "" && w.BlockID == "" {
		return ImageRefBlock{}, invalidf("%s: a resource or blockId reference is required", field)
	}
	if w.Resource != "" {
		if err := validateURI(field+".resource", w.Resource); err != nil {
			return ImageRefBlock{}, err
		}
	}
	if w.BlockID != "" {
		if err := validateOpaqueID(field+".blockId", w.BlockID); err != nil {
			return ImageRefBlock{}, err
		}
	}
	if w.MediaType != "" && !mediaTypePattern.MatchString(w.MediaType) {
		return ImageRefBlock{}, invalidf("%s.mediaType: must be a type/subtype token", field)
	}
	if err := validateText(field+".alt", w.Alt, false); err != nil {
		return ImageRefBlock{}, err
	}
	return ImageRefBlock(w), nil
}

type wireChildren struct {
	ChildCallIDs []string       `json:"childCallIds"`
	Retained     []wireRetained `json:"retained"`
}

type wireRetained struct {
	CallID   string        `json:"callId"`
	Snapshot *wireSnapshot `json:"snapshot"`
}

func (w wireChildren) convert(field string, depth int) (ChildrenBlock, error) {
	if len(w.ChildCallIDs) == 0 {
		return ChildrenBlock{}, invalidf("%s.childCallIds: at least one child call ID is required", field)
	}
	children := ChildrenBlock{}
	callIDs := make(map[string]struct{}, len(w.ChildCallIDs))
	for i, callID := range w.ChildCallIDs {
		if err := validateOpaqueID(fmt.Sprintf("%s.childCallIds[%d]", field, i), callID); err != nil {
			return ChildrenBlock{}, err
		}
		if _, duplicate := callIDs[callID]; duplicate {
			return ChildrenBlock{}, invalidf("%s.childCallIds[%d]: duplicate child call ID", field, i)
		}
		callIDs[callID] = struct{}{}
		children.ChildCallIDs = append(children.ChildCallIDs, callID)
	}
	if len(w.Retained) > 0 && depth+1 > MaxDepth {
		return ChildrenBlock{}, invalidf("%s: nested child snapshots exceed depth %d", field, MaxDepth)
	}
	retainedCallIDs := make(map[string]struct{}, len(w.Retained))
	for i, wire := range w.Retained {
		retainedField := fmt.Sprintf("%s.retained[%d]", field, i)
		if wire.Snapshot == nil {
			return ChildrenBlock{}, invalidf("%s.snapshot: is required", retainedField)
		}
		if err := validateOpaqueID(retainedField+".callId", wire.CallID); err != nil {
			return ChildrenBlock{}, err
		}
		if _, known := callIDs[wire.CallID]; !known {
			return ChildrenBlock{}, invalidf("%s.callId: is not a listed child call ID", retainedField)
		}
		if _, duplicate := retainedCallIDs[wire.CallID]; duplicate {
			return ChildrenBlock{}, invalidf("%s.callId: duplicate retained snapshot", retainedField)
		}
		retainedCallIDs[wire.CallID] = struct{}{}
		snapshot, err := wire.Snapshot.convert(depth + 1)
		if err != nil {
			return ChildrenBlock{}, nestedf(retainedField+".snapshot", err)
		}
		children.Retained = append(children.Retained, RetainedChild{CallID: wire.CallID, Snapshot: &snapshot})
	}
	return children, nil
}

type wireContinuation struct {
	URI         string `json:"uri"`
	Cursor      string `json:"cursor"`
	Explanation string `json:"explanation"`
}

func (w wireContinuation) convert(field string) (Continuation, error) {
	if w.Explanation == "" {
		return Continuation{}, invalidf("%s.explanation: is required", field)
	}
	if err := validateText(field+".explanation", w.Explanation, true); err != nil {
		return Continuation{}, err
	}
	if w.URI != "" {
		if err := validateURI(field+".uri", w.URI); err != nil {
			return Continuation{}, err
		}
	}
	if w.Cursor != "" {
		if err := validateOpaqueID(field+".cursor", w.Cursor); err != nil {
			return Continuation{}, err
		}
	}
	return Continuation(w), nil
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (w wireError) convert(field string) (PresentationError, error) {
	if !identifierPattern.MatchString(w.Code) {
		return PresentationError{}, invalidf("%s.code: must match [A-Za-z][A-Za-z0-9._-]{0,63}", field)
	}
	if w.Message == "" {
		return PresentationError{}, invalidf("%s.message: is required", field)
	}
	if err := validateText(field+".message", w.Message, true); err != nil {
		return PresentationError{}, err
	}
	return PresentationError(w), nil
}

type wireProvenance struct {
	Revision string      `json:"revision"`
	Source   *wireSource `json:"source"`
	Adapter  string      `json:"adapter"`
	View     string      `json:"view"`
}

type wireSource struct {
	URI     string `json:"uri"`
	Line    *int   `json:"line"`
	EndLine *int   `json:"endLine"`
}

func (w wireProvenance) convert(field string) (Provenance, error) {
	provenance := Provenance{Revision: w.Revision, Adapter: w.Adapter, View: w.View}
	if w.Revision != "" {
		if err := validateOpaqueID(field+".revision", w.Revision); err != nil {
			return Provenance{}, err
		}
	}
	if w.Adapter != "" && !identifierPattern.MatchString(w.Adapter) {
		return Provenance{}, invalidf("%s.adapter: must match [A-Za-z][A-Za-z0-9._-]{0,63}", field)
	}
	if w.View != "" && !identifierPattern.MatchString(w.View) {
		return Provenance{}, invalidf("%s.view: must match [A-Za-z][A-Za-z0-9._-]{0,63}", field)
	}
	if w.Source != nil {
		source, err := w.Source.convert(field + ".source")
		if err != nil {
			return Provenance{}, err
		}
		provenance.Source = &source
	}
	return provenance, nil
}

func (w wireSource) convert(field string) (SourceCoordinate, error) {
	if w.URI == "" {
		return SourceCoordinate{}, invalidf("%s.uri: is required", field)
	}
	if err := validateURI(field+".uri", w.URI); err != nil {
		return SourceCoordinate{}, err
	}
	source := SourceCoordinate{URI: w.URI}
	if w.Line != nil {
		if *w.Line < 1 {
			return SourceCoordinate{}, invalidf("%s.line: must be positive", field)
		}
		source.Line = *w.Line
	}
	if w.EndLine != nil {
		if w.Line == nil {
			return SourceCoordinate{}, invalidf("%s.endLine: requires line", field)
		}
		if *w.EndLine < source.Line {
			return SourceCoordinate{}, invalidf("%s.endLine: must not precede line", field)
		}
		source.EndLine = *w.EndLine
	}
	return source, nil
}
