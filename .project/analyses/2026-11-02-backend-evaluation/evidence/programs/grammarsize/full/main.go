package main

import (
	"fmt"

	"github.com/odvcencio/gotreesitter/grammars"
)

func main() { fmt.Println(len(grammars.AllLanguages())) }
