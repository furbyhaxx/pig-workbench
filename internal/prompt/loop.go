package prompt

// The for-loop evaluator and its loop metadata are adapted from gonja v2.9.1
// builtins/control_structures/for.go (MIT, Copyright (c) 2013-2014 Florian
// Schlachter); see LICENSE.gonja. Differences: a loop-filter evaluation error
// is propagated instead of dropped, the render context is polled between
// iterations, and channel-backed iterables are consumed lazily.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"

	controlStructures "github.com/nikolalohinski/gonja/v2/builtins/control_structures"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/parser"
)

type strictFor struct {
	*controlStructures.ForControlStructure
}

func strictForParser(original parser.ControlStructureParser) parser.ControlStructureParser {
	return func(p *parser.Parser, args *parser.Parser) (nodes.ControlStructure, error) {
		structure, err := original(p, args)
		if err != nil {
			return nil, err
		}
		forStructure, ok := structure.(*controlStructures.ForControlStructure)
		if !ok {
			return nil, fmt.Errorf("gonja for parser produced %T", structure)
		}
		return &strictFor{ForControlStructure: forStructure}, nil
	}
}

func (f *strictFor) Execute(r *exec.Renderer, _ *nodes.ControlStructureBlock) error {
	state := renderStateOf(r.Environment)
	if err := state.err(); err != nil {
		return err
	}
	obj := r.Eval(f.ObjectEvaluator)
	if obj.IsError() {
		return valueError(obj)
	}
	return f.renderIterable(state.context(), r, obj)
}

func valueError(value *exec.Value) error {
	if err, ok := value.Interface().(error); ok {
		return err
	}
	return errors.New(value.Error())
}

// iterate visits iterable items like exec.Value.Iterate, polling ctx between
// items and receiving from channel-backed iterables without draining them.
func iterate(ctx context.Context, obj *exec.Value, fn func(idx int, key, value *exec.Value) bool) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if channel, ok := channelOf(obj); ok {
		return iterateChannel(ctx, channel, fn)
	}
	count := 0
	var ctxErr error
	obj.Iterate(func(idx, _ int, key, value *exec.Value) bool {
		if err := ctx.Err(); err != nil {
			ctxErr = err
			return false
		}
		count++
		return fn(idx, key, value)
	}, func() {})
	return count, ctxErr
}

func channelOf(value *exec.Value) (reflect.Value, bool) {
	resolved := value.Val
	if resolved.IsValid() && resolved.Kind() == reflect.Pointer {
		if resolved.IsNil() {
			return reflect.Value{}, false
		}
		resolved = resolved.Elem()
	}
	if resolved.IsValid() && resolved.Kind() == reflect.Chan {
		return resolved, true
	}
	return reflect.Value{}, false
}

func iterateChannel(ctx context.Context, channel reflect.Value, fn func(idx int, key, value *exec.Value) bool) (int, error) {
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: channel},
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
	}
	count := 0
	for {
		chosen, received, ok := reflect.Select(cases)
		if chosen == 1 {
			return count, ctx.Err()
		}
		if !ok {
			return count, nil
		}
		if !fn(count, &exec.Value{Val: received}, nil) {
			return count, nil
		}
		count++
	}
}

