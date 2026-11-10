package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
		row := skillRow(text)
		row.pad = options.OutputPad
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
	switch {
	case strings.HasPrefix(first, "Instructions from: "):
		row.facts = []string{instructionPath(strings.TrimPrefix(first, "Instructions from: "), root, width), "loaded"}
		row.body = row.body[1:]
	case strings.HasPrefix(first, "Instruction update for "):
		path, _, _ := strings.Cut(strings.TrimPrefix(first, "Instruction update for "), ". Apply this diff")
		row.facts = []string{instructionPath(path, root, width), "changed"}
		row.diff = true
		row.body = row.body[1:]
	case envelope.Source == "session.location":
		where, _, _ := strings.Cut(first, "; ")
		if _, to, moved := strings.Cut(where, " to "); moved {
			where = to
		}
		row.facts = []string{displayTarget(strings.TrimPrefix(where, locationIntro), "", width)}
		if strings.HasPrefix(first, locationChangedIntro) {
			row.facts = append(row.facts, "changed")
		}
	case strings.HasPrefix(first, factChangeIntro):
		row.body = row.body[1:]
		row.facts = []string{fmt.Sprintf("%d changes", len(row.body))}
		if len(row.body) == 1 {
			row.facts = []string{strings.Replace(strings.Replace(row.body[0], ": ", " ", 1), " -> ", " → ", 1)}
		}
	case title != "":
		row.facts = []string{title}
	default:
		row.facts = []string{bodySummary(envelope.Body)}
		if summary := contextSummary(envelope); summary != "" {
			row.facts = []string{summary}
		}
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

// contextSummary states what a session context frame says in the words a person would use.
func contextSummary(envelope frames.Envelope) string {
	var facts struct {
		Date, Timezone, OS, Architecture, Name, Slug, ID, Provider, EditMode, Thinking string
		ContextWindow                                                                  int
		ActiveTools, Schemes, Skills                                                   []any
	}
	if json.Unmarshal([]byte(envelope.Body), &facts) != nil {
		return ""
	}
	switch envelope.Source {
	case "session.date":
		zone, _, _ := strings.Cut(facts.Timezone, " ")
		return strings.TrimSpace(facts.Date + " " + envelope.CreatedAt.Local().Format("15:04") + " " + zone)
	case "session.environment":
		return facts.OS + "/" + facts.Architecture
	case "session.project":
		name, id := facts.Name, facts.Slug
		if id == "" {
			id = facts.ID
		}
		if name == "" {
			return id
		}
		return name + " (" + id + ")"
	case "session.model":
		model := facts.Provider + "/" + facts.ID
		if facts.Thinking != "" {
			model += ":" + facts.Thinking
		}
		if facts.ContextWindow >= 1000 {
			model += fmt.Sprintf(" · %dk context", facts.ContextWindow/1000)
		}
		return model
	case "session.capabilities":
		parts := []string{fmt.Sprintf("%d tools", len(facts.ActiveTools)), fmt.Sprintf("%d schemes", len(facts.Schemes))}
		if len(facts.Skills) > 0 {
			parts = append(parts, fmt.Sprintf("%d skills", len(facts.Skills)))
		}
		if facts.EditMode != "" {
			parts = append(parts, facts.EditMode+" edits")
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// instructionPath shows a file inside the working location relative to it and any other file absolute.
func instructionPath(path, root string, width int) string {
	if root != "" && (path == root || !strings.HasPrefix(path, strings.TrimSuffix(root, "/")+"/")) {
		root = ""
	}
	return displayTarget(path, root, width)
}

var askAnswerID = regexp.MustCompile(` \[[^\]]*\]: `)

// askRow lists every answer of a recovered form on its own line by question label; it is never collapsed.
// skillRow heads a skill message with its first line and keeps the rest as the body.
func skillRow(text string) frameRow {
	row := frameRow{glyph: "◆", tone: "accent", label: "skill"}
	if lines := bodyLines(text); len(lines) > 0 {
		row.facts, row.body = []string{lines[0]}, lines[1:]
	}
	return row
}

func askRow(text string) frameRow {
	row := frameRow{glyph: "●", tone: "success", label: "ask", open: true}
	lines := bodyLines(text)
	if len(lines) == 0 {
		return row
	}
	head, outcome, _ := strings.Cut(strings.TrimPrefix(lines[0], recoveredAnswersIntro), ":")
	if title := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(head), "("), ")"); title != "" {
		row.facts = []string{title}
	}
	if outcome = strings.TrimSpace(outcome); outcome != "" {
		row.facts = append(row.facts, outcome)
	} else {
		row.facts = append(row.facts, fmt.Sprintf("%d answered", len(lines)-1))
	}
	for _, line := range lines[1:] {
		row.body = append(row.body, askAnswerID.ReplaceAllString(line, ": "))
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
	// The hint goes first, then the count; the facts keep up to 32 columns.
	suffix := count + hint
	for _, shorter := range []string{count, ""} {
		if tui.VisibleWidth(suffix) <= room-min(tui.VisibleWidth(facts), 32) {
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
