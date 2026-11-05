package presentation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// failedBuildSample is the SPEC-0003 §3 example, verbatim including the details envelope.
const failedBuildSample = `{"workbenchPresentation":{"version":1,"revision":3,"phase":"settled","outcome":"failed","title":"Build app","target":{"kind":"path","value":"packages/app"},"summary":"Compiler rejected two files","complete":true,"effects":"none","metrics":[{"id":"durationMs","value":1280,"unit":"ms"}],"blocks":[{"kind":"text","text":"src/main.go:42: undefined: serve","role":"error"}],"error":{"code":"compileFailed","message":"Fix the compiler errors and retry."}}}`

const searchTable = `{"version":1,"revision":0,"phase":"settled","outcome":"succeeded","blocks":[{"kind":"table","table":{"columns":[{"id":"title","label":"Title","cell":"text"},{"id":"url","label":"URL","cell":"uri"}],"rows":[[{"kind":"text","value":"Go 1.26 released"},{"kind":"uri","value":"https://go.dev/blog/go1.26"}]]}}]}`

func decodeOK(t *testing.T, payload string) Snapshot {
	t.Helper()
	snapshot, err := Decode([]byte(payload))
	if err != nil {
		t.Fatalf("decode %s: %v", payload, err)
	}
	return snapshot
}

func decodeFails(t *testing.T, payload, wantField string) error {
	t.Helper()
	snapshot, err := Decode([]byte(payload))
	if err == nil {
		t.Fatalf("payload accepted: %s", payload)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error does not match ErrInvalid: %v", err)
	}
	if !strings.Contains(err.Error(), wantField) {
		t.Fatalf("error does not name %q: %v", wantField, err)
	}
	if !reflect.DeepEqual(snapshot, Snapshot{}) {
		t.Fatalf("failed decode returned a partial snapshot: %#v", snapshot)
	}
	return err
}

func TestDecodeFailedBuildSample(t *testing.T) {
	snapshot := decodeOK(t, failedBuildSample)
	if snapshot.Version != Version || snapshot.Revision != 3 || snapshot.Phase != PhaseSettled {
		t.Fatalf("identity fields: %#v", snapshot)
	}
	if snapshot.Outcome == nil || *snapshot.Outcome != OutcomeFailed {
		t.Fatalf("outcome: %#v", snapshot.Outcome)
	}
	if snapshot.Title != "Build app" || snapshot.Summary != "Compiler rejected two files" {
		t.Fatalf("text fields: %#v", snapshot)
	}
	if snapshot.Target == nil || snapshot.Target.Kind != TargetPath || snapshot.Target.Value != "packages/app" {
		t.Fatalf("target: %#v", snapshot.Target)
	}
	if snapshot.Complete == nil || !*snapshot.Complete {
		t.Fatalf("complete: %#v", snapshot.Complete)
	}
	if snapshot.Effects == nil || *snapshot.Effects != EffectsNone {
		t.Fatalf("effects: %#v", snapshot.Effects)
	}
	if len(snapshot.Metrics) != 1 || snapshot.Metrics[0] != (Metric{ID: "durationMs", Value: 1280, Unit: UnitMilliseconds}) {
		t.Fatalf("metrics: %#v", snapshot.Metrics)
	}
	if len(snapshot.Blocks) != 1 || snapshot.Blocks[0].Kind != BlockText || snapshot.Blocks[0].Text == nil {
		t.Fatalf("blocks: %#v", snapshot.Blocks)
	}
	if body := snapshot.Blocks[0].Text; body.Text != "src/main.go:42: undefined: serve" || body.Role != TextRoleError {
		t.Fatalf("text block: %#v", body)
	}
	if snapshot.Error == nil || snapshot.Error.Code != "compileFailed" ||
		snapshot.Error.Message != "Fix the compiler errors and retry." {
		t.Fatalf("error: %#v", snapshot.Error)
	}
	bare := strings.TrimSuffix(strings.TrimPrefix(failedBuildSample, `{"workbenchPresentation":`), "}")
	if !reflect.DeepEqual(snapshot, decodeOK(t, bare)) {
		t.Fatal("details envelope and bare snapshot decode differently")
	}
}

