package api

// Range is an inclusive 1-based source-line interval.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SnapshotViewParams asks Workbench to read a local file for a running tool call and render it with hashline anchors.
type SnapshotViewParams struct {
	ToolCallID string  `json:"toolCallId"`
	URI        string  `json:"uri"`
	Ranges     []Range `json:"ranges,omitempty"`
	Raw        bool    `json:"raw,omitempty"`
}

// SnapshotView is the rendered block the caller must include verbatim in its tool result.
type SnapshotView struct {
	Block    string  `json:"block"`
	URI      string  `json:"uri"`
	Tag      string  `json:"tag,omitempty"`
	Revision string  `json:"revision"`
	Shown    []Range `json:"shown"`
}

// SnapshotStatusParams names the resource whose edit-tracking state is requested.
type SnapshotStatusParams struct {
	URI string `json:"uri"`
}

// SnapshotStatus reports counts only, never content.
type SnapshotStatus struct {
	URI       string `json:"uri"`
	Tag       string `json:"tag,omitempty"`
	Revision  string `json:"revision,omitempty"`
	Retained  bool   `json:"retained"`
	SeenLines int    `json:"seenLines"`
	Stale     bool   `json:"stale"`
}

// SnapshotTouchedParams lists files the caller changed.
type SnapshotTouchedParams struct {
	URIs []string `json:"uris"`
}

// SnapshotTouched holds the revalidated state of each hinted file, in request order.
type SnapshotTouched struct {
	Files []SnapshotStatus `json:"files"`
}
