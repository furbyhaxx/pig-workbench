package main

import (
	"encoding/json"
	"os"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	src, _ := os.ReadFile(os.Args[2])
	entry := grammars.DetectLanguageByName(os.Args[1])
	lang := entry.Language()
	tree, err := ts.NewParser(lang).Parse(src)
	if err != nil {
		panic(err)
	}
	out := [][]any{}
	var walk func(n *ts.Node)
	walk = func(n *ts.Node) {
		if n.IsNamed() || n.IsError() {
			out = append(out, []any{n.Type(lang), n.StartPoint().Row, n.StartPoint().Column, n.EndPoint().Row, n.EndPoint().Column})
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(tree.RootNode())
	json.NewEncoder(os.Stdout).Encode(out)
}