func TestDecodeDetailsWithProducerFields(t *testing.T) {
	snapshot := decodeOK(t, `{"stdout":"build log","exitCode":1,"workbenchPresentation":{"version":1,"revision":1,"phase":"running"}}`)
	if snapshot.Phase != PhaseRunning || snapshot.Outcome != nil || snapshot.Revision != 1 {
		t.Fatalf("running snapshot: %#v", snapshot)
	}
}

func TestDecodeSearchTableURICell(t *testing.T) {
	snapshot := decodeOK(t, searchTable)
	if len(snapshot.Blocks) != 1 || snapshot.Blocks[0].Table == nil {
		t.Fatalf("blocks: %#v", snapshot.Blocks)
	}
	table := snapshot.Blocks[0].Table
	if len(table.Columns) != 2 || table.Columns[1] != (Column{ID: "url", Label: "URL", Cell: CellURI}) {
		t.Fatalf("columns: %#v", table.Columns)
	}
	cell := table.Rows[0][1]
	if cell != (Cell{Kind: CellURI, Text: "https://go.dev/blog/go1.26"}) {
		t.Fatalf("uri cell: %#v", cell)
	}
	encoded, err := json.Marshal(cell)
	if err != nil {
		t.Fatalf("encode cell: %v", err)
	}
	if string(encoded) != `{"kind":"uri","value":"https://go.dev/blog/go1.26"}` {
		t.Fatalf("uri cell carries more than kind and value: %s", encoded)
	}
}

func TestDecodeZeroMetricAndAbsentFields(t *testing.T) {
	snapshot := decodeOK(t, `{"version":1,"revision":0,"phase":"running","metrics":[{"id":"bytes","value":0,"unit":"bytes"}],"blocks":[{"kind":"table","table":{"columns":[{"id":"hits","label":"Hits","cell":"count"}],"rows":[[{"kind":"count","value":0}]]}}]}`)
	if len(snapshot.Metrics) != 1 || snapshot.Metrics[0].Value != 0 {
		t.Fatalf("zero metric: %#v", snapshot.Metrics)
	}
	if snapshot.Blocks[0].Table.Rows[0][0] != (Cell{Kind: CellCount, Number: 0}) {
		t.Fatalf("zero cell: %#v", snapshot.Blocks[0].Table.Rows[0][0])
	}
	if snapshot.Outcome != nil || snapshot.Target != nil || snapshot.Complete != nil || snapshot.Effects != nil ||
		snapshot.Continuation != nil || snapshot.Error != nil || snapshot.Provenance != nil || snapshot.ParentCallID != "" {
		t.Fatalf("absent fields were fabricated: %#v", snapshot)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	for _, absent := range []string{"outcome", "target", "complete", "effects", "continuation", "provenance", "parentCallId"} {
		if strings.Contains(string(encoded), `"`+absent+`"`) {
			t.Fatalf("absent field %q was emitted: %s", absent, encoded)
		}
	}
	if !strings.Contains(string(encoded), `{"id":"bytes","value":0,"unit":"bytes"}`) {
		t.Fatalf("explicit zero metric was dropped: %s", encoded)
	}
	if roundTripped := decodeOK(t, string(encoded)); !reflect.DeepEqual(snapshot, roundTripped) {
		t.Fatal("snapshot did not survive a round trip")
	}
}

func TestDecodeRejectsControlSequences(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		field   string
		greedy  []string
	}{
		{
			"escape in text block",
			`{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"text","text":"a\u001bb[2J"}]}`,
			"blocks[0].text", []string{"\x1b", "b[2J"},
		},
		{
			"bell in title",
			`{"version":1,"revision":0,"phase":"running","title":"x\u0007y"}`,
			"title", []string{"\x07"},
		},
		{
			"escape in summary",
			`{"version":1,"revision":0,"phase":"running","summary":"x\u001b]0;title"}`,
			"summary", []string{"\x1b"},
		},
		{
			"carriage return in error message",
			`{"version":1,"revision":0,"phase":"settled","outcome":"failed","error":{"code":"bad","message":"a\rb"}}`,
			"error.message", []string{"\r"},
		},
		{
			"escape in diff line",
			`{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"diff","diff":{"operation":"update","after":{"path":"a.go"},"hunks":[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":[{"kind":"add","text":"\u001b[31mred"}]}]}}]}`,
			"lines[0].text", []string{"\x1b", "[31m"},
		},
		{
			"del in cell value",
			`{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"t","label":"T","cell":"text"}],"rows":[[{"kind":"text","value":"a\u007fb"}]]}}]}`,
			"value", []string{"\x7f"},
		},
		{
			"escape in retained child snapshot",
			`{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"children","children":{"childCallIds":["c"],"retained":[{"callId":"c","snapshot":{"version":1,"revision":0,"phase":"running","title":"\u001b[2J"}}]}}]}`,
			"blocks[0].children.retained[0].snapshot", []string{"\x1b"},
		},
		{
			"notice body",
			`{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"notice","severity":"error","text":"\u009b"}]}`,
			"blocks[0].text", []string{"\u009b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := decodeFails(t, tt.payload, tt.field)
			for _, greedy := range tt.greedy {
				if strings.Contains(err.Error(), greedy) {
					t.Fatalf("error echoed the offending text: %q", err.Error())
				}
			}
		})
	}
	raw := "{\"version\":1,\"revision\":0,\"phase\":\"running\",\"title\":\"a\x1bb\"}"
	if _, err := Decode([]byte(raw)); err == nil || !strings.Contains(err.Error(), "control characters") {
		t.Fatalf("raw escape byte must be rejected: %v", err)
	}
}

