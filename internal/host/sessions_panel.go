package host

import (
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// sessionsPanel lists the other live sessions; m messages the selected one, t and b toggle its
// trust and block flags, h hides or shows this session. Every action is the command's own.
type sessionsPanel struct {
	title     string
	self      string
	rows      func() ([]peerRow, bool, error)
	act       func(verb, rest string) (string, error)
	height    func() int
	selected  int
	composing bool
	input     string
	message   string
}

func (p *sessionsPanel) View(width int) kit.View {
	rows, hidden, err := p.rows()
	if p.composing {
		target := ""
		if p.selected < len(rows) {
			target = rows[p.selected].id
		}
		input := kit.NewInput("message")
		input.Value = p.input
		body := kit.NewContainer(kit.NewText(fitLine("Message to "+target, width), 1, 0), input)
		hint := "enter send · " + sdk.KeyText("tui.select.cancel") + " cancel"
		return kit.View{Root: panelFrame(p.title+" › message", body, p.message, hint, width), Focus: "message"}
	}
	items := make([]kit.SelectItem, len(rows))
	for i, row := range rows {
		label := row.id + "  " + row.state
		if row.flags != "" {
			label += " [" + row.flags + "]"
		}
		items[i] = kit.SelectItem{Value: row.key, Label: label, Description: fitLine(strings.TrimSpace(row.name+" "+row.project+" "+row.cwd), max(12, width-30))}
	}
	status := p.message
	switch {
	case err != nil && status == "":
		status = err.Error()
	case status == "":
		status = "This session " + p.self + " is " + map[bool]string{true: "hidden", false: "discoverable"}[hidden]
	}
	hint := "m message · t trust · b block · h hide/show · " + sdk.KeyText("tui.select.cancel") + " close"
	if len(items) == 0 {
		body := kit.NewText("No other live session is discoverable.", 1, 0)
		return kit.View{Root: panelFrame(p.title, body, status, hint, width)}
	}
	p.selected = max(0, min(p.selected, len(items)-1))
	list := kit.NewSelectList("peers", items, listRows(p.height(), 7, len(items)))
	list.SetSelectedIndex(p.selected)
	return kit.View{Root: panelFrame(p.title, list, status, hint, width), Focus: "peers"}
}

func (p *sessionsPanel) selectedRow() (peerRow, bool) {
	rows, _, err := p.rows()
	if err != nil || p.selected >= len(rows) {
		return peerRow{}, false
	}
	return rows[p.selected], true
}

func (p *sessionsPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	cancel := tui.GetKeybindings().Matches(data, "tui.select.cancel")
	if p.composing {
		if cancel {
			p.composing, p.input, p.message = false, "", ""
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
	row, have := p.selectedRow()
	run := func(verb, rest string) {
		message, err := p.act(verb, rest)
		if err != nil {
			message = err.Error()
		}
		p.message = message
	}
	switch text {
	case "m":
		if have {
			p.composing, p.input = true, ""
		}
	case "t":
		if have {
			verb := "trust"
			if strings.Contains(row.flags, "trusted") {
				verb = "untrust"
			}
			run(verb, row.key)
		}
	case "b":
		if have {
			verb := "block"
			if strings.Contains(row.flags, "blocked") {
				verb = "unblock"
			}
			run(verb, row.key)
		}
	case "h":
		_, hidden, _ := p.rows()
		verb := "hide"
		if hidden {
			verb = "show"
		}
		run(verb, "")
	}
	return sdk.RemoteComponentResult{}, nil
}

func (p *sessionsPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	if p.composing {
		switch event.Type {
		case kit.EventChange:
			p.input = event.Value
		case kit.EventCancel, kit.EventEscape:
			p.composing, p.input, p.message = false, "", ""
		case kit.EventSubmit:
			if event.Value != "" {
				p.input = event.Value
			}
			row, ok := p.selectedRow()
			text := strings.TrimSpace(p.input)
			if !ok || text == "" {
				p.message = "Type a message first."
				return sdk.RemoteComponentResult{}, nil
			}
			message, err := p.act("send", row.key+" "+text)
			if err != nil {
				p.message = err.Error()
				return sdk.RemoteComponentResult{}, nil
			}
			p.composing, p.input, p.message = false, "", message
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
