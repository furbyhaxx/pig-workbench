package prompt

import (
	"fmt"
	"reflect"

	"github.com/nikolalohinski/gonja/v2/exec"
)

const attributeStateKey = "\x00prompt-attribute-state"

type attributeState struct {
	err error
}

// ErrUnknownAttribute identifies a missing attribute on a map or struct.
type ErrUnknownAttribute struct {
	Attribute string
	Kind      string
}

func (e *ErrUnknownAttribute) Error() string {
	return fmt.Sprintf("attribute %q not found on %s", e.Attribute, e.Kind)
}

func strictAttributeFilter(original exec.FilterFunction) exec.FilterFunction {
	return func(e *exec.Evaluator, in *exec.Value, args *exec.VarArgs) *exec.Value {
		result := original(e, in, args)
		if result.IsError() {
			return result
		}
		name := args.Args[0].String()
		if in.IsDict() {
			if value, found := in.GetItem(name); found {
				return value
			}
			return unknownAttribute(e, name, "map")
		}
		value := in.Val
		for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
			if value.IsNil() {
				return result
			}
			value = value.Elem()
		}
		if value.IsValid() && value.Kind() == reflect.Struct {
			if _, found := in.GetAttribute(name); !found {
				return unknownAttribute(e, name, "struct")
			}
		}
		return result
	}
}

func unknownAttribute(e *exec.Evaluator, name, kind string) *exec.Value {
	err := &ErrUnknownAttribute{Attribute: name, Kind: kind}
	if value, found := e.Environment.Context.Get(attributeStateKey); found {
		if state, ok := value.(*attributeState); ok && state.err == nil {
			state.err = err
		}
	}
	return exec.AsValue(err)
}
