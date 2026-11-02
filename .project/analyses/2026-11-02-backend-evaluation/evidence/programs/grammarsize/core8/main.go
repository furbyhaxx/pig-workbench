package main

import (
	"fmt"
	"os"
	ts "github.com/odvcencio/gotreesitter"
	l_go "github.com/odvcencio/gotreesitter/grammars/go"
	l_rust "github.com/odvcencio/gotreesitter/grammars/rust"
	l_python "github.com/odvcencio/gotreesitter/grammars/python"
	l_typescript "github.com/odvcencio/gotreesitter/grammars/typescript"
	l_tsx "github.com/odvcencio/gotreesitter/grammars/tsx"
	l_javascript "github.com/odvcencio/gotreesitter/grammars/javascript"
	l_bash "github.com/odvcencio/gotreesitter/grammars/bash"
	l_json "github.com/odvcencio/gotreesitter/grammars/json"
)

func main() {
	langs := map[string]*ts.Language{
		"go": l_go.Language(),
		"rust": l_rust.Language(),
		"python": l_python.Language(),
		"typescript": l_typescript.Language(),
		"tsx": l_tsx.Language(),
		"javascript": l_javascript.Language(),
		"bash": l_bash.Language(),
		"json": l_json.Language(),
	}
	src, _ := os.ReadFile(os.Args[2])
	tree, err := ts.NewParser(langs[os.Args[1]]).Parse(src)
	if err != nil { fmt.Println("ERR", err); os.Exit(1) }
	fmt.Println(os.Args[1], tree.RootNode().Type(langs[os.Args[1]]), tree.RootNode().ChildCount())
}
