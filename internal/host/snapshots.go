package host

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/fileedit"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type pendingView struct {
	uri, revision, epoch, block string
	spans                       []hashline.LineSpan
	tagged                      bool
}

func (s *session) rememberRead(callID, epoch string, target resource.Target, page resource.ReadResult) {
	if s.snapshots == nil || callID == "" || target.Scheme != "file" || page.Facts.CoordinateSpace != "source-lines" || page.Facts.Revision == "" {
		return
	}
	file, info, err := fileio.OpenRegular(target.Path)
	if err != nil {
		return
	}
	defer file.Close()
	if info.Size() > 4<<20 {
		return
	}
	source, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(source) > 4<<20 || hashline.SourceRevision(source) != page.Facts.Revision {
		return
	}
	snapshot, err := s.snapshots.Retain(page.Facts.CanonicalURI, source)
	if err != nil {
		return
	}
	spans := make([]hashline.LineSpan, len(page.Facts.FullyShownRanges))
	for i, span := range page.Facts.FullyShownRanges {
		spans[i] = hashline.LineSpan{Start: span.Start, End: span.End}
	}
	view := pendingView{uri: snapshot.URI, revision: snapshot.Revision, epoch: epoch, block: page.Content, spans: spans, tagged: !target.Selection.Raw && page.Facts.Tag != "" && page.Facts.Tag == snapshot.Tag}
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if len(s.pendingViews) >= 256 {
		clear(s.pendingViews)
	}
	s.pendingViews[callID] = append(s.pendingViews[callID], view)
}

func (s *session) rememberCommitted(effect hashline.Effects, outcome fileedit.Outcome, epoch string) {
	if s.snapshots == nil || effect.Delete || !outcome.Committed || effect.Bytes == nil {
		return
	}
	path := outcome.Path
	if outcome.Destination != "" {
		path = outcome.Destination
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return
	}
	uri := resource.FileURI(canonical)
	snapshot, err := s.snapshots.Retain(uri, effect.Bytes)
	if err != nil {
		return
	}
	_ = s.snapshots.Delivered(uri, snapshot.Revision, epoch, effect.Seen, false)
}

func (s *session) rememberWritten(callID, epoch string, target resource.Target, raw []byte, receipt string) {
	if s.snapshots == nil || callID == "" || len(raw) > 4<<20 {
		return
	}
	source, err := hashline.Normalize(raw)
	if err != nil {
		return
	}
	snapshot, err := s.snapshots.Retain(target.URI, raw)
	if err != nil {
		return
	}
	var spans []hashline.LineSpan
	if len(source.Lines) > 0 {
		spans = []hashline.LineSpan{{Start: 1, End: len(source.Lines)}}
	}
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if len(s.pendingViews) >= 256 {
		clear(s.pendingViews)
	}
	s.pendingViews[callID] = append(s.pendingViews[callID], pendingView{uri: target.URI, revision: snapshot.Revision, epoch: epoch, block: receipt, spans: spans})
}

// trackCall records which tool runs a call so snapshots.view can verify its caller owns it.
func (a *app) trackCall(_ sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	id, _ := data["toolCallId"].(string)
	name, _ := data["toolName"].(string)
	if id == "" || name == "" {
		return nil, nil
	}
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if len(s.runningCalls) >= 256 {
		clear(s.runningCalls)
	}
	s.runningCalls[id] = name
	if len(s.callRoots) >= 512 {
		clear(s.callRoots)
	}
	s.callRoots[id] = s.location.Current().Cwd
	return nil, nil
}

// callRoot is the working root a call resolved against; absent for calls from before this runtime.
func (s *session) callRoot(id string) string {
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	return s.callRoots[id]
}

func (a *app) confirmRead(ctx sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	id, _ := data["toolCallId"].(string)
	s.viewsMu.Lock()
	views := s.pendingViews[id]
	delete(s.pendingViews, id)
	delete(s.runningCalls, id)
	s.viewsMu.Unlock()
	for _, view := range views {
		if view.block != "" && (containsTextBlock(data["content"], view.block) || containsTextBlock(data["content"], s.redactedView(view.block))) {
			_ = s.snapshots.Delivered(view.uri, view.revision, view.epoch, view.spans, view.tagged)
		}
	}
	return nil, nil
}

func containsTextBlock(content any, needle string) bool {
	if text, ok := content.(string); ok {
		return strings.Contains(text, needle)
	}
	if blocks, ok := content.([]any); ok {
		for _, raw := range blocks {
			if block, ok := raw.(map[string]any); ok && block["type"] == "text" {
				if text, ok := block["text"].(string); ok && strings.Contains(text, needle) {
					return true
				}
			}
		}
	}
	return false
}

func (a *app) invalidateSeen(_ sdk.Context, _ map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	s.viewsMu.Lock()
	clear(s.pendingViews)
	s.viewsMu.Unlock()
	return nil, s.snapshots.SetEpoch(hex.EncodeToString(nonce[:]))
}

// redactedView is the block as the model sees it after the secrets hook ran, so seen-state still matches.
func (s *session) redactedView(block string) string {
	if s.secrets == nil {
		return block
	}
	if redacted, active, err := s.secrets.redactText(block); err == nil && active {
		return redacted
	}
	return block
}
