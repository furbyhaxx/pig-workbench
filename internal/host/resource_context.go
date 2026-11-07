package host

import (
	"context"

	"github.com/furbyhaxx/pig-workbench/internal/location"
)

type capturedLocationKey struct{}

func withCapturedLocation(ctx context.Context, captured location.Snapshot) context.Context {
	return context.WithValue(ctx, capturedLocationKey{}, captured)
}

func (s *session) routingLocation(ctx context.Context) location.Snapshot {
	if captured, ok := ctx.Value(capturedLocationKey{}).(location.Snapshot); ok {
		return captured
	}
	return s.location.Current()
}
