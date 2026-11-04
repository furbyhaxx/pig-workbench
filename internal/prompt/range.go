package prompt

// The range global function is adapted from gonja v2.9.1
// builtins/global_functions.go (MIT, Copyright (c) 2013-2014 Florian
// Schlachter); see LICENSE.gonja. Differences: the producer stops when the
// render context ends instead of blocking on a send nobody receives, it is
// registered with the render so the render can join it, and a step that would
// overflow the integer range ends the series.

import (
	"errors"

	"github.com/nikolalohinski/gonja/v2/exec"
)

func rangeFunction(e *exec.Evaluator, params *exec.VarArgs) (<-chan int, error) {
	var (
		start = 0
		stop  = -1
		step  = 1
	)
	switch n := len(params.Args); n > 0 {
	case n == 1 && params.Args[0].IsInteger():
		stop = params.Args[0].Integer()
	case n == 2 && params.Args[0].IsInteger() && params.Args[1].IsInteger():
		start = params.Args[0].Integer()
		stop = params.Args[1].Integer()
	case n == 3 && params.Args[0].IsInteger() && params.Args[1].IsInteger() && params.Args[2].IsInteger():
		start = params.Args[0].Integer()
		stop = params.Args[1].Integer()
		step = params.Args[2].Integer()
	default:
		return nil, exec.ErrInvalidCall(errors.New("expected signature is [start, ]stop[, step] where all arguments are integers"))
	}

	if step == 0 {
		return nil, exec.ErrInvalidCall(errors.New("step cannot be 0"))
	}

	state := renderStateOf(e.Environment)
	if err := state.err(); err != nil {
		return nil, err
	}
	channel := make(chan int)
	started := state.start(func() {
		defer close(channel)
		for i := start; ; {
			if (step > 0 && i >= stop) || (step < 0 && i <= stop) {
				return
			}
			select {
			case channel <- i:
			case <-state.context().Done():
				return
			}
			next := i + step
			if (step > 0 && next < i) || (step < 0 && next > i) {
				return
			}
			i = next
		}
	})
	if !started {
		close(channel)
	}
	return channel, nil
}
