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

// snapshotFileBytes applies the configured edit.snapshot limits to the store and returns the per-file bound.
func (s *session) snapshotFileBytes() int64 {
	limits := hashline.StoreLimits{Resources: 256, Versions: 4, Bytes: 64 << 20, FileBytes: 4 << 20}
	if s.settings != nil {
		if snapshot, err := s.settings.Current(); err == nil {
			limits.Resources = intValue(snapshot, "edit.snapshot.max-paths", limits.Resources)
			limits.Versions = intValue(snapshot, "edit.snapshot.max-versions", limits.Versions)
			limits.Bytes = int64(intValue(snapshot, "edit.snapshot.max-bytes", int(limits.Bytes)))
			limits.FileBytes = int64(intValue(snapshot, "edit.snapshot.max-file-bytes", int(limits.FileBytes)))
		}
	}
	if s.snapshots != nil {
		_ = s.snapshots.SetLimits(limits)
	}
	return limits.FileBytes
}

func (s *session) rememberRead(callID, epoch string, target resource.Target, page resource.ReadResult) {
	if s.snapshots == nil || callID == "" || target.Scheme != "file" || page.Facts.CoordinateSpace != "source-lines" || page.Facts.Revision == "" {
		return
	}
	limit := s.snapshotFileBytes()
	file, info, err := fileio.OpenRegular(target.Path)
	if err != nil {
		return
	}
	defer file.Close()
	if info.Size() > limit {
		return
	}
	source, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(source)) > limit || hashline.SourceRevision(source) != page.Facts.Revision {
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
	s.addPendingView(callID, view)
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
	if s.snapshots == nil || callID == "" || int64(len(raw)) > s.snapshotFileBytes() {
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
	s.addPendingView(callID, pendingView{uri: target.URI, revision: snapshot.Revision, epoch: epoch, block: receipt, spans: spans})
}

// addPendingView requires viewsMu.
func (s *session) addPendingView(callID string, view pendingView) {
	views, _ := s.pendingViews.get(callID)
	s.pendingViews.put(callID, append(views, view))
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
	s.runningCalls.put(id, name)
	s.callRoots.put(id, s.location.Current().Cwd)
	return nil, nil
}

// callRoot is the working root a call resolved against; absent for calls from before this runtime.
func (s *session) callRoot(id string) string {
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	root, _ := s.callRoots.get(id)
	return root
}

func (a *app) confirmRead(ctx sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	id, _ := data["toolCallId"].(string)
	s.viewsMu.Lock()
	views, _ := s.pendingViews.get(id)
	s.pendingViews.remove(id)
	s.runningCalls.remove(id)
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
	s.pendingViews.reset()
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
