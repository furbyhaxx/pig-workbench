package host

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/pathdisplay"
)

const (
	activityKey = "workbench-activity"
	// activityMinInterval coalesces event-driven publishing to at most four per second.
	activityMinInterval = 250 * time.Millisecond
	// activityFallbackInterval heals a missed event and refreshes the peer count, the one fact read from disk.
	activityFallbackInterval = 5 * time.Second
	narrowActivity           = 50
)

// activityPhase is the turn state the line leads with. Compaction can run inside or outside a turn,
// so both facts are kept and compaction wins while it lasts.
type activityPhase struct {
	mu         sync.Mutex
	working    bool
	compacting bool
	since      time.Time
}

func (p *activityPhase) setWorking(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.working != on {
		p.working, p.since = on, time.Now()
	}
}

func (p *activityPhase) setCompacting(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.compacting = on
}

func (p *activityPhase) get() (working, compacting bool, since time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.working, p.compacting, p.since
}

func elapsedText(d time.Duration) string {
	seconds := int(d / time.Second)
	if seconds < 60 {
		return strconv.Itoa(seconds) + "s"
	}
	return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
}

// activityItem is one counted fact of the activity line. Zero counts are not shown.
type activityItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Glyph string `json:"glyph"`
	Count int    `json:"count"`
	// Text replaces "<count> <label>" for items that state a fact instead of a count.
	Text     string `json:"text,omitempty"`
	Severity string `json:"severity,omitempty"`
	Priority int    `json:"priority"`
	// Open is the command that inspects the item.
	Open string `json:"open,omitempty"`
}

