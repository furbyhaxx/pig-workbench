package host

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
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
	view := pendingView{uri: snapshot.URI, revision: snapshot.Revision, epoch: epoch, block: page.Content, spans: spans, tagged: page.Facts.Tag != "" && page.Facts.Tag == snapshot.Tag}
	s.viewsMu.Lock()
	defer s.viewsMu.Unlock()
	if len(s.pendingViews) >= 256 {
		clear(s.pendingViews)
	}
	s.pendingViews[callID] = append(s.pendingViews[callID], view)
}

func (a *app) confirmRead(ctx sdk.Context, data map[string]any) (any, error) {
	if data["toolName"] != "read" {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	id, _ := data["toolCallId"].(string)
	s.viewsMu.Lock()
	views := s.pendingViews[id]
	delete(s.pendingViews, id)
	s.viewsMu.Unlock()
	if failed, _ := data["isError"].(bool); failed {
		return nil, nil
	}
	var text strings.Builder
	if blocks, ok := data["content"].([]any); ok {
		for _, raw := range blocks {
			if block, ok := raw.(map[string]any); ok && block["type"] == "text" {
				if value, ok := block["text"].(string); ok {
					text.WriteString(value)
				}
			}
		}
	}
	if value, ok := data["content"].(string); ok {
		text.WriteString(value)
	}
	for _, view := range views {
		if view.block != "" && strings.Contains(text.String(), view.block) {
			_ = s.snapshots.Delivered(view.uri, view.revision, view.epoch, view.spans, view.tagged)
		}
	}
	return nil, nil
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
