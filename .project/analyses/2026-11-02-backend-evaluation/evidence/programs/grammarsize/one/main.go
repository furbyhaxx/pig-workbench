package main

import (
	"fmt"
	"os"
	ts "github.com/odvcencio/gotreesitter"
	l_go "github.com/odvcencio/gotreesitter/grammars/go"
)

func main() {
	langs := map[string]*ts.Language{
		"go": l_go.Language(),
	}
	src, _ := os.ReadFile(os.Args[2])
	tree, err := ts.NewParser(langs[os.Args[1]]).Parse(src)
	if err != nil { fmt.Println("ERR", err); os.Exit(1) }
	fmt.Println(os.Args[1], tree.RootNode().Type(langs[os.Args[1]]), tree.RootNode().ChildCount())
}
