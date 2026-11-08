package host

import (
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/kit"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// jobRow is one job as the panel lists it.
type jobRow struct{ id, label, command string }

// jobsPanel lists the session's jobs and acts on them through the jobs tool, so the panel and the
// model share one policy. Detail shows a bounded output page; k asks once before stopping a job.
type jobsPanel struct {
	title    string
	rows     func() []jobRow
	act      func(id, action string) (string, error)
	height   func() int
	selected int
	detail   string
	output   []string
	top      int
	width    int
	confirm  string
	message  string
}

func (p *jobsPanel) current() (jobRow, bool) {
	rows := p.rows()
	if p.detail != "" {
		for _, row := range rows {
			if row.id == p.detail {
				return row, true
			}
		}
		return jobRow{id: p.detail, label: "gone"}, true
	}
	if p.selected < 0 || p.selected >= len(rows) {
		return jobRow{}, false
	}
	return rows[p.selected], true
}

func (p *jobsPanel) View(width int) kit.View {
	p.width = width
	if p.detail != "" {
		return p.detailView(width)
	}
	rows := p.rows()
	items := make([]kit.SelectItem, len(rows))
	for i, row := range rows {
		items[i] = kit.SelectItem{Value: row.id, Label: row.id + "  " + row.label, Description: fitLine(row.command, max(12, width-28))}
	}
	hint := "enter open · k stop · w wait · r refresh · " + sdk.KeyText("tui.select.cancel") + " close"
	status := p.status()
	if len(items) == 0 {
		body := kit.NewText("No jobs in this session.", 1, 0)
		return kit.View{Root: panelFrame(p.title, body, status, hint, width)}
	}
	p.selected = max(0, min(p.selected, len(items)-1))
	list := kit.NewSelectList("jobs", items, listRows(p.height(), 7, len(items)))
	list.SetSelectedIndex(p.selected)
	return kit.View{Root: panelFrame(p.title, list, status, hint, width), Focus: "jobs"}
}

func (p *jobsPanel) status() string {
	if p.confirm != "" {
		return "Stop job " + p.confirm + "? y confirms, any other key cancels"
	}
	return p.message
}

func (p *jobsPanel) detailView(width int) kit.View {
	rows := bodyRows(p.height())
	var window []string
	p.top, window = pageWindow(p.output, p.top, rows)
	row, _ := p.current()
	title := p.title + " › " + p.detail + " · " + row.label
	status := p.status()
	if status == "" && len(p.output) > rows {
		status = fmt.Sprintf("lines %d-%d of %d", p.top+1, p.top+len(window), len(p.output))
	}
	hint := "k stop · w wait · r refresh · " + sdk.KeyText("tui.select.cancel") + " back"
	body := kit.NewText(strings.Join(window, "\n"), 1, 0)
	return kit.View{Root: panelFrame(title, body, status, hint, width)}
}

func (p *jobsPanel) refreshDetail() {
	text, err := p.act(p.detail, "output")
	if err != nil {
		p.message = err.Error()
		return
	}
	p.output = wrapLines(text, max(p.width, 20))
	p.top = max(0, len(p.output)-bodyRows(p.height()))
}

func (p *jobsPanel) run(action string) {
	row, ok := p.current()
	if !ok {
		return
	}
	text, err := p.act(row.id, action)
	switch {
	case err != nil:
		p.message = action + ": " + err.Error()
	case action == "kill":
		p.message = "Stopped " + row.id
	case action == "wait":
		p.message = "Waited on " + row.id
	}
	if p.detail != "" {
		if err == nil && (action == "wait" || action == "kill") {
			p.output = wrapLines(text, max(p.width, 20))
			p.top = max(0, len(p.output)-bodyRows(p.height()))
		}
	}
}

func (p *jobsPanel) HandleInput(data string) (sdk.RemoteComponentResult, error) {
	if p.confirm != "" {
		id := p.confirm
		p.confirm = ""
		if text, ok := printable(data); ok && text == "y" {
			p.message = ""
			p.selectID(id)
			p.run("kill")
		} else {
			p.message = "Stop cancelled"
		}
		return sdk.RemoteComponentResult{}, nil
	}
	keys := tui.GetKeybindings()
	if p.detail != "" {
		rows := bodyRows(p.height())
		switch {
		case keys.Matches(data, "tui.select.cancel"):
			p.detail, p.output, p.message = "", nil, ""
		case keys.Matches(data, "tui.select.up"):
			p.top--
		case keys.Matches(data, "tui.select.down"):
			p.top++
		case keys.Matches(data, "tui.select.pageUp"):
			p.top -= rows
		case keys.Matches(data, "tui.select.pageDown"):
			p.top += rows
		default:
			p.letter(data)
		}
		return sdk.RemoteComponentResult{}, nil
	}
	if keys.Matches(data, "tui.select.cancel") {
		return sdk.RemoteComponentResult{Done: true}, nil
	}
	p.letter(data)
	return sdk.RemoteComponentResult{}, nil
}

func (p *jobsPanel) selectID(id string) {
	if p.detail != "" {
		return
	}
	for i, row := range p.rows() {
		if row.id == id {
			p.selected = i
		}
	}
}

func (p *jobsPanel) letter(data string) {
	text, ok := printable(data)
	if !ok {
		return
	}
	p.message = ""
	switch text {
	case "k":
		if row, ok := p.current(); ok && row.id != "" {
			p.confirm = row.id
		}
	case "w":
		p.run("wait")
	case "r":
		if p.detail != "" {
			p.refreshDetail()
		}
		p.message = "Refreshed"
	}
}

func (p *jobsPanel) HandleViewEvent(event kit.Event) (sdk.RemoteComponentResult, error) {
	if p.detail != "" {
		return sdk.RemoteComponentResult{}, nil
	}
	switch event.Type {
	case kit.EventSelectionChange:
		p.selected = event.Index
	case kit.EventSelect:
		if row, ok := p.current(); ok && row.id != "" {
			p.detail = row.id
			p.refreshDetail()
		}
	case kit.EventCancel:
		return sdk.RemoteComponentResult{Done: true}, nil
	}
	return sdk.RemoteComponentResult{}, nil
}
