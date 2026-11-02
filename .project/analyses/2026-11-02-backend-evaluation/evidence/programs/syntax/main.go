package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func walk(n *ts.Node, f func(*ts.Node)) {
	f(n)
	for i := 0; i < n.ChildCount(); i++ {
		walk(n.Child(i), f)
	}
}

func main() {
	for _, arg := range os.Args[1:] {
		name, path, _ := strings.Cut(arg, ":")
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Println(name, "read error", err)
			continue
		}
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			fmt.Printf("%-10s NO GRAMMAR\n", name)
			continue
		}
		t0 := time.Now()
		lang := entry.Language()
		load := time.Since(t0)
		parser := ts.NewParser(lang)
		t1 := time.Now()
		tree, err := parser.Parse(src)
		parse := time.Since(t1)
		if err != nil {
			fmt.Printf("%-10s parse error %v\n", name, err)
			continue
		}
		root := tree.RootNode()
		errs, missing, nodes := 0, 0, 0
		firstErr := ""
		walk(root, func(n *ts.Node) {
			nodes++
			if n.IsError() {
				errs++
				if firstErr == "" {
					firstErr = fmt.Sprintf("line %d", n.StartPoint().Row+1)
				}
			}
			if n.IsMissing() {
				missing++
			}
		})
		// block anchors: lines whose first non-space column starts a named node below root spanning >1 line
		lines := strings.Split(string(src), "\n")
		anchors := map[uint32]string{}
		walk(root, func(n *ts.Node) {
			if n == root || !n.IsNamed() || n.EndPoint().Row == n.StartPoint().Row {
				return
			}
			row := n.StartPoint().Row
			if int(row) >= len(lines) {
				return
			}
			indent := uint32(len(lines[row]) - len(strings.TrimLeft(lines[row], " \t")))
			if n.StartPoint().Column != indent {
				return
			}
			if _, ok := anchors[row]; !ok {
				anchors[row] = fmt.Sprintf("%s[%d-%d]", n.Type(lang), row+1, n.EndPoint().Row+1)
			}
		})
		sample := []string{}
		for r := uint32(0); r < uint32(len(lines)) && len(sample) < 3; r++ {
			if a, ok := anchors[r]; ok {
				sample = append(sample, a)
			}
		}
		fmt.Printf("%-10s %7dB load=%-8s parse=%-8s nodes=%-6d errors=%d missing=%d first=%s anchors=%d %v\n", name, len(src), load.Round(time.Millisecond), parse.Round(time.Millisecond), nodes, errs, missing, firstErr, len(anchors), sample)
	}
}
