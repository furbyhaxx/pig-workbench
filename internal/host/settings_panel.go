package host

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

// stagedChange is one unsaved edit; reset removes the override at the chosen scope.
type stagedChange struct {
	value any
	reset bool
}

// settingsSaver writes edits and returns the refreshed snapshot.
type settingsSaver func(scope config.Scope, edits []config.Edit) (config.SaveResult, *config.Snapshot, error)

// settingsPanel stages scoped setting edits and saves them explicitly.
type settingsPanel struct {
	definitions []config.Definition
	snapshot    *config.Snapshot
	scopes      []config.Scope
	scopeIndex  int
	staged      map[string]stagedChange
	filter      string
	selected    int
	editing     string
	input       string
	message     string
	armed       bool
	height      func() int
	save        settingsSaver
}

func newSettingsPanel(snapshot *config.Snapshot, scopes []config.Scope, height func() int, save settingsSaver) *settingsPanel {
	return &settingsPanel{definitions: snapshot.Catalog(), snapshot: snapshot, scopes: scopes, staged: map[string]stagedChange{}, height: height, save: save}
}

func (p *settingsPanel) scope() config.Scope { return p.scopes[p.scopeIndex] }

// visible returns the definitions matching the search text.
func (p *settingsPanel) visible() []config.Definition {
	needle := strings.ToLower(strings.TrimSpace(p.filter))
	if needle == "" {
		return p.definitions
	}
	var out []config.Definition
	for _, definition := range p.definitions {
		if strings.Contains(strings.ToLower(definition.Key), needle) || strings.Contains(strings.ToLower(definition.Description), needle) {
			out = append(out, definition)
		}
	}
	return out
}

func (p *settingsPanel) current() (config.Definition, bool) {
	rows := p.visible()
	if len(rows) == 0 {
		return config.Definition{}, false
	}
	return rows[max(0, min(p.selected, len(rows)-1))], true
}

func formatSettingValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strconv.Quote(typed)
	case nil:
		return "(unset)"
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	}
}

// valueOf is the shown value: staged first, then the effective one.
func (p *settingsPanel) valueOf(definition config.Definition) (text, source string) {
	if change, ok := p.staged[definition.Key]; ok {
		if change.reset {
			return "(reset)", "unsaved"
		}
		return formatSettingValue(change.value), "unsaved"
	}
	entry, ok := p.snapshot.Get(definition.Key)
	if !ok {
		return "(unset)", "default"
	}
	return formatSettingValue(entry.Value), string(entry.Scope)
}

func editable(definition config.Definition) bool {
	switch definition.Kind {
	case config.Boolean, config.Integer, config.String:
		return true
	}
	return false
}

// next returns the value a toggle or choice cycle moves to; ok is false for kinds typed in.
func (p *settingsPanel) next(definition config.Definition) (any, bool) {
	var current any
	if change, ok := p.staged[definition.Key]; ok && !change.reset {
		current = change.value
	} else if entry, ok := p.snapshot.Get(definition.Key); ok {
		current = entry.Value
	}
	switch {
	case definition.Kind == config.Boolean:
		value, _ := current.(bool)
		return !value, true
	case definition.Kind == config.String && len(definition.Choices) > 0:
		index := slices.Index(definition.Choices, fmt.Sprint(current))
		return definition.Choices[(index+1)%len(definition.Choices)], true
	}
	return nil, false
}

// parseTyped converts typed text into the setting's value.
func parseTyped(definition config.Definition, text string) (any, error) {
	text = strings.TrimSpace(text)
	switch definition.Kind {
	case config.Integer:
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s needs a whole number", definition.Key)
		}
		if value < definition.Min || value > definition.Max {
			return nil, fmt.Errorf("%s must be between %d and %d", definition.Key, definition.Min, definition.Max)
		}
		return value, nil
	case config.String:
		return text, nil
	}
	return nil, fmt.Errorf("%s cannot be typed here", definition.Key)
}

func (p *settingsPanel) dirty() bool { return len(p.staged) > 0 }

