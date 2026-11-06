package host

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

const (
	activityKey          = "workbench-activity"
	activityPollInterval = 500 * time.Millisecond
	narrowActivity       = 50
)

// activityItem is one counted fact of the activity line. Zero counts are not shown.
type activityItem struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Glyph    string `json:"glyph"`
	Count    int    `json:"count"`
	Severity string `json:"severity,omitempty"`
	Priority int    `json:"priority"`
	// Open is the command that inspects the item.
	Open string `json:"open,omitempty"`
}

// activityItems derives the line from the live registries: no I/O beyond reading them.
func (s *session) activityItems() []activityItem {
	var items []activityItem
	if s.jobs != nil {
		running, failed := 0, 0
		for _, job := range s.jobs.List() {
			switch {
			case job.Running:
				running++
			case job.Cancelled:
			case job.Error != "" || (job.Exit != nil && (job.Exit.Code != 0 || job.Exit.Signaled)):
				failed++
			}
		}
		items = append(items,
			activityItem{Key: "jobs", Label: "jobs", Glyph: "⚙", Count: running, Priority: 80, Open: "/workbench jobs"},
			activityItem{Key: "jobs.failed", Label: "failed", Glyph: "✗", Count: failed, Severity: "error", Priority: 90, Open: "/workbench jobs"})
	}
	if e := s.endpoint; e != nil {
		e.mu.Lock()
		asks := e.waiting
		e.mu.Unlock()
		items = append(items, activityItem{Key: "ask", Label: "waiting for you", Glyph: "?", Count: min(asks, 1), Severity: "warning", Priority: 100, Open: "/workbench ask show"})
		if peers, err := e.peers(context.Background()); err == nil {
			items = append(items, activityItem{Key: "peers", Label: "live sessions", Glyph: "⚇", Count: len(peers), Priority: 60, Open: "/workbench sessions"})
		}
	}
	if s.skills != nil {
		s.skills.mu.Lock()
		loaded := 0
		for _, on := range s.skills.loaded {
			if on {
				loaded++
			}
		}
		s.skills.mu.Unlock()
		items = append(items, activityItem{Key: "skills", Label: "skills", Glyph: "◆", Count: loaded, Priority: 40, Open: "/workbench skills"})
	}
	for _, segment := range s.status.activity() {
		count := 0
		if segment.value.Count != nil {
			count = *segment.value.Count
		} else if strings.TrimSpace(segment.value.Text) != "" {
			count = 1
		}
		glyph := segment.descriptor.Glyph
		if glyph == "" {
			glyph = "•"
		}
		label := segment.descriptor.Label
		if segment.value.Count == nil {
			label = strings.TrimSpace(segment.value.Text)
		}
		items = append(items, activityItem{Key: segment.owner + "/" + segment.key, Label: label, Glyph: glyph, Count: count, Severity: segment.value.Severity, Priority: segment.descriptor.Priority, Open: segment.descriptor.Open})
	}
	visible := items[:0]
	for _, item := range items {
		if item.Count > 0 {
			visible = append(visible, item)
		}
	}
	slices.SortStableFunc(visible, func(a, b activityItem) int { return b.Priority - a.Priority })
	return visible
}

func (i activityItem) wide() string {
	text := strconv.Itoa(i.Count) + " " + i.Label
	if i.Severity == "error" || i.Severity == "warning" {
		if !strings.Contains(i.Label, i.Severity) && i.Label != "failed" && i.Label != "waiting for you" {
			text += " (" + i.Severity + ")"
		}
	}
	return text
}

func (i activityItem) narrow() string { return i.Glyph + strconv.Itoa(i.Count) }

// renderActivity lays the items out in one row of at most width columns. Below 50 columns items
// become glyph plus number; the lowest priority drops first and no item is cut.
func renderActivity(items []activityItem, width int) string {
	if len(items) == 0 || width <= 0 {
		return ""
	}
	form := activityItem.wide
	separator := "  "
	if width < narrowActivity {
		form, separator = activityItem.narrow, " "
	}
	for keep := len(items); keep > 0; keep-- {
		parts := make([]string, keep)
		for i, item := range items[:keep] {
			parts[i] = tui.StripTerminalSequences(form(item))
		}
		line := strings.Join(parts, separator)
		if tui.VisibleWidth(line) <= width {
			return line
		}
	}
	return ""
}

func activitySummary(items []activityItem) string {
	if len(items) == 0 {
		return "idle"
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = item.wide()
	}
	return strings.Join(parts, ", ")
}

// activityLine owns the widget: it republishes only when the derived items change.
type activityLine struct {
	mu     sync.Mutex
	items  []activityItem
	signal string
	stop   chan struct{}
	done   chan struct{}
}

func (a *activityLine) current() []activityItem {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.items)
}

// startActivity shows the line above the editor and keeps it current until Close.
func (a *app) startActivity(ctx sdk.Context, s *session) {
	if !s.settingBool("activity.enabled", true) || !ctx.HasUI() {
		return
	}
	line := &activityLine{stop: make(chan struct{}), done: make(chan struct{})}
	s.activity = line
	placement := "aboveEditor"
	if s.settingString("activity.placement", "above-editor") == "below-editor" {
		placement = "belowEditor"
	}
	publish := func() {
		items := s.activityItems()
		signature := fmt.Sprint(items)
		line.mu.Lock()
		changed := signature != line.signal
		line.signal, line.items = signature, items
		line.mu.Unlock()
		if !changed {
			return
		}
		render := func(width int) []string {
			if text := renderActivity(line.current(), width); text != "" {
				return []string{text}
			}
			return nil
		}
		_ = ctx.SetWidgetRenderer(activityKey, render, sdk.WidgetOptions{"placement": placement})
	}
	publish()
	go func() {
		defer close(line.done)
		ticker := time.NewTicker(activityPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-line.stop:
				return
			case <-ticker.C:
				publish()
			}
		}
	}()
}

func (a *activityLine) close(ctx sdk.Context) {
	close(a.stop)
	<-a.done
	_ = ctx.SetWidgetRenderer(activityKey, nil)
}

// activityShortcut runs the inspector of the first non-zero item. Only Workbench's own commands run;
// a contributed item's open target is named, never executed, because the shortcut is not a click on it.
func (a *app) activityShortcut(ctx sdk.Context) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	items := s.activityItems()
	if len(items) == 0 {
		notify(ctx, "Workbench activity: idle.")
		return nil
	}
	notify(ctx, "Workbench activity: "+activitySummary(items))
	if args, ok := strings.CutPrefix(items[0].Open, "/workbench "); ok {
		return a.workbench(ctx, args)
	}
	if items[0].Open != "" {
		notify(ctx, "Open with "+items[0].Open)
	}
	return nil
}
