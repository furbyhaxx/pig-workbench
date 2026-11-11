package host

import (
	"fmt"
	"strings"
	"unicode/utf8"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"

	"github.com/furbyhaxx/pig-workbench/internal/pathdisplay"
)

// panelsAvailable reports whether kit panels can open; RPC, print and JSON keep plain notifications.
func panelsAvailable(ctx sdk.Context) bool { return ctx.Mode() == "tui" && ctx.HasUI() }

// hostRows is what the host keeps below an overlay panel (footer and spacing), measured on a 16-row terminal.
const hostRows = 3

// chromeRows is the two borders, title, status and hint rows plus the host footer.
const chromeRows = 5 + hostRows

// listRows is how many list rows fit once overhead extra rows (borders, title, hint, ...) are drawn.
func listRows(height, overhead, most int) int {
	if height <= 0 {
		height = 24
	}
	return max(3, min(most, height-overhead-hostRows))
}

func bodyRows(height int) int {
	if height <= 0 {
		height = 24
	}
	return max(3, height-chromeRows)
}

// pageWindow returns the clamped top row and the visible slice of lines.
func pageWindow(lines []string, top, rows int) (int, []string) {
	top = max(0, min(top, max(0, len(lines)-rows)))
	end := min(len(lines), top+rows)
	return top, lines[top:end]
}

// wrapLines wraps text to width columns after stripping terminal control sequences.
func wrapLines(text string, width int) []string {
	text = tui.StripTerminalSequences(text)
	width = max(8, width-2)
	var lines []string
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, tui.WrapTextWithAnsi(line, width)...)
	}
	return lines
}

func fitLine(text string, width int) string {
	return tui.StripTerminalSequences(tui.TruncateToWidth(tui.StripTerminalSequences(text), max(1, width-2), "…", false))
}

func panelFrame(title string, body kit.Node, status, hint string, width int) *kit.Container {
	children := []kit.Node{kit.NewDynamicBorder("borderAccent"), kit.NewText(fitLine(title, width), 1, 0), body}
	if status != "" {
		lines := wrapLines(status, width)
		if len(lines) > 4 {
			lines = append(lines[:3], fitLine(lines[3]+" …", width))
		}
		for _, line := range lines {
			children = append(children, kit.NewText(fitLine(line, width), 1, 0))
		}
	}
	for _, line := range hintLines(hint, width) {
		children = append(children, kit.NewText(line, 1, 0))
	}
	children = append(children, kit.NewDynamicBorder("borderAccent"))
	return kit.NewContainer(children...)
}

// hintLines breaks a hint between its " · " parts so a narrow panel still names every key.
func hintLines(hint string, width int) []string {
	var lines []string
	line := ""
	for _, part := range strings.Split(hint, " · ") {
		switch {
		case line == "":
			line = part
		case tui.VisibleWidth(line+" · "+part) <= max(width-2, 10):
			line += " · " + part
		default:
			lines = append(lines, fitLine(line, width))
			line = part
		}
	}
	return append(lines, fitLine(line, width))
}

// textPanel is a read-only scrollable view of a command's output.
type textPanel struct {
	title  string
	text   string
	height func() int
	width  int
	lines  []string
	top    int
}

func (p *textPanel) View(width int) kit.View {
	if width != p.width || p.lines == nil {
		p.width, p.lines = width, wrapLines(p.text, width)
	}
	rows := bodyRows(p.height())
	var window []string
	p.top, window = pageWindow(p.lines, p.top, rows)
	position := ""
	if len(p.lines) > rows {
		position = fmt.Sprintf("lines %d-%d of %d", p.top+1, p.top+len(window), len(p.lines))
	}
	hint := sdk.KeyHint("tui.select.up", "scroll") + " · " + sdk.KeyHint("tui.select.pageDown", "page") + " · " + sdk.KeyHint("tui.select.cancel", "close")
	body := kit.NewText(strings.Join(window, "\n"), 1, 0)
	return kit.View{Root: panelFrame(p.title, body, position, hint, width)}
}

func (p *textPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	keys := tui.GetKeybindings()
	rows := bodyRows(p.height())
	switch {
	case keys.Matches(data, "tui.select.cancel"):
		return sdk.RemoteComponentResult{Done: true}, nil
	case keys.Matches(data, "tui.select.up"):
		p.top--
	case keys.Matches(data, "tui.select.down"):
		p.top++
	case keys.Matches(data, "tui.select.pageUp"):
		p.top -= rows
	case keys.Matches(data, "tui.select.pageDown"):
		p.top += rows
	case tui.MatchesKey(data, "home"):
		p.top = 0
	case tui.MatchesKey(data, "end"):
		p.top = len(p.lines)
	}
	return sdk.RemoteComponentResult{}, nil
}

type hubItem struct{ value, label, description string }

// hubLabelColumn is where the kit list starts a description: cursor and its 32-cell label column.
const hubLabelColumn = 34

// emptyState is the one wording for a list with nothing in it, with an optional next step.
func emptyState(what, next string) string {
	text := "No " + what + "."
	if next != "" {
		text += " " + next
	}
	return text
}

// hubPanel is the Workbench entry menu: one selectable row per feature, narrowed by typing.
type hubPanel struct {
	location string
	items    []hubItem
	filter   string
	selected int
	height   func() int
}

// visible returns the entries whose name, verb or description contains the typed text.
func (p *hubPanel) visible() []hubItem {
	needle := strings.ToLower(strings.TrimSpace(p.filter))
	if needle == "" {
		return p.items
	}
	var out []hubItem
	for _, item := range p.items {
		if strings.Contains(strings.ToLower(item.label+" "+item.value+" "+item.description), needle) {
			out = append(out, item)
		}
	}
	return out
}

func (p *hubPanel) View(width int) kit.View {
	visible := p.visible()
	items := make([]kit.SelectItem, len(visible))
	for i, item := range visible {
		items[i] = kit.SelectItem{Value: item.value, Label: item.label, Description: fitLine(item.description, max(8, width-hubLabelColumn))}
	}
	list := kit.NewSelectList("hub", items, listRows(p.height(), 7, len(p.items)))
	list.SetSelectedIndex(max(0, min(p.selected, max(0, len(items)-1))))
	hint := sdk.KeyHint("tui.select.confirm", "open") + " · type to search · " + sdk.KeyHint("tui.select.cancel", "close")
	body := kit.NewContainer(kit.NewText(fitLine("search: "+p.filter+"▏", width), 1, 0), list)
	if len(items) == 0 {
		body.AddChild(kit.NewText(emptyState("entry matches the search", ""), 1, 0))
	}
	const prefix = "Workbench · "
	title := prefix + pathdisplay.Display(pathdisplay.Target{Value: p.location, NoFish: true}, max(8, width-2-tui.VisibleWidth(prefix)))
	return kit.View{Root: panelFrame(title, body, "", hint, width), Focus: "hub"}
}

func (p *hubPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	switch {
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

func (p *hubPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	switch event.Type {
	case kit.EventSelectionChange:
		p.selected = event.Index
	case kit.EventSelect:
		if event.Item != nil {
			return sdk.RemoteComponentResult{Done: true, Value: event.Item.Value}, nil
		}
	case kit.EventCancel:
		if p.filter != "" {
			p.filter, p.selected = "", 0
			return sdk.RemoteComponentResult{}, nil
		}
		return sdk.RemoteComponentResult{Done: true}, nil
	}
	return sdk.RemoteComponentResult{}, nil
}