func (p *settingsPanel) edits() []config.Edit {
	keys := make([]string, 0, len(p.staged))
	for key := range p.staged {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	edits := make([]config.Edit, 0, len(keys))
	for _, key := range keys {
		change := p.staged[key]
		edits = append(edits, config.Edit{Key: key, Value: change.value, Reset: change.reset})
	}
	return edits
}

func (p *settingsPanel) title(width int) string {
	title := "Workbench › Settings · " + string(p.scope())
	if p.dirty() {
		title += fmt.Sprintf(" · %d unsaved", len(p.staged))
	}
	return fitLine(title, width)
}

func (p *settingsPanel) View(width int) kit.View {
	if p.editing != "" {
		return p.editView(width)
	}
	rows := p.visible()
	items := make([]kit.SelectItem, len(rows))
	for i, definition := range rows {
		value, source := p.valueOf(definition)
		items[i] = kit.SelectItem{Value: definition.Key, Label: definition.Key, Description: fitLine(fmt.Sprintf("%s [%s]", value, source), max(16, width-34))}
	}
	maxVisible := listRows(p.height(), 10, 12)
	list := kit.NewSelectList("settings", items, maxVisible)
	list.SetSelectedIndex(max(0, min(p.selected, max(0, len(items)-1))))
	detail := "No setting matches the search."
	if definition, ok := p.current(); ok {
		detail = "applies: " + definition.Application + " · project: " + string(definition.Project) + " · " + definition.Description
	}
	search := "search: " + p.filter + "▏"
	hint := "enter change · ctrl+s save · ctrl+r reset · tab scope · " + sdk.KeyText("tui.select.cancel") + " back"
	body := kit.NewContainer(kit.NewText(fitLine(search, width), 1, 0), list, kit.NewText(strings.Join(wrapLines(detail, width)[:min(2, len(wrapLines(detail, width)))], "\n"), 1, 0))
	return kit.View{Root: panelFrame(p.title(width), body, p.message, hint, width), Focus: "settings"}
}

func (p *settingsPanel) editView(width int) kit.View {
	definition, _ := p.current()
	input := kit.NewInput("value")
	input.Value = p.input
	value, source := p.valueOf(definition)
	lines := wrapLines("current "+value+" ["+source+"] · applies: "+definition.Application+" · "+definition.Description, width)
	detail := kit.NewText(strings.Join(lines[:min(3, len(lines))], "\n"), 1, 0)
	body := kit.NewContainer(detail, input)
	if definition.Kind == config.Integer {
		body.AddChild(kit.NewText(fmt.Sprintf("whole number %d to %d", definition.Min, definition.Max), 1, 0))
	}
	hint := "enter stage · " + sdk.KeyText("tui.select.cancel") + " cancel"
	return kit.View{Root: panelFrame(fitLine("Workbench › Settings › "+p.editing, width), body, p.message, hint, width), Focus: "value"}
}

func printable(data string) (string, bool) {
	if text, ok := tui.DecodePrintableKey(data); ok {
		return text, true
	}
	if utf8.RuneCountInString(data) == 1 && data[0] >= 0x20 && data[0] != 0x7f {
		return data, true
	}
	return "", false
}

func (p *settingsPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	if p.editing != "" {
		return sdk.RemoteComponentResult{}, nil
	}
	switch {
	case tui.MatchesKey(data, "ctrl+s"):
		p.flush()
	case tui.MatchesKey(data, "ctrl+r"):
		if definition, ok := p.current(); ok {
			p.staged[definition.Key] = stagedChange{reset: true}
			p.message = "Reset staged for " + definition.Key + " at " + string(p.scope()) + "; ctrl+s saves."
		}
	case tui.MatchesKey(data, "tab"):
		p.scopeIndex = (p.scopeIndex + 1) % len(p.scopes)
		p.message = "Editing the " + string(p.scope()) + " scope."
	case tui.MatchesKey(data, "backspace"):
		if p.filter != "" {
			_, size := utf8.DecodeLastRuneInString(p.filter)
			p.filter = p.filter[:len(p.filter)-size]
			p.selected = 0
		}
	default:
		if text, ok := printable(data); ok {
			p.filter += text
			p.selected = 0
		}
	}
	return sdk.RemoteComponentResult{}, nil
}

func (p *settingsPanel) flush() {
	if !p.dirty() {
		p.message = "Nothing to save."
		return
	}
	result, snapshot, err := p.save(p.scope(), p.edits())
	if err != nil {
		p.message = "Not saved: " + noticeText(err.Error(), 0)
		return
	}
	if snapshot != nil {
		p.snapshot, p.definitions = snapshot, snapshot.Catalog()
	}
	p.staged = map[string]stagedChange{}
	p.armed = false
	p.message = fmt.Sprintf("Saved %d to %s (revision %d).", len(result.Changed), result.Scope, result.Revision)
	if len(result.Pending) > 0 {
		p.message += " Applies after reload: " + strings.Join(result.Pending, ", ")
	}
}

func (p *settingsPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	if p.editing != "" {
		return p.handleEdit(event)
	}
	switch event.Type {
	case kit.EventSelectionChange:
		p.selected = event.Index
	case kit.EventCancel:
		if p.dirty() && !p.armed {
			p.armed = true
			p.message = "Unsaved changes: ctrl+s saves, " + sdk.KeyText("tui.select.cancel") + " again discards."
			return sdk.RemoteComponentResult{}, nil
		}
		if p.filter != "" {
			p.filter, p.selected, p.armed = "", 0, false
			return sdk.RemoteComponentResult{}, nil
		}
		return sdk.RemoteComponentResult{Done: true}, nil
	case kit.EventSelect:
		p.armed = false
		definition, ok := p.current()
		if !ok {
			return sdk.RemoteComponentResult{}, nil
		}
		if !editable(definition) {
			p.message = definition.Key + " is not editable here; use /workbench settings set " + string(p.scope()) + " " + definition.Key + " <TOML value>."
			return sdk.RemoteComponentResult{}, nil
		}
		if value, ok := p.next(definition); ok {
			p.staged[definition.Key] = stagedChange{value: value}
			p.message = ""
			return sdk.RemoteComponentResult{}, nil
		}
		p.editing, p.input, p.message = definition.Key, "", ""
	}
	return sdk.RemoteComponentResult{}, nil
}

func (p *settingsPanel) handleEdit(event kit.Event) (sdk.RemoteComponentResult, error) {
	switch event.Type {
	case kit.EventChange:
		p.input = event.Value
	case kit.EventEscape, kit.EventCancel:
		p.editing, p.message = "", ""
	case kit.EventSubmit:
		definition, _ := p.current()
		if event.Value != "" {
			p.input = event.Value
		}
		if strings.TrimSpace(p.input) == "" {
			p.message = "Type a value, or " + sdk.KeyText("tui.select.cancel") + " to cancel."
			return sdk.RemoteComponentResult{}, nil
		}
		value, err := parseTyped(definition, p.input)
		if err != nil {
			p.message = err.Error()
			return sdk.RemoteComponentResult{}, nil
		}
		p.staged[definition.Key] = stagedChange{value: value}
		p.editing, p.message = "", ""
	}
	return sdk.RemoteComponentResult{}, nil
}
