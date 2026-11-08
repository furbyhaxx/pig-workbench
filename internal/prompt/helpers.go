package prompt

import (
	"errors"
	"fmt"
	"slices"

	"github.com/nikolalohinski/gonja/v2/exec"
)

type globalFunction = func(*exec.Evaluator, *exec.VarArgs) (any, error)

func singleName(params *exec.VarArgs, helper string) (string, error) {
	if len(params.Args) != 1 || !params.Args[0].IsString() || params.Args[0].String() == "" {
		return "", exec.ErrInvalidCall(fmt.Errorf("%s expects one non-empty name", helper))
	}
	return params.Args[0].String(), nil
}

// hasToolFunction reports whether the model is offered the named tool.
func hasToolFunction(input *exec.Context) globalFunction {
	return func(_ *exec.Evaluator, params *exec.VarArgs) (any, error) {
		name, err := singleName(params, "has_tool")
		if err != nil {
			return nil, err
		}
		value, ok := input.Get("tools")
		if !ok {
			return false, nil
		}
		tools, ok := value.(templateTools)
		return ok && slices.Contains(tools.Declared, name), nil
	}
}

// variantFunction names the family-specific template when it exists, else the shared one.
func variantFunction(input *exec.Context, loader TemplateLoader) globalFunction {
	return func(_ *exec.Evaluator, params *exec.VarArgs) (any, error) {
		name, err := singleName(params, "variant")
		if err != nil {
			return nil, err
		}
		available := loader.List()
		if value, ok := input.Get("model"); ok {
			if model, ok := value.(Model); ok && model.Family != "" {
				if candidate := "family/" + model.Family + "/" + name; slices.Contains(available, candidate) {
					return candidate, nil
				}
			}
		}
		if slices.Contains(available, name) {
			return name, nil
		}
		return nil, exec.ErrInvalidCall(errors.New("no template named " + name + " for variant"))
	}
}
