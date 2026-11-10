package host

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type aheadResult struct {
	page resource.ReadResult
	err  error
}

// readAhead fetches multi-read targets concurrently; the caller still consumes them in input order within one budget.
type readAhead struct {
	results []chan aheadResult
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func startReadAhead(work context.Context, workers, count int, read func(context.Context, int) (resource.ReadResult, error)) *readAhead {
	if workers > count {
		workers = count
	}
	if workers < 2 {
		return nil
	}
	ctx, cancel := context.WithCancel(work)
	ahead := &readAhead{results: make([]chan aheadResult, count), cancel: cancel}
	for i := range ahead.results {
		ahead.results[i] = make(chan aheadResult, 1)
	}
	var next atomic.Int64
	for range workers {
		ahead.wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= count {
					return
				}
				if err := ctx.Err(); err != nil {
					ahead.results[i] <- aheadResult{err: err}
					continue
				}
				page, err := read(ctx, i)
				ahead.results[i] <- aheadResult{page: page, err: err}
			}
		})
	}
	return ahead
}

// take returns target i's early result; ok is false when it must be read again under the remaining budget.
func (r *readAhead) take(work context.Context, i int, budget resource.Budget) (resource.ReadResult, error, bool) {
	if r == nil {
		return resource.ReadResult{}, nil, false
	}
	result := <-r.results[i]
	if work.Err() != nil {
		return resource.ReadResult{}, work.Err(), true
	}
	if result.err != nil {
		// A fetch stopped by stop() is not a read failure.
		return result.page, result.err, !errors.Is(result.err, context.Canceled)
	}
	if len(result.page.Content) > budget.Bytes || strings.Count(result.page.Content, "\n")+1 > budget.Lines {
		return resource.ReadResult{}, nil, false
	}
	return result.page, nil, true
}

// stop cancels unread fetches and joins the workers.
func (r *readAhead) stop() {
	if r == nil {
		return
	}
	r.cancel()
	r.wg.Wait()
}
