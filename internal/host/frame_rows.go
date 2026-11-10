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

// frameRow is what one transcript row states about a frame or a plain Workbench message.
type frameRow struct {
	glyph, label, summary string
	body                  []string
	provenance            string
	pad                   int
}

func (a *app) registerMessageRenderers(ext *sdk.Extension) {
	ext.MessageRenderer("pig-workbench.context", a.renderFrameMessage)
	ext.MessageRenderer("pig-workbench.skill", plainMessageRenderer("◆", "skill"))
	ext.MessageRenderer("pig-workbench.ask", plainMessageRenderer("●", "ask"))
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

func (a *app) renderFrameMessage(_ sdk.Context, message map[string]any, options sdk.MessageRenderOptions, width int) ([]string, error) {
	text, err := messageText(message)
	if err != nil {
		return nil, err
	}
	envelope, err := frames.DecodeEnvelope(text)
	if err != nil {
		return nil, err
	}
	row := frameRow{glyph: frameGlyphs[envelope.Kind], label: envelope.Source, body: bodyLines(envelope.Body), pad: options.OutputPad}
	state := ""
	root := ""
	if s, err := a.current(); err == nil {
		if frame, current, ok := s.contextFrames.ledger.Describe(envelope.Sequence); ok && frame.Source == envelope.Source {
			row.summary, state = frame.Title, current
		}
		root = s.location.Current().Cwd
	}
	if row.summary == "" {
		row.summary = bodySummary(envelope.Body)
	}
	if verb, path, ok := strings.Cut(row.summary, ": "); ok && strings.HasPrefix(path, "/") && !strings.ContainsAny(path, " ,") {
		row.summary = verb + ": " + displayTarget(path, root, width)
	}
	parts := []string{envelope.Kind, envelope.Trust + " " + envelope.Authority, fmt.Sprintf("seq %d", envelope.Sequence), envelope.CreatedAt.Local().Format("02.01.2006 15:04")}
	if state != "" {
		parts = append(parts, state)
	}
	row.provenance = strings.Join(parts, " · ")
	return row.render(options.Expanded, width), nil
}

func plainMessageRenderer(glyph, label string) sdk.RendererFunc {
	return func(_ sdk.Context, message map[string]any, options sdk.MessageRenderOptions, width int) ([]string, error) {
		text, err := messageText(message)
		if err != nil {
			return nil, err
		}
		lines := bodyLines(text)
		row := frameRow{glyph: glyph, label: label, pad: options.OutputPad}
		if len(lines) > 0 {
			row.summary, row.body = lines[0], lines[1:]
		}
		return row.render(options.Expanded, width), nil
	}
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

func (r frameRow) render(expanded bool, width int) []string {
	margin := strings.Repeat(" ", max(0, r.pad))
	width = max(width-len(margin), 8)
	head := r.glyph + " " + r.label
	if r.summary != "" {
		head += " · " + r.summary
	}
	if !expanded {
		suffix := ""
		if len(r.body) > 1 {
			suffix = fmt.Sprintf(" · %d lines · ", len(r.body)) + sdk.KeyHint("app.tools.expand", "expand")
		}
		return []string{margin + withSuffix(head, suffix, width, tui.VisibleWidth(r.glyph+" "+r.label))}
	}
	lines := []string{margin + tui.TruncateToWidth(head, width, "…", false)}
	shown := r.body
	if len(shown) > frameRowBodyLines {
		shown = shown[:frameRowBodyLines]
	}
	for _, line := range shown {
		if line == "" {
			lines = append(lines, margin+"│")
			continue
		}
		for _, wrapped := range tui.WrapTextWithAnsi(line, width-2) {
			lines = append(lines, margin+"│ "+wrapped)
		}
	}
	if hidden := len(r.body) - len(shown); hidden > 0 {
		lines = append(lines, margin+fmt.Sprintf("│ … %d more lines", hidden))
	}
	if r.provenance != "" {
		lines = append(lines, margin+tui.TruncateToWidth("└ "+r.provenance, width, "…", false))
	}
	return lines
}

// withSuffix keeps suffix visible by shortening text down to keep columns; a narrower width drops the suffix.
func withSuffix(text, suffix string, width, keep int) string {
	room := width - tui.VisibleWidth(suffix)
	if room < keep {
		return tui.TruncateToWidth(text, width, "…", false)
	}
	return tui.TruncateToWidth(text, room, "…", false) + suffix
}
