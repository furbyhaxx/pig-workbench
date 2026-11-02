package main

import (
	"fmt"

	ts "github.com/odvcencio/gotreesitter"
)

func main() { fmt.Println(ts.NewParser(nil) != nil) }
