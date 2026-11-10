package host

import (
	"context"
	"errors"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestReadAheadKeepsFittingPagesAndHandsBackTheRest(t *testing.T) {
	failed := errors.New("no such file")
	ahead := startReadAhead(context.Background(), 4, 3, func(_ context.Context, i int) (resource.ReadResult, error) {
		if i == 2 {
			return resource.ReadResult{}, failed
		}
		return resource.ReadResult{Content: []string{"small", "line one\nline two\nline three"}[i]}, nil
	})
	defer ahead.stop()
	if page, err, early := ahead.take(context.Background(), 0, resource.Budget{Bytes: 64, Lines: 5}); !early || err != nil || page.Content != "small" {
		t.Fatalf("fitting page = %q %v %v", page.Content, err, early)
	}
	if _, _, early := ahead.take(context.Background(), 1, resource.Budget{Bytes: 64, Lines: 2}); early {
		t.Fatal("a page over the remaining line budget was kept")
	}
	if _, err, early := ahead.take(context.Background(), 2, resource.Budget{Bytes: 64, Lines: 5}); !early || !errors.Is(err, failed) {
		t.Fatalf("read failure = %v %v", err, early)
	}
}

func TestReadAheadStopIsNotAReadFailure(t *testing.T) {
	started := make(chan struct{})
	ahead := startReadAhead(context.Background(), 2, 2, func(ctx context.Context, i int) (resource.ReadResult, error) {
		if i == 0 {
			return resource.ReadResult{Content: "first"}, nil
		}
		close(started)
		<-ctx.Done()
		return resource.ReadResult{}, ctx.Err()
	})
	<-started
	ahead.stop()
	if _, _, early := ahead.take(context.Background(), 1, resource.Budget{Bytes: 64, Lines: 5}); early {
		t.Fatal("a fetch cancelled by stop was reported as a result")
	}
	if startReadAhead(context.Background(), 1, 5, nil) != nil {
		t.Fatal("serial reads must not start workers")
	}
}
