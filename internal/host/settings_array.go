package host

import (
	"fmt"
	"slices"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

// arrayEditor edits one string-array setting as a list: enter changes an item, the last row adds
// one, d removes the selected item, and escape stages the result as a whole replacement array.
type arrayEditor struct {
	key      string
	items    []string
	original []string
	selected int
	editing  bool
	adding   bool
	input    string
	message  string
}

// currentStrings is the array shown now: staged first, then the effective value, then the default.
func (p *settingsPanel) currentStrings(definition config.Definition) []string {
	var value any = definition.Default
	if change, ok := p.staged[definition.Key]; ok && !change.reset {
		value = change.value
	} else if entry, ok := p.snapshot.Get(definition.Key); ok {
		value = entry.Value
	}
	switch typed := value.(type) {
	case []string:
		return slices.Clone(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return out
	}
	return nil
}

func (e *arrayEditor) view(p *settingsPanel, width int) kit.View {
	title := fitLine("Workbench › Settings › "+e.key+" · "+string(p.scope()), width)
	if e.editing {
		input := kit.NewInput("item")
		input.Value = e.input
		label := "Edit item " + fmt.Sprint(e.selected+1)
		if e.adding {
			label = "New item"
		}
		body := kit.NewContainer(kit.NewText(fitLine(label, width), 1, 0), input)
		hint := "enter keep · " + sdk.KeyText("tui.select.cancel") + " cancel"
		return kit.View{Root: panelFrame(title, body, e.message, hint, width), Focus: "item"}
	}
	items := make([]kit.SelectItem, 0, len(e.items)+1)
	for i, item := range e.items {
		items = append(items, kit.SelectItem{Value: fmt.Sprint(i), Label: fitLine(item, max(16, width-6))})
	}
	items = append(items, kit.SelectItem{Value: "add", Label: "+ add item"})
	list := kit.NewSelectList("items", items, listRows(p.height(), 8, 12))
	list.SetSelectedIndex(max(0, min(e.selected, len(items)-1)))
	status := e.message
	if status == "" && !slices.Equal(e.items, e.original) {
		status = "changed; " + sdk.KeyText("tui.select.cancel") + " stages it"
	}
	hint := "enter edit · d remove · " + sdk.KeyText("tui.select.cancel") + " done"
	return kit.View{Root: panelFrame(title, list, status, hint, width), Focus: "items"}
}

func (e *arrayEditor) handleInput(data string) {
	if e.editing {
		return
	}
	if text, ok := printable(data); ok && (text == "d" || text == "x") && e.selected < len(e.items) {
		e.items = slices.Delete(e.items, e.selected, e.selected+1)
		e.selected = min(e.selected, len(e.items))
	}
}

// handleEvent returns after staging or leaving; the panel clears p.array when the editor is done.
func (e *arrayEditor) handleEvent(p *settingsPanel, event kit.Event) {
	if e.editing {
		switch event.Type {
		case kit.EventChange:
			e.input = event.Value
		case kit.EventEscape, kit.EventCancel:
			e.editing, e.adding, e.message = false, false, ""
		case kit.EventSubmit:
			if event.Value != "" {
				e.input = event.Value
			}
			value := strings.TrimSpace(e.input)
			if value == "" {
				e.message = "An item cannot be empty; " + sdk.KeyText("tui.select.cancel") + " cancels."
				return
			}
			if e.adding {
				e.items = append(e.items, value)
				e.selected = len(e.items) - 1
			} else {
				e.items[e.selected] = value
			}
			e.editing, e.adding, e.message = false, false, ""
		}
		return
	}
	switch event.Type {
	case kit.EventSelectionChange:
		e.selected = event.Index
	case kit.EventSelect:
		e.message = ""
		if e.selected >= len(e.items) {
			e.editing, e.adding, e.input = true, true, ""
			return
		}
		e.editing, e.input = true, e.items[e.selected]
	case kit.EventCancel:
		if !slices.Equal(e.items, e.original) {
			p.staged[e.key] = stagedChange{value: slices.Clone(e.items)}
			p.message = fmt.Sprintf("Staged %d item(s) for %s; ctrl+s saves.", len(e.items), e.key)
		}
		p.array = nil
	}
}
