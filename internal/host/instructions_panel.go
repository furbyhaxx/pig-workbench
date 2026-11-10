package host

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// instructionsPanel shows pinned instruction files with pin and unpin, and the class rules on demand.
// It acts through the same functions as the command and session://self/instructions/ctl.
type instructionsPanel struct {
	title    string
	pins     func() []instructionPin
	rules    func() (string, error)
	pin      func(path string) (string, error)
	unpin    func(path string) (string, error)
	height   func() int
	selected int
	mode     string
	input    string
	message  string
	text     []string
	top      int
	width    int
}

func (p *instructionsPanel) View(width int) kit.View {
	p.width = width
	switch p.mode {
	case "input":
		input := kit.NewInput("path")
		input.Value = p.input
		body := kit.NewContainer(kit.NewText(fitLine("Path of the file to pin (relative to the location)", width), 1, 0), input)
		hint := "enter pin · " + sdk.KeyText("tui.select.cancel") + " cancel"
		return kit.View{Root: panelFrame(p.title+" › pin", body, p.message, hint, width), Focus: "path"}
	case "rules":
		rows := bodyRows(p.height())
		var window []string
		p.top, window = pageWindow(p.text, p.top, rows)
		status := ""
		if len(p.text) > rows {
			status = fmt.Sprintf("lines %d-%d of %d", p.top+1, p.top+len(window), len(p.text))
		}
		hint := sdk.KeyHint("tui.select.up", "scroll") + " · " + sdk.KeyText("tui.select.cancel") + " back"
		return kit.View{Root: panelFrame(p.title+" › rules", kit.NewText(strings.Join(window, "\n"), 1, 0), status, hint, width)}
	}
	pins := p.pins()
	items := make([]kit.SelectItem, 0, len(pins))
	for _, pin := range pins {
		items = append(items, kit.SelectItem{Value: pin.Path, Label: filepath.Base(pin.Path), Description: fitLine(pin.Class+" · "+pin.Origin+" · "+filepath.Dir(pin.Path), max(12, width-24))})
	}
	hint := "p pin · u unpin · v rules · " + sdk.KeyText("tui.select.cancel") + " close"
	if len(items) == 0 {
		body := kit.NewText(emptyState("pinned instruction files", "Root and nearest files apply automatically; press p to pin one."), 1, 0)
		return kit.View{Root: panelFrame(p.title, body, p.message, hint, width)}
	}
	p.selected = max(0, min(p.selected, len(items)-1))
	list := kit.NewSelectList("pins", items, listRows(p.height(), 7, len(items)))
	list.SetSelectedIndex(p.selected)
	return kit.View{Root: panelFrame(p.title, list, p.message, hint, width), Focus: "pins"}
}

func (p *instructionsPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	keys := tui.GetKeybindings()
	switch p.mode {
	case "input":
		if keys.Matches(data, "tui.select.cancel") {
			p.mode, p.input, p.message = "", "", ""
		}
		return sdk.RemoteComponentResult{}, nil
	case "rules":
		rows := bodyRows(p.height())
		switch {
		case keys.Matches(data, "tui.select.cancel"):
			p.mode = ""
		case keys.Matches(data, "tui.select.up"):
			p.top--
		case keys.Matches(data, "tui.select.down"):
			p.top++
		case keys.Matches(data, "tui.select.pageUp"):
			p.top -= rows
		case keys.Matches(data, "tui.select.pageDown"):
			p.top += rows
		}
		return sdk.RemoteComponentResult{}, nil
	}
	if keys.Matches(data, "tui.select.cancel") {
		return sdk.RemoteComponentResult{Done: true}, nil
	}
	text, ok := printable(data)
	if !ok {
		return sdk.RemoteComponentResult{}, nil
	}
	p.message = ""
	switch text {
	case "p":
		p.mode, p.input = "input", ""
	case "u":
		pins := p.pins()
		if p.selected < len(pins) {
			message, err := p.unpin(pins[p.selected].Path)
			p.report(message, err)
		}
	case "v":
		rules, err := p.rules()
		if err != nil {
			p.message = err.Error()
			break
		}
		p.text, p.top, p.mode = wrapLines(rules, max(p.width, 20)), 0, "rules"
	}
	return sdk.RemoteComponentResult{}, nil
}

func (p *instructionsPanel) report(message string, err error) {
	if err != nil {
		p.message = err.Error()
		return
	}
	p.message = message
}

func (p *instructionsPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	if p.mode == "input" {
		switch event.Type {
		case kit.EventChange:
			p.input = event.Value
		case kit.EventCancel, kit.EventEscape:
			p.mode, p.input, p.message = "", "", ""
		case kit.EventSubmit:
			if event.Value != "" {
				p.input = event.Value
			}
			path := strings.TrimSpace(p.input)
			if path == "" || !utf8.ValidString(path) {
				p.message = "Enter a file path."
				return sdk.RemoteComponentResult{}, nil
			}
			message, err := p.pin(path)
			if err != nil {
				p.message = err.Error()
				return sdk.RemoteComponentResult{}, nil
			}
			p.mode, p.input, p.message = "", "", message
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