// activityItems derives the line from the live registries: no I/O beyond reading them.
func (s *session) activityItems() []activityItem {
	var items []activityItem
	asks := 0
	if e := s.endpoint; e != nil {
		e.mu.Lock()
		asks = e.waiting
		e.mu.Unlock()
	}
	working, compacting, since := s.phase.get()
	state := activityItem{Key: "state", Text: "Ready", Glyph: "●", Count: 1, Priority: 110, Open: "/workbench"}
	switch {
	case asks > 0:
		state.Text, state.Severity = "Waiting for you", "warning"
	case compacting:
		state.Text = "Compacting"
	case working:
		state.Text = "Working " + elapsedText(time.Since(since))
	}
	items = append(items, state)
	if s.jobs != nil {
		running, failed := 0, 0
		for _, job := range s.jobs.List() {
			switch {
			case job.Unknown, job.FailedToStart:
				failed++
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
	items = append(items, activityItem{Key: "peers", Label: "live sessions", Glyph: "⚇", Count: s.livePeers(), Priority: 60, Open: "/workbench sessions"})
	if c := s.contextFrames; c != nil {
		c.mu.Lock()
		files := 0
		for key := range c.views {
			if strings.HasPrefix(key, "instructions.") {
				files++
			}
		}
		c.mu.Unlock()
		items = append(items, activityItem{Key: "instructions", Label: "instructions", Glyph: "≡", Count: files, Priority: 50, Open: "/workbench instructions"})
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
	if s.location != nil {
		if current := s.location.Current(); current.Cwd != current.Origin {
			items = append(items, activityItem{Key: "location", Text: "in " + pathdisplay.Display(pathdisplay.Target{Value: current.Cwd, Root: current.Origin, NoFish: true}, 32), Glyph: "⌂", Count: 1, Priority: 30, Open: "/cd"})
		}
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
	if i.Text != "" {
		return i.Text
	}
	text := strconv.Itoa(i.Count) + " " + i.Label
	if i.Severity == "error" || i.Severity == "warning" {
		if !strings.Contains(i.Label, i.Severity) && i.Label != "failed" && i.Label != "waiting for you" {
			text += " (" + i.Severity + ")"
		}
	}
	return text
}

func (i activityItem) narrow() string {
	switch {
	case i.Key == "state":
		return i.Text
	case i.Text != "":
		return i.Glyph
	}
	return i.Glyph + strconv.Itoa(i.Count)
}

// livePeers counts discoverable sessions. With a line running it answers from the count that line
// last read, so deriving the items never touches the registry on disk.
func (s *session) livePeers() int {
	if line := s.activity; line != nil {
		line.mu.Lock()
		defer line.mu.Unlock()
		return line.peers
	}
	return s.readPeers()
}

func (s *session) readPeers() int {
	if s.endpoint == nil {
		return 0
	}
	peers, err := s.endpoint.peers(context.Background())
	if err != nil {
		return 0
	}
	return len(peers)
}

// pokeActivity asks the line to republish after a state change; it never blocks.
func (s *session) pokeActivity() {
	if line := s.activity; line != nil {
		select {
		case line.poke <- struct{}{}:
		default:
		}
	}
}

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
	peers  int
	poke   chan struct{}
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
	line := &activityLine{poke: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), peers: s.readPeers()}
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
		fallback := time.NewTicker(activityFallbackInterval)
		defer fallback.Stop()
		second := time.NewTicker(time.Second)
		defer second.Stop()
		var last time.Time
		for {
			select {
			case <-line.stop:
				return
			case <-line.poke:
				if wait := activityMinInterval - time.Since(last); wait > 0 {
					select {
					case <-line.stop:
						return
					case <-time.After(wait):
					}
				}
			case <-second.C:
				if working, compacting, _ := s.phase.get(); !working || compacting {
					continue
				}
			case <-fallback.C:
				peers := s.readPeers()
				line.mu.Lock()
				line.peers = peers
				line.mu.Unlock()
			}
			last = time.Now()
			if detached(ctx, publish) {
				return
			}
		}
	}()
}

func (a *activityLine) close(ctx sdk.Context) {
	close(a.stop)
	<-a.done
	_ = ctx.SetWidgetRenderer(activityKey, nil)
}

const defaultActivityShortcut = "alt+w"

var shortcutSyntax = regexp.MustCompile(`^[a-z0-9]+(\+[a-z0-9]+)*$`)

// activityShortcutKey reads activity.shortcut from the user layer. PiG registers shortcuts while the
// extension loads, before a session's settings exist, so the key is fixed per load and a change applies
// on the next /reload. The setting is user/runtime-only; the project layer is never consulted.
func activityShortcutKey() (key, note string) {
	agent, err := sdk.AgentDir()
	if err != nil {
		return defaultActivityShortcut, "default: " + err.Error()
	}
	manager, err := config.New(config.Paths{User: filepath.Join(agent, "Workbench.toml"), Project: filepath.Join(agent, "no-project", "Workbench.toml")}, nil)
	if err == nil {
		err = manager.Load(context.Background(), false)
	}
	if err != nil {
		return defaultActivityShortcut, "default; Workbench.toml is unreadable"
	}
	snapshot, err := manager.Current()
	if err != nil {
		return defaultActivityShortcut, "default; Workbench.toml is unreadable"
	}
	entry, ok := snapshot.Get("activity.shortcut")
	value, _ := entry.Value.(string)
	if !ok || value == "" {
		return defaultActivityShortcut, ""
	}
	if !shortcutSyntax.MatchString(value) {
		return defaultActivityShortcut, fmt.Sprintf("default; %q is not a key such as alt+w or ctrl+shift+s", value)
	}
	return value, ""
}

// activityShortcut runs the inspector of the first non-zero item. Only Workbench's own commands run;
// a contributed item's open target is named, never executed, because the shortcut is not a click on it.
func (a *app) activityShortcut(ctx sdk.Context) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	items := s.activityItems()
	a.notify(ctx, "Workbench activity: "+activitySummary(items))
	for _, item := range items {
		if item.Key == "state" {
			continue
		}
		if args, ok := strings.CutPrefix(item.Open, "/workbench "); ok {
			return a.workbench(ctx, args)
		}
		if item.Open != "" {
			a.notify(ctx, "Open with "+item.Open)
			return nil
		}
	}
	return a.workbench(ctx, "")
}