func TestDecodeRejectsExecutableLookingFields(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{"shell script", `{"version":1,"revision":0,"phase":"running","script":"rm -rf /"}`, `"script"`},
		{"template", `{"version":1,"revision":0,"phase":"running","template":"{{ .Exec }}"}`, `"template"`},
		{"kit node graph", `{"version":1,"revision":0,"phase":"running","kit":{"type":"node"}}`, `"kit"`},
		{"block command", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"text","text":"x","command":"ls"}]}`, `"command"`},
		{"embedded image data", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"imageRef","image":{"resource":"artifact://x","data":"aGk="}}]}`, `"data"`},
		{"dropped image bytes", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"imageRef","image":{"resource":"artifact://x","base64":"aGk="}}]}`, `"base64"`},
		{"uri cell with path", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"u","label":"U","cell":"uri"}],"rows":[[{"kind":"uri","value":"https://x","path":"/x"}]]}}]}`, `"path"`},
		{"unknown block kind", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"kit"}]}`, "blocks[0].kind"},
		{"unknown cell kind", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"u","label":"U","cell":"percent"}]}}]}`, "columns[0].cell"},
		{"unknown outcome", `{"version":1,"revision":0,"phase":"settled","outcome":"success"}`, "outcome"},
		{"unknown phase", `{"version":1,"revision":0,"phase":"success"}`, "phase"},
		{"unknown unit", `{"version":1,"revision":0,"phase":"running","metrics":[{"id":"t","value":1,"unit":"seconds"}]}`, "unit"},
		{"unknown effects", `{"version":1,"revision":0,"phase":"running","effects":"success"}`, "effects"},
		{"unknown target kind", `{"version":1,"revision":0,"phase":"running","target":{"kind":"url","value":"https://x"}}`, "target.kind"},
		{"unknown hunk line kind", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"diff","diff":{"operation":"update","after":{"path":"a.go"},"hunks":[{"lines":[{"kind":"change","text":"x"}]}]}}]}`, "lines[0].kind"},
		{"unknown diff operation", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"diff","diff":{"operation":"patch","after":{"path":"a.go"}}}]}`, "operation"},
		{"wrong block payload", `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"text","text":"x","diff":{"operation":"update","after":{"path":"a.go"}}}]}`, "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decodeFails(t, tt.payload, tt.want)
		})
	}
}

