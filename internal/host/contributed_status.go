package host

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
)

type statusDescriptor struct {
	Label      string `json:"label"`
	Priority   int    `json:"priority,omitempty"`
	MinColumns int    `json:"minColumns,omitempty"`
	MaxColumns int    `json:"maxColumns,omitempty"`
	Align      string `json:"align,omitempty"`
	Placement  string `json:"placement,omitempty"`
	Glyph      string `json:"glyph,omitempty"`
	Open       string `json:"open,omitempty"`
}

func parseStatusSegment(data []byte) (statusDescriptor, error) {
	var d statusDescriptor
	if err := decodeStrict(data, &d); err != nil {
		return d, err
	}
	invalid := func(message string) (statusDescriptor, error) {
		return statusDescriptor{}, fmt.Errorf("%w: statusSegment %s", errInvalidContribution, message)
	}
	if d.Label == "" || !plainField(d.Label, 64) {
		return invalid("label must be 1-64 bytes of plain text")
	}
	if d.Priority < 0 || d.Priority > 1000 || d.MinColumns < 0 || d.MaxColumns < 0 || d.MaxColumns > 64 || d.MaxColumns != 0 && d.MinColumns > d.MaxColumns {
		return invalid("priority or column bounds are out of range")
	}
	if d.Align != "" && d.Align != "left" && d.Align != "right" {
		return invalid("align must be left or right")
	}
	if d.Placement == "" {
		d.Placement = "status"
	}
	if d.Placement != "status" && d.Placement != "activity" {
		return invalid("placement must be status or activity")
	}
	if d.Glyph != "" && (tui.VisibleWidth(d.Glyph) != 1 || !plainField(d.Glyph, 8)) {
		return invalid("glyph must be one terminal cell")
	}
	if d.Open != "" && (!plainField(d.Open, 512) || !strings.HasPrefix(d.Open, "/") && !strings.Contains(d.Open, "://")) {
		return invalid("open must be a slash command or a resource URI")
	}
	return d, nil
}

