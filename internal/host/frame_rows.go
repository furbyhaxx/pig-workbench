package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
)

const frameRowBodyLines = 40

var frameGlyphs = map[string]string{"instruction": "◆", "reminder": "↻", "warning": "▲", "error": "✗", "notice": "●", "info": "i", "message": "»"}

// painter colours text with a theme token; tests use the identity.
type painter func(token, text string) string

var frameTones = map[string]string{"instruction": "accent", "reminder": "dim", "warning": "warning", "error": "error", "notice": "success", "info": "mdLink", "message": "accent"}

// frameRow is what one transcript row states about a frame or a plain Workbench message.
type frameRow struct {
	glyph, tone, label string
	// facts follow the label on the header: subject, state, exact counts.
	facts      []string
	body       []string
	provenance string
	pad        int
	// open shows the body without expansion; diff tones added and removed lines.
	open, diff bool
}

func (a *app) registerMessageRenderers(ext *sdk.Extension) {
	ext.MessageRenderer("pig-workbench.context", func(ctx sdk.Context, message map[string]any, options sdk.MessageRenderOptions, width int) ([]string, error) {
		text, err := messageText(message)
		if err != nil {
			return nil, err
		}
		row, err := a.frameRowFor(text, width)
		if err != nil {
			return nil, err
		}
		row.pad = options.OutputPad
		return row.render(ctx.UITheme().Fg, options.Expanded, width), nil
	})
	ext.MessageRenderer("pig-workbench.skill", func(ctx sdk.Context, message map[string]any, options sdk.MessageRenderOptions, width int) ([]string, error) {
		text, err := messageText(message)
		if err != nil {
			return nil, err
		}
		lines := bodyLines(text)
		row := frameRow{glyph: "◆", tone: "accent", label: "skill", pad: options.OutputPad}
		if len(lines) > 0 {
			row.facts, row.body = []string{lines[0]}, lines[1:]
		}
		return row.render(ctx.UITheme().Fg, options.Expanded, width), nil
	})
	ext.MessageRenderer("pig-workbench.ask", func(ctx sdk.Context, message map[string]any, options sdk.MessageRenderOptions, width int) ([]string, error) {
		text, err := messageText(message)
		if err != nil {
			return nil, err
		}
		row := askRow(text)
		row.pad = options.OutputPad
		return row.render(ctx.UITheme().Fg, options.Expanded, width), nil
	})
}

func messageText(message map[string]any) (string, error) {
	switch content := message["content"].(type) {
	case string:
		return content, nil
	case []any:
		var text strings.Builder
		for _, raw := range content {
			if block, ok := raw.(map[string]any); ok && block["type"] == "text" {
				part, _ := block["text"].(string)
				text.WriteString(part)
			}
		}
		return text.String(), nil
	}
	return "", errors.New("message has no text content")
}

// frameRowFor describes a stored frame envelope; anything else is an error so the host renders it.
func (a *app) frameRowFor(text string, width int) (frameRow, error) {
	envelope, err := frames.DecodeEnvelope(text)
	if err != nil {
		return frameRow{}, err
	}
	row := frameRow{glyph: frameGlyphs[envelope.Kind], tone: frameTones[envelope.Kind], label: envelope.Source, body: bodyLines(envelope.Body)}
	title, state, root := "", "", ""
	if s, err := a.current(); err == nil {
		if frame, current, ok := s.contextFrames.ledger.Describe(envelope.Sequence); ok && frame.Source == envelope.Source {
			title, state = frame.Title, current
		}
		root = s.location.Current().Cwd
	}
	first := ""
	if len(row.body) > 0 {
		first = row.body[0]
	}
	var location struct {
		DisplayCwd string `json:"displayCwd"`
	}
	switch {
	case strings.HasPrefix(first, "Instructions from: "):
		row.facts = []string{instructionPath(strings.TrimPrefix(first, "Instructions from: "), root, width), "loaded"}
		row.body = row.body[1:]
	case strings.HasPrefix(first, "Instruction update for "):
		path, _, _ := strings.Cut(strings.TrimPrefix(first, "Instruction update for "), ". Apply this diff")
		row.facts = []string{instructionPath(path, root, width), "changed"}
		row.diff = true
		row.body = row.body[1:]
	case envelope.Source == "session.location" && json.Unmarshal([]byte(envelope.Body), &location) == nil && location.DisplayCwd != "":
		row.facts = []string{displayTarget(location.DisplayCwd, "", width)}
	case title != "":
		row.facts = []string{title}
	default:
		row.facts = []string{bodySummary(envelope.Body)}
	}
	if envelope.Trust == "untrusted" {
		row.facts = append(row.facts, "untrusted")
	}
	parts := []string{envelope.Kind, envelope.Trust + " " + envelope.Authority, fmt.Sprintf("seq %d", envelope.Sequence), envelope.CreatedAt.Local().Format("02.01.2006 15:04")}
	if state != "" {
		parts = append(parts, state)
	}
	row.provenance = strings.Join(parts, " · ")
	return row, nil
}

// instructionPath shows a file inside the working location relative to it and any other file absolute.
func instructionPath(path, root string, width int) string {
	if root != "" && (path == root || !strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/")) {
		root = ""
	}
	return displayTarget(path, root, width)
}