func TestDecodeSettledOutcome(t *testing.T) {
	settled := decodeOK(t, `{"version":1,"revision":0,"phase":"settled","outcome":"unknown"}`)
	if settled.Outcome == nil || *settled.Outcome != OutcomeUnknown || *settled.Outcome == OutcomeSucceeded {
		t.Fatalf("unknown outcome: %#v", settled.Outcome)
	}
	for _, outcome := range []Outcome{OutcomeSucceeded, OutcomeFailed, OutcomeCancelled, OutcomeTimedOut, OutcomeDenied} {
		payload := fmt.Sprintf(`{"version":1,"revision":0,"phase":"settled","outcome":%q}`, outcome)
		if decoded := decodeOK(t, payload); decoded.Outcome == nil || *decoded.Outcome != outcome {
			t.Fatalf("outcome %q: %#v", outcome, decoded.Outcome)
		}
	}
	preparing := decodeOK(t, `{"version":1,"revision":0,"phase":"preparing"}`)
	if preparing.Outcome != nil {
		t.Fatalf("preparing claimed an outcome: %#v", preparing.Outcome)
	}
	running := decodeOK(t, `{"version":1,"revision":0,"phase":"running"}`)
	if running.Outcome != nil {
		t.Fatalf("running claimed an outcome: %#v", running.Outcome)
	}
	decodeFails(t, `{"version":1,"revision":0,"phase":"settled"}`, "outcome")
	decodeFails(t, `{"version":1,"revision":0,"phase":"settled","outcome":null}`, "null is not allowed")
	decodeFails(t, `{"version":1,"revision":0,"phase":"preparing","outcome":"succeeded"}`, "outcome")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","outcome":"failed"}`, "outcome")
}

func TestDecodeRejectsDuplicateAndNullMembers(t *testing.T) {
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","phase":"settled","outcome":"succeeded"}`, "duplicate field")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","target":null}`, "null is not allowed")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","metrics":null}`, "null is not allowed")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"text","text":null}]}`, "null is not allowed")
}

func TestDecodeRejectsMalformedIdentity(t *testing.T) {
	decodeFails(t, `{}`, "version")
	decodeFails(t, `{"version":2,"revision":0,"phase":"running"}`, "version")
	decodeFails(t, `{"version":1,"phase":"running"}`, "revision")
	decodeFails(t, `{"version":1,"revision":-1,"phase":"running"}`, "revision")
	decodeFails(t, `{"version":1,"revision":0}`, "phase")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running"} {}`, "trailing JSON")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","revision":1}`, "duplicate field")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","metrics":[{"id":"m","value":1.5,"unit":"count"}]}`, "whole number")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","metrics":[{"id":"m","unit":"count"}]}`, "value")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","metrics":[{"id":"m","value":-1,"unit":"count"}]}`, "value")
	decodeFails(t, "{\"version\":1,\"revision\":0,\"phase\":\"running\",\"title\":\"\xff\"}", "invalid UTF-8")
	decodeFails(t, `{"workbenchPresentation":null}`, "must be an object")
}

func TestDecodeAcceptsCurrencyMetrics(t *testing.T) {
	snapshot := decodeOK(t, `{"version":1,"revision":0,"phase":"running","metrics":[{"id":"cost","value":0.25,"unit":"USD"}]}`)
	if snapshot.Metrics[0].Unit != MetricUnit("USD") || snapshot.Metrics[0].Value != 0.25 {
		t.Fatalf("currency metric: %#v", snapshot.Metrics[0])
	}
}

func TestDecodeRejectsTableShapes(t *testing.T) {
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[]}}]}`, "columns")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"text"}],"rows":[[]]}}]}`, "must have 1 cells")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"text"}],"rows":[[{"kind":"uri","value":"https://x"}]]}}]}`, "must match column")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"uri"}],"rows":[[{"kind":"uri","value":"search?q=x"}]]}}]}`, "value")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"text"}],"rows":[[{"kind":"text"}]]}}]}`, "value")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"text"}],"rows":[[{"kind":"text","value":1}]]}}]}`, "must be a string")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"count"}],"rows":[[{"kind":"count","value":"1"}]]}}]}`, "must be a number")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"a","label":"A","cell":"text"},{"id":"a","label":"B","cell":"text"}]}}]}`, "duplicate column id")
}

func TestDecodeRejectsImageAndChildrenShapes(t *testing.T) {
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"imageRef","image":{"alt":"x"}}]}`, "resource or blockId")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"children","children":{"childCallIds":[]}}]}`, "childCallIds")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"children","children":{"childCallIds":["c"],"retained":[{"callId":"d","snapshot":{"version":1,"revision":0,"phase":"running"}}]}}]}`, "not a listed child call ID")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"children","children":{"childCallIds":["c"],"retained":[{"callId":"c"}]}}]}`, "snapshot")
	decodeFails(t, `{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"children","children":{"childCallIds":["c"],"retained":[{"callId":"c","snapshot":{"version":1,"revision":0,"phase":"settled"}}]}}]}`, "outcome")
}