type statusValue struct {
	Text     string `json:"text"`
	Style    string `json:"style,omitempty"`
	Count    *int   `json:"count,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type statusSegment struct {
	registrationID string
	owner          string
	key            string
	descriptor     statusDescriptor
	value          statusValue
	updated        time.Time
	pending        *time.Timer
}

// statusBoard composes contributed status segments; Workbench owns the host status keys.
type statusBoard struct {
	mu       sync.Mutex
	segments map[string]*statusSegment
	host     func() (sdk.Context, bool)
	closed   bool
}

func newStatusBoard(host func() (sdk.Context, bool)) *statusBoard {
	return &statusBoard{segments: make(map[string]*statusSegment), host: host}
}

func statusKey(segment *statusSegment) string {
	return "workbench/" + segment.owner + "/" + segment.key
}

// Activity returns live activity-placement items in priority order.
func (b *statusBoard) activity() []statusSegment {
	b.mu.Lock()
	defer b.mu.Unlock()
	var items []statusSegment
	for _, segment := range b.segments {
		if segment.descriptor.Placement == "activity" {
			copied := *segment
			copied.pending = nil
			items = append(items, copied)
		}
	}
	slices.SortFunc(items, func(a, b statusSegment) int {
		if a.descriptor.Priority != b.descriptor.Priority {
			return b.descriptor.Priority - a.descriptor.Priority
		}
		return strings.Compare(a.owner+"/"+a.key, b.owner+"/"+b.key)
	})
	return items
}

func (b *statusBoard) add(segment *statusSegment) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.segments[segment.registrationID] = segment
}

func (b *statusBoard) remove(registrationID string) {
	b.mu.Lock()
	segment := b.segments[registrationID]
	delete(b.segments, registrationID)
	if segment != nil && segment.pending != nil {
		segment.pending.Stop()
	}
	b.mu.Unlock()
	if segment != nil && segment.descriptor.Placement == "status" {
		if ctx, ok := b.host(); ok {
			ctx.SetStatus(statusKey(segment), "")
		}
	}
}

// close clears the status segments through ctx, the live shutdown context: the session-start
// context the board retains is stale once a replacement runs and its UI calls would wait on the host.
func (b *statusBoard) close(ctx sdk.Context) {
	b.mu.Lock()
	b.closed = true
	segments := make([]*statusSegment, 0, len(b.segments))
	for _, segment := range b.segments {
		if segment.pending != nil {
			segment.pending.Stop()
		}
		segments = append(segments, segment)
	}
	b.mu.Unlock()
	for _, segment := range segments {
		if segment.descriptor.Placement == "status" {
			ctx.SetStatus(statusKey(segment), "")
		}
	}
}

// renderStatus fits one status-placement segment to its column bounds.
func renderStatus(segment statusSegment, width int) string {
	text := strings.TrimSpace(segment.value.Text)
	if segment.value.Count != nil {
		if *segment.value.Count == 0 {
			return ""
		}
		text = strconv.Itoa(*segment.value.Count) + " " + segment.descriptor.Label
	}
	if text == "" {
		return ""
	}
	if segment.value.Style == "warning" || segment.value.Style == "error" || segment.value.Severity == "warning" || segment.value.Severity == "error" {
		word := segment.value.Severity
		if word == "" {
			word = segment.value.Style
		}
		if !strings.Contains(strings.ToLower(text), word) {
			text += " (" + word + ")"
		}
	}
	limit := 64
	if segment.descriptor.MaxColumns > 0 {
		limit = segment.descriptor.MaxColumns
	}
	if width > 0 {
		limit = min(limit, max(1, width/3))
	}
	if segment.descriptor.MinColumns > 0 && width > 0 && width < segment.descriptor.MinColumns*2 && segment.descriptor.Glyph != "" {
		return segment.descriptor.Glyph
	}
	return tui.StripTerminalSequences(tui.TruncateToWidth(text, limit, "…", false))
}

const statusMinInterval = 250 * time.Millisecond

func (b *statusBoard) update(ctx sdk.Context, registrationID string, value statusValue) error {
	b.mu.Lock()
	segment := b.segments[registrationID]
	if segment == nil || b.closed {
		b.mu.Unlock()
		return fmt.Errorf("status segment is not registered")
	}
	segment.value = value
	if segment.descriptor.Placement != "status" {
		segment.updated = time.Now()
		b.mu.Unlock()
		return nil
	}
	wait := statusMinInterval - time.Since(segment.updated)
	if wait > 0 {
		if segment.pending == nil {
			segment.pending = time.AfterFunc(wait, func() { b.flush(registrationID) })
		}
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()
	// A nested startup request must not re-enter the session-start host call lane.
	b.flushWithContext(ctx, registrationID)
	return nil
}

func (b *statusBoard) flush(registrationID string) {
	if ctx, ok := b.host(); ok {
		detached(ctx, func() { b.flushWithContext(ctx, registrationID) })
	}
}

func (b *statusBoard) flushWithContext(ctx sdk.Context, registrationID string) {
	b.mu.Lock()
	segment := b.segments[registrationID]
	if segment == nil || b.closed {
		b.mu.Unlock()
		return
	}
	segment.pending = nil
	segment.updated = time.Now()
	snapshot := *segment
	b.mu.Unlock()
	ctx.SetStatus(statusKey(&snapshot), renderStatus(snapshot, ctx.Width()))
}

func installStatusSegment(s *session, registration contribution.Registration) (func(), error) {
	descriptor, err := parseStatusSegment(registration.Descriptor)
	if err != nil {
		return nil, err
	}
	s.status.add(&statusSegment{registrationID: registration.RegistrationID, owner: registration.Owner, key: registration.Key, descriptor: descriptor})
	return func() { s.status.remove(registration.RegistrationID) }, nil
}

func (a *app) statusUpdate(ctx sdk.Context, s *session, request api.Request) (any, *api.Error) {
	registration, err := s.contributions.Get(request.From, request.RegistrationID)
	if err != nil {
		return nil, contributionProtocolError(err)
	}
	if registration.Kind != contribution.KindStatusSegment {
		return nil, &api.Error{Code: "invalidRequest", Field: "registrationId", Message: "status.update requires a statusSegment registration", Outcome: "notApplied"}
	}
	var value statusValue
	if err := api.Decode(request.Params, &value); err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "status.update expects text, style, count and severity", Outcome: "notApplied"}
	}
	if !plainField(value.Text, 256) || tui.VisibleWidth(value.Text) > 64 {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.text", Message: "text must be plain and at most 64 columns", Outcome: "notApplied"}
	}
	if value.Style != "" && !slices.Contains([]string{"normal", "dim", "success", "warning", "error"}, value.Style) {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.style", Message: "style must be normal, dim, success, warning or error", Outcome: "notApplied"}
	}
	if value.Severity != "" && !slices.Contains([]string{"info", "warning", "error"}, value.Severity) {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.severity", Message: "severity must be info, warning or error", Outcome: "notApplied"}
	}
	if value.Count != nil && *value.Count < 0 {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.count", Message: "count must not be negative", Outcome: "notApplied"}
	}
	if err := s.status.update(ctx, request.RegistrationID, value); err != nil {
		return nil, &api.Error{Code: "notFound", Message: err.Error(), Outcome: "notApplied"}
	}
	s.pokeActivity()
	return map[string]any{"accepted": true, "composedByWorkbench": true}, nil
}