// askRow lists every answer of a recovered form on its own line; it is never collapsed.
func askRow(text string) frameRow {
	row := frameRow{glyph: "●", tone: "success", label: "ask", open: true}
	intro, payload, ok := strings.Cut(text, "): {")
	if !ok {
		lines := bodyLines(text)
		if len(lines) > 0 {
			row.facts, row.body = []string{lines[0]}, lines[1:]
		}
		return row
	}
	if _, title, found := strings.Cut(intro, "("); found && title != "" {
		row.facts = []string{title}
	}
	var result struct {
		Answers   map[string]json.RawMessage `json:"answers"`
		Cancelled bool                       `json:"cancelled"`
		TimedOut  bool                       `json:"timedOut"`
		Reason    string                     `json:"reason"`
	}
	payload = "{" + payload
	if json.Unmarshal([]byte(payload), &result) != nil {
		row.body = bodyLines(payload)
		return row
	}
	switch {
	case result.Cancelled:
		row.facts = append(row.facts, "cancelled")
	case result.TimedOut:
		row.facts = append(row.facts, "timed out")
	default:
		row.facts = append(row.facts, fmt.Sprintf("%d answered", len(result.Answers)))
	}
	if result.Reason != "" {
		row.body = append(row.body, tui.StripTerminalSequences(result.Reason))
	}
	ids := make([]string, 0, len(result.Answers))
	for id := range result.Answers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return strings.Index(payload, `"`+ids[i]+`":`) < strings.Index(payload, `"`+ids[j]+`":`)
	})
	for _, id := range ids {
		var answer struct {
			Values      []string `json:"values"`
			Custom      string   `json:"custom"`
			Note        string   `json:"note"`
			Placeholder string   `json:"placeholder"`
		}
		_ = json.Unmarshal(result.Answers[id], &answer)
		value := strings.Join(answer.Values, ", ")
		for _, extra := range []string{answer.Custom, answer.Placeholder} {
			if extra != "" && value != "" {
				value += ", "
			}
			value += extra
		}
		if answer.Note != "" {
			value += " (note: " + answer.Note + ")"
		}
		row.body = append(row.body, tui.StripTerminalSequences(strings.ReplaceAll(id+": "+value, "\n", " ")))
	}
	return row
}

func bodyLines(body string) []string {
	body = strings.TrimRight(tui.StripTerminalSequences(body), "\n")
	if body == "" {
		return nil
	}
	return strings.Split(body, "\n")
}

// bodySummary names what a body holds: the fields of a JSON object, else its first line.
func bodySummary(body string) string {
	var object map[string]any
	if json.Unmarshal([]byte(body), &object) == nil && len(object) > 0 {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			switch value := object[key].(type) {
			case []any:
				parts = append(parts, fmt.Sprintf("%s %d", key, len(value)))
			case map[string]any:
				parts = append(parts, key)
			default:
				parts = append(parts, fmt.Sprintf("%s %v", key, value))
			}
		}
		return strings.Join(parts, ", ")
	}
	for _, line := range bodyLines(body) {
		if strings.TrimSpace(line) != "" {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func (r frameRow) render(paint painter, expanded bool, width int) []string {
	margin := strings.Repeat(" ", max(0, r.pad))
	width = max(width-len(margin), 8)
	show := expanded || r.open
	count, hint := "", ""
	if len(r.body) > 1 && !r.open {
		count = fmt.Sprintf(" · %d lines", len(r.body))
	}
	if len(r.body) > 0 && !show {
		hint = " · " + tui.StripTerminalSequences(sdk.KeyHint("app.tools.expand", "expand"))
	}
	label := r.glyph + " " + r.label
	room := width - tui.VisibleWidth(label)
	facts := ""
	if text := strings.Join(r.facts, " · "); text != "" {
		facts = " · " + strings.ReplaceAll(text, "\n", " ")
	}
	// The hint goes first, then the count; the subject keeps at least nine columns.
	suffix := count + hint
	for _, shorter := range []string{count, ""} {
		if tui.VisibleWidth(suffix) <= room-min(tui.VisibleWidth(facts), 9) {
			break
		}
		suffix = shorter
	}
	facts = tui.TruncateToWidth(facts, max(0, room-tui.VisibleWidth(suffix)), "…", false)
	head := paint(r.tone, r.glyph) + " " + r.label + facts
	if suffix != "" {
		head += paint("dim", suffix)
	}
	lines := []string{margin + tui.TruncateToWidth(head, width, "…", false)}
	if !show {
		return lines
	}
	shown := r.body
	if len(shown) > frameRowBodyLines {
		shown = shown[:frameRowBodyLines]
	}
	rail := paint("dim", "│")
	for _, line := range shown {
		if line == "" {
			lines = append(lines, margin+rail)
			continue
		}
		tone := "toolOutput"
		switch {
		case !r.diff:
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			tone = "toolDiffAdded"
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			tone = "toolDiffRemoved"
		}
		for _, wrapped := range tui.WrapTextWithAnsi(line, width-2) {
			lines = append(lines, margin+rail+" "+paint(tone, wrapped))
		}
	}
	if hidden := len(r.body) - len(shown); hidden > 0 {
		lines = append(lines, margin+rail+" "+paint("dim", fmt.Sprintf("… %d more lines", hidden)))
	}
	if r.provenance != "" && expanded {
		lines = append(lines, margin+paint("dim", tui.TruncateToWidth("└ "+r.provenance, width, "…", false)))
	}
	return lines
}