func (f *strictFor) renderIterable(ctx context.Context, r *exec.Renderer, obj *exec.Value) error {
	items := exec.NewDict()
	var filterErr error
	if _, err := iterate(ctx, obj, func(idx int, key, value *exec.Value) bool {
		sub := r.Inherit()
		itemContext := sub.Environment.Context
		pair := &exec.Pair{}

		if f.Value != "" && !key.IsString() && key.Len() == 2 {
			key.Iterate(func(idx, count int, key, value *exec.Value) bool {
				switch idx {
				case 0:
					itemContext.Set(f.Key, key)
					pair.Key = key
				case 1:
					itemContext.Set(f.Value, key)
					pair.Value = key
				}
				return true
			}, func() {})
		} else {
			itemContext.Set(f.Key, key)
			pair.Key = key
			if value != nil {
				itemContext.Set(f.Value, value)
				pair.Value = value
			}
		}

		if f.IfCondition != nil {
			condition := sub.Eval(f.IfCondition)
			if condition.IsError() {
				filterErr = valueError(condition)
				return false
			}
			if !condition.IsTrue() {
				return true
			}
		}
		items.Pairs = append(items.Pairs, pair)
		return true
	}); err != nil {
		return err
	}
	if filterErr != nil {
		return filterErr
	}

	length := len(items.Pairs)
	if length == 0 {
		if f.EmptyWrapper != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return r.Inherit().ExecuteWrapper(f.EmptyWrapper)
		}
		return nil
	}

	depth := 1
	if parentLoop, ok := r.Environment.Context.Get("loop"); ok {
		switch p := parentLoop.(type) {
		case *loopInfos:
			depth = p.depth + 1
		case *loopCallable:
			depth = p.infos.depth + 1
		}
	}

	loop := &loopInfos{
		first:  true,
		index0: -1,
		length: length,
		depth:  depth,
		depth0: depth - 1,
	}

	var loopContextValue any = loop
	if f.Recursive {
		recurseFn := func(va *exec.VarArgs) *exec.Value {
			if len(va.Args) == 0 {
				return exec.AsValue("")
			}
			buf := new(bytes.Buffer)
			sub := r.Inherit()
			originalOutput := sub.Output
			sub.Output = buf
			if err := f.renderIterable(ctx, sub, va.Args[0]); err != nil {
				sub.Output = originalOutput
				return exec.AsValue(err)
			}
			sub.Output = originalOutput
			return exec.AsSafeValue(buf.String())
		}
		loop.recurseFn = func(items *exec.Value) *exec.Value {
			return recurseFn(&exec.VarArgs{Args: []*exec.Value{items}})
		}
		loopContextValue = recurseFn
	}

	for idx, pair := range items.Pairs {
		if err := ctx.Err(); err != nil {
			return err
		}
		sub := r.Inherit()
		itemContext := sub.Environment.Context

		itemContext.Set(f.Key, pair.Key)
		if pair.Value != nil {
			itemContext.Set(f.Value, pair.Value)
		}

		itemContext.Set("loop", loopContextValue)
		loop.index0 = idx
		loop.index = loop.index0 + 1
		if idx == 1 {
			loop.first = false
		}
		if idx+1 == length {
			loop.last = true
		}
		loop.revindex = length - idx
		loop.revindex0 = length - (idx + 1)

		if idx == 0 {
			loop.PrevItem = exec.AsValue(nil)
		} else if previous := items.Pairs[idx-1]; previous.Value != nil {
			loop.PrevItem = exec.AsValue([2]*exec.Value{previous.Key, previous.Value})
		} else {
			loop.PrevItem = previous.Key
		}

		if idx == length-1 {
			loop.NextItem = exec.AsValue(nil)
		} else if next := items.Pairs[idx+1]; next.Value != nil {
			loop.NextItem = exec.AsValue([2]*exec.Value{next.Key, next.Value})
		} else {
			loop.NextItem = next.Key
		}

		if err := sub.ExecuteWrapper(f.BodyWrapper); err != nil {
			if isLoopControlError[*controlStructures.LoopBreakError](err) {
				break
			}
			if isLoopControlError[*controlStructures.LoopContinueError](err) {
				continue
			}
			return err
		}
	}

	return nil
}

// loopInfos exposes Jinja2's loop variable inside a {% for %} body.
type loopInfos struct {
	index     int
	index0    int
	length    int
	revindex  int
	revindex0 int
	first     bool
	last      bool
	depth     int
	depth0    int
	PrevItem  *exec.Value
	NextItem  *exec.Value
	lastValue *exec.Value

	recurseFn func(items *exec.Value) *exec.Value
}

func (li *loopInfos) GetAttribute(name string) (*exec.Value, bool) {
	switch name {
	case "index":
		return exec.AsValue(li.index), true
	case "index0":
		return exec.AsValue(li.index0), true
	case "revindex":
		return exec.AsValue(li.revindex), true
	case "revindex0":
		return exec.AsValue(li.revindex0), true
	case "first":
		return exec.AsValue(li.first), true
	case "last":
		return exec.AsValue(li.last), true
	case "length":
		return exec.AsValue(li.length), true
	case "depth":
		return exec.AsValue(li.depth), true
	case "depth0":
		return exec.AsValue(li.depth0), true
	case "previtem", "PrevItem":
		return li.PrevItem, true
	case "nextitem", "NextItem":
		return li.NextItem, true
	case "cycle", "Cycle":
		fn := func(va *exec.VarArgs) *exec.Value {
			if len(va.Args) == 0 {
				return exec.AsValue("")
			}
			idx := li.index0 % len(va.Args)
			if idx < 0 {
				idx += len(va.Args)
			}
			return va.Args[idx]
		}
		return exec.AsValue(fn), true
	case "changed", "Changed":
		fn := func(va *exec.VarArgs) *exec.Value {
			var current *exec.Value
			if len(va.Args) == 1 {
				current = va.Args[0]
			} else {
				items := make([]any, 0, len(va.Args))
				for _, a := range va.Args {
					items = append(items, a.Interface())
				}
				current = exec.AsValue(items)
			}
			same := li.lastValue != nil && current.EqualValueTo(li.lastValue)
			li.lastValue = current
			return exec.AsValue(!same)
		}
		return exec.AsValue(fn), true
	}
	return exec.AsValue(nil), false
}

type loopCallable struct {
	infos *loopInfos
}

func (lc *loopCallable) GetAttribute(name string) (*exec.Value, bool) {
	return lc.infos.GetAttribute(name)
}

func (lc *loopCallable) Call(va *exec.VarArgs) *exec.Value {
	if lc.infos.recurseFn == nil || len(va.Args) == 0 {
		return exec.AsValue("")
	}
	return lc.infos.recurseFn(va.Args[0])
}

// isLoopControlError reports whether err unwraps to a loop-control sentinel.
func isLoopControlError[T error](err error) bool {
	for e := err; e != nil; {
		if _, ok := e.(T); ok {
			return true
		}
		switch unwrapped := e.(type) {
		case interface{ Unwrap() error }:
			e = unwrapped.Unwrap()
		case interface{ Cause() error }:
			e = unwrapped.Cause()
		default:
			return false
		}
	}
	return false
}