func TestDecodeRejectsOversizedPayload(t *testing.T) {
	base := `{"version":1,"revision":0,"phase":"running"}`
	exact := base + strings.Repeat(" ", MaxPayloadBytes-len(base))
	if _, err := Decode([]byte(exact)); err != nil {
		t.Fatalf("payload of exactly %d bytes rejected: %v", MaxPayloadBytes, err)
	}
	over := exact + " "
	if _, err := Decode([]byte(over)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("payload above %d bytes accepted: %v", MaxPayloadBytes, err)
	}
}

func TestDecodeEnforcesCounts(t *testing.T) {
	blocks := func(count int) string {
		items := make([]string, count)
		for i := range items {
			items[i] = `{"kind":"text","text":"x"}`
		}
		return fmt.Sprintf(`{"version":1,"revision":0,"phase":"running","blocks":[%s]}`, strings.Join(items, ","))
	}
	metrics := func(count int) string {
		items := make([]string, count)
		for i := range items {
			items[i] = fmt.Sprintf(`{"id":"m%d","value":1,"unit":"count"}`, i)
		}
		return fmt.Sprintf(`{"version":1,"revision":0,"phase":"running","metrics":[%s]}`, strings.Join(items, ","))
	}
	rows := func(count int) string {
		items := make([]string, count)
		for i := range items {
			items[i] = fmt.Sprintf(`[{"kind":"count","value":%d}]`, i)
		}
		return fmt.Sprintf(`{"version":1,"revision":0,"phase":"running","blocks":[{"kind":"table","table":{"columns":[{"id":"n","label":"N","cell":"count"}],"rows":[%s]}}]}`, strings.Join(items, ","))
	}
	decodeOK(t, blocks(MaxBlocks))
	decodeFails(t, blocks(MaxBlocks+1), "blocks")
	decodeOK(t, metrics(MaxMetrics))
	decodeFails(t, metrics(MaxMetrics+1), "metrics")
	decodeOK(t, rows(MaxTableRows))
	decodeFails(t, rows(MaxTableRows+1), "rows")
}

func nestedSnapshots(depth int) string {
	payload := `{"version":1,"revision":0,"phase":"settled","outcome":"succeeded"}`
	for level := 1; level < depth; level++ {
		payload = fmt.Sprintf(`{"version":1,"revision":0,"phase":"settled","outcome":"succeeded","blocks":[{"kind":"children","children":{"childCallIds":["c"],"retained":[{"callId":"c","snapshot":%s}]}}]}`, payload)
	}
	return payload
}

func TestDecodeEnforcesDepth(t *testing.T) {
	decodeOK(t, nestedSnapshots(MaxDepth))
	decodeFails(t, nestedSnapshots(MaxDepth+1), "depth")
	if _, err := Decode([]byte(nestedSnapshots(MaxDepth))); err != nil {
		t.Fatalf("depth %d rejected: %v", MaxDepth, err)
	}
}

