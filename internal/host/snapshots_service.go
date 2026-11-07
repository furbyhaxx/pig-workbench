package host

import (
	"context"
	"io"
	"path/filepath"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/api"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func (a *app) snapshotsService(work context.Context, host sdk.Context, s *session, request api.Request) (any, *api.Error) {
	switch request.Method {
	case "snapshots.view":
		var params api.SnapshotViewParams
		if err := api.Decode(request.Params, &params); err != nil || params.ToolCallID == "" || params.URI == "" {
			return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "snapshots.view requires toolCallId and uri", Outcome: "notApplied"}
		}
		return a.snapshotView(work, host, s, request, params)
	case "snapshots.status":
		var params api.SnapshotStatusParams
		if err := api.Decode(request.Params, &params); err != nil || params.URI == "" {
			return nil, &api.Error{Code: "invalidRequest", Field: "params", Message: "snapshots.status requires uri"}
		}
		status, apiErr := s.snapshotStatus(params.URI)
		if apiErr != nil {
			return nil, apiErr
		}
		return status, nil
	default:
		var params api.SnapshotTouchedParams
		if err := api.Decode(request.Params, &params); err != nil || len(params.URIs) == 0 || len(params.URIs) > 64 {
			return nil, &api.Error{Code: "invalidRequest", Field: "params.uris", Message: "snapshots.touched requires 1–64 uris"}
		}
		files := make([]api.SnapshotStatus, 0, len(params.URIs))
		for _, uri := range params.URIs {
			status, apiErr := s.snapshotStatus(uri)
			if apiErr != nil {
				return nil, apiErr
			}
			files = append(files, status)
		}
		return api.SnapshotTouched{Files: files}, nil
	}
}

func (a *app) snapshotView(work context.Context, host sdk.Context, s *session, request api.Request, params api.SnapshotViewParams) (any, *api.Error) {
	s.viewsMu.Lock()
	toolName := s.runningCalls[params.ToolCallID]
	s.viewsMu.Unlock()
	denied := &api.Error{Code: "permissionDenied", Field: "params.toolCallId", Message: "toolCallId is not a running call of a tool this owner registered", Outcome: "notApplied"}
	if toolName == "" {
		return nil, denied
	}
	tools, err := host.GetAllTools()
	if err != nil {
		return nil, protocolError(err)
	}
	if !ownsTool(tools, request.From.Owner, toolName) {
		return nil, denied
	}
	configuration, err := s.settings.Current()
	if err != nil {
		return nil, protocolError(err)
	}
	captured, done, err := s.location.Acquire(request.ID, false)
	if err != nil {
		return nil, protocolError(err)
	}
	defer done()
	target, err := resource.ParsePath(captured.Cwd, params.URI, "")
	if err != nil {
		return nil, &api.Error{Code: "invalidRequest", Field: "params.uri", Message: err.Error(), Outcome: "notApplied"}
	}
	if target.Scheme != "file" {
		return nil, &api.Error{Code: "unsupported", Field: "params.uri", Message: "snapshots.view anchors local files; " + target.Scheme + ":// resources cannot be edited through Workbench", Outcome: "notApplied"}
	}
	target.Selection = resource.Selection{Raw: params.Raw}
	for _, r := range params.Ranges {
		target.Selection.Ranges = append(target.Selection.Ranges, resource.Range{Start: r.Start, End: r.End})
	}
	target.Selection.Explicit = len(params.Ranges) > 0
	policy := configuration.Read()
	page, err := s.router.Read(work, target, resource.Budget{Bytes: policy.MaxBytes, Lines: policy.MaxLines})
	if err != nil {
		return nil, protocolError(err)
	}
	s.rememberRead(params.ToolCallID, s.snapshots.Epoch(), target, page)
	shown := make([]api.Range, len(page.Facts.ShownRanges))
	for i, r := range page.Facts.ShownRanges {
		shown[i] = api.Range{Start: r.Start, End: r.End}
	}
	return api.SnapshotView{Block: page.Content, URI: page.Facts.CanonicalURI, Tag: page.Facts.Tag, Revision: page.Facts.Revision, Shown: shown}, nil
}

func (s *session) snapshotStatus(uri string) (api.SnapshotStatus, *api.Error) {
	target, err := resource.ParsePath(s.location.Current().Cwd, uri, "")
	if err != nil {
		return api.SnapshotStatus{}, &api.Error{Code: "invalidRequest", Field: "params.uri", Message: err.Error()}
	}
	if target.Scheme != "file" {
		return api.SnapshotStatus{}, &api.Error{Code: "unsupported", Field: "params.uri", Message: "edit tracking covers local files"}
	}
	canonical := target.Path
	if resolved, err := filepath.EvalSymlinks(target.Path); err == nil {
		canonical = resolved
	}
	status := api.SnapshotStatus{URI: resource.FileURI(canonical)}
	snapshot, ok := s.snapshots.Latest(status.URI)
	if !ok {
		return status, nil
	}
	status.Retained, status.Tag, status.Revision = true, snapshot.Tag, snapshot.Revision
	if snapshot.Epoch == s.snapshots.Epoch() {
		for _, span := range snapshot.Seen {
			status.SeenLines += span.End - span.Start + 1
		}
	}
	current, ok := currentRevision(canonical)
	status.Stale = !ok || current != snapshot.Revision
	return status, nil
}

func currentRevision(path string) (string, bool) {
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		return "", false
	}
	defer file.Close()
	if info.Size() > 4<<20 {
		return "", false
	}
	raw, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return "", false
	}
	return hashline.SourceRevision(raw), true
}
