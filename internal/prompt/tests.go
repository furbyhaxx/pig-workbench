package prompt

import (
	"path/filepath"
	"regexp"
	"sync"

	"github.com/nikolalohinski/gonja/v2/exec"
)

func testFamily(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	want := params.First().String()
	switch model := in.Interface().(type) {
	case Model:
		return model.Family == want, nil
	default:
		return false, nil
	}
}

func testGlob(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	ok, err := filepath.Match(params.First().String(), in.String())
	if err != nil {
		return false, err
	}
	return ok, nil
}

var regexCache sync.Map

func testRegex(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	pattern := params.First().String()
	compiled, ok := regexCache.Load(pattern)
	if !ok {
		value, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		compiled = value
		regexCache.Store(pattern, value)
	}
	return compiled.(*regexp.Regexp).MatchString(in.String()), nil
}
