package main

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

func main() {
	src := []byte(`{"n": 12345678901234567890, "f": 1.10, "e": 1e400, "items":[{"a":1},{"a":2}]}`)
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	var v any
	dec.Decode(&v)
	for _, q := range []string{".n", ".f", ".e", ".items[] | select(.a > 1)", ".n + 1", "input", "env.HOME", "$__loc__"} {
		query, err := gojq.Parse(q)
		if err != nil {
			fmt.Printf("%-28s parse error: %v\n", q, err)
			continue
		}
		code, err := gojq.Compile(query, gojq.WithEnvironLoader(func() []string { return nil }))
		if err != nil {
			fmt.Printf("%-28s compile error: %v\n", q, err)
			continue
		}
		iter := code.Run(v)
		out := []string{}
		for {
			x, ok := iter.Next()
			if !ok {
				break
			}
			if e, isErr := x.(error); isErr {
				out = append(out, "ERR:"+e.Error())
				break
			}
			b, _ := gojq.Marshal(x)
			out = append(out, string(b))
		}
		fmt.Printf("%-28s %v\n", q, out)
	}
}
