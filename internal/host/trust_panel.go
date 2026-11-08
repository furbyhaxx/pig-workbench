package host

import (
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// trustPanel lists Workbench trust rules (not PiG's /trust) and edits them at user scope.
type trustPanel struct {
	title    string
	rules    func() (valid, invalid []string)
	edit     func(verb, rest string) error
	height   func() int
	selected int
	adding   bool
	input    string
	message  string
}

func (p *trustPanel) View(width int) kit.View {
	if p.adding {
		input := kit.NewInput("rule")
		input.Value = p.input
		body := kit.NewContainer(kit.NewText(fitLine("allow <absolute dir> or deny <absolute dir>", width), 1, 0), input)
		hint := "enter save · " + sdk.KeyText("tui.select.cancel") + " cancel"
		return kit.View{Root: panelFrame(p.title+" › add", body, p.message, hint, width), Focus: "rule"}
	}
	valid, invalid := p.rules()
	items := make([]kit.SelectItem, len(valid))
	for i, rule := range valid {
		items[i] = kit.SelectItem{Value: rule, Label: fitLine(rule, max(12, width-4))}
	}
	status := p.message
	if status == "" && len(invalid) > 0 {
		status = "Ignored malformed rules: " + noticeText(strings.Join(invalid, "; "), 0)
	}
	hint := "a add · d remove · " + sdk.KeyText("tui.select.cancel") + " close"
	if len(items) == 0 {
		body := kit.NewText("No trust rules: nested repositories stay a boundary. Nearest rule wins; deny beats allow at equal depth.", 1, 0)
		return kit.View{Root: panelFrame(p.title, body, status, hint, width)}
	}
	p.selected = max(0, min(p.selected, len(items)-1))
	list := kit.NewSelectList("rules", items, listRows(p.height(), 7, len(items)))
	list.SetSelectedIndex(p.selected)
	return kit.View{Root: panelFrame(p.title, list, status, hint, width), Focus: "rules"}
}

func (p *trustPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	cancel := tui.GetKeybindings().Matches(data, "tui.select.cancel")
	if p.adding {
		if cancel {
			p.adding, p.input, p.message = false, "", ""
		}
		return sdk.RemoteComponentResult{}, nil
	}
	if cancel {
		return sdk.RemoteComponentResult{Done: true}, nil
	}
	text, ok := printable(data)
	if !ok {
		return sdk.RemoteComponentResult{}, nil
	}
	p.message = ""
	switch text {
	case "a":
		p.adding, p.input = true, ""
	case "d":
		valid, _ := p.rules()
		if p.selected < len(valid) {
			_, dir, _ := strings.Cut(valid[p.selected], " ")
			if err := p.edit("rm", dir); err != nil {
				p.message = err.Error()
			} else {
				p.message = "Removed " + valid[p.selected]
			}
		}
	}
	return sdk.RemoteComponentResult{}, nil
}

func (p *trustPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	if p.adding {
		switch event.Type {
		case kit.EventChange:
			p.input = event.Value
		case kit.EventCancel, kit.EventEscape:
			p.adding, p.input, p.message = false, "", ""
		case kit.EventSubmit:
			if event.Value != "" {
				p.input = event.Value
			}
			if err := p.edit("add", strings.TrimSpace(p.input)); err != nil {
				p.message = err.Error()
				return sdk.RemoteComponentResult{}, nil
			}
			p.message = "Saved " + strings.TrimSpace(p.input)
			p.adding, p.input = false, ""
		}
		return sdk.RemoteComponentResult{}, nil
	}
	switch event.Type {
	case kit.EventSelectionChange:
		p.selected = event.Index
	case kit.EventCancel:
		return sdk.RemoteComponentResult{Done: true}, nil
	}
	return sdk.RemoteComponentResult{}, nil
}