func representativeSnapshot() Snapshot {
	outcome := OutcomeFailed
	complete := false
	effects := EffectsPartial
	return Snapshot{
		Version:  Version,
		Revision: 7,
		Phase:    PhaseSettled,
		Outcome:  &outcome,
		Title:    "Build app",
		Target:   &Target{Kind: TargetPath, Value: "packages/app"},
		Summary:  "Two files changed",
		Complete: &complete,
		Effects:  &effects,
		Metrics: []Metric{
			{ID: "durationMs", Value: 1280, Unit: UnitMilliseconds},
			{ID: "cost", Value: 0, Unit: MetricUnit("USD")},
		},
		Blocks: []Block{
			{Kind: BlockText, Text: &TextBlock{Text: "first line\nsecond line", Role: TextRoleError}},
			{Kind: BlockCode, Code: &CodeBlock{Text: "go build ./...", Language: "sh"}},
			{Kind: BlockDiff, Diff: &DiffBlock{
				Operation: DiffUpdate,
				Before:    &FileIdentity{Path: "src/main.go", Revision: "sha256:aa"},
				After:     &FileIdentity{Path: "src/main.go", Revision: "sha256:bb"},
				Hunks: []Hunk{{
					OldStart: 41, OldLines: 2, NewStart: 41, NewLines: 2,
					Lines: []HunkLine{
						{Kind: HunkContext, Text: "func serve() {"},
						{Kind: HunkRemove, Text: ""},
						{Kind: HunkAdd, Text: "\tserve()"},
					},
				}},
			}},
			{Kind: BlockTable, Table: &TableBlock{
				Columns: []Column{
					{ID: "file", Label: "File", Cell: CellPath},
					{ID: "lines", Label: "Lines", Cell: CellCount},
					{ID: "url", Label: "URL", Cell: CellURI},
					{ID: "label", Label: "Label", Cell: CellLabel},
					{ID: "ms", Label: "Time", Cell: CellDuration},
					{ID: "bytes", Label: "Bytes", Cell: CellBytes},
					{ID: "text", Label: "Text", Cell: CellText},
				},
				Rows: [][]Cell{{
					{Kind: CellPath, Text: "src/main.go"},
					{Kind: CellCount, Number: 0},
					{Kind: CellURI, Text: "https://go.dev/blog/go1.26"},
					{Kind: CellLabel, Text: "main"},
					{Kind: CellDuration, Number: 12.5},
					{Kind: CellBytes, Number: 2048},
					{Kind: CellText, Text: "undefined: serve"},
				}},
			}},
			{Kind: BlockImageRef, Image: &ImageRefBlock{Resource: "artifact://build/shot.png", BlockID: "blk-1", MediaType: "image/png", Alt: "screenshot"}},
			{Kind: BlockNotice, Notice: &NoticeBlock{Text: "Output truncated", Severity: SeverityWarning}},
			{Kind: BlockChildren, Children: &ChildrenBlock{
				ChildCallIDs: []string{"call_0_0/1", "call_0_0/2"},
				Retained: []RetainedChild{{
					CallID: "call_0_0/1",
					Snapshot: &Snapshot{
						Version: 1, Revision: 0, Phase: PhaseRunning,
						Blocks: []Block{{Kind: BlockText, Text: &TextBlock{Text: "child output", Role: TextRolePlain}}},
					},
				}},
			}},
		},
		Continuation: &Continuation{URI: "artifact://build/log", Cursor: "c-1", Explanation: "Omitted 512 lines\nread the resource"},
		Error:        &PresentationError{Code: "compileFailed", Message: "Fix the compiler errors and retry."},
		Provenance: &Provenance{
			Revision: "sha256:cc",
			Source:   &SourceCoordinate{URI: "file:///work/app/src/main.go", Line: 42, EndLine: 44},
			Adapter:  "read",
			View:     "preview",
		},
		ParentCallID: "call_0_0/1",
	}
}

func TestRoundTripRepresentativeBlocks(t *testing.T) {
	snapshot := representativeSnapshot()
	if len(snapshot.Blocks) != 7 {
		t.Fatalf("fixture must cover every block kind: %d", len(snapshot.Blocks))
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("encode snapshot: %v", err)
	}
	for _, shape := range []string{
		`{"kind":"text","text":"first line\nsecond line","role":"error"}`,
		`{"kind":"code","text":"go build ./...","language":"sh"}`,
		`{"kind":"imageRef","image":`,
		`{"kind":"notice","text":"Output truncated","severity":"warning"}`,
		`{"kind":"children","children":`,
	} {
		if !strings.Contains(string(encoded), shape) {
			t.Fatalf("encoded blocks lack %s: %s", shape, encoded)
		}
	}
	decoded := decodeOK(t, string(encoded))
	if !reflect.DeepEqual(snapshot, decoded) {
		t.Fatalf("round trip mismatch:\n got %#v\nwant %#v", decoded, snapshot)
	}
}

func TestDecodeIsolatesValues(t *testing.T) {
	raw := []byte(searchTable)
	original := bytes.Clone(raw)
	first, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	second, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	first.Blocks[0].Table.Rows[0][0].Text = "changed"
	first.Blocks[0].Table.Columns[0].Label = "changed"
	if second.Blocks[0].Table.Rows[0][0].Text != "Go 1.26 released" ||
		second.Blocks[0].Table.Columns[0].Label != "Title" {
		t.Fatal("decodes share memory")
	}
	if !bytes.Equal(raw, original) {
		t.Fatal("Decode mutated its input")
	}
}
