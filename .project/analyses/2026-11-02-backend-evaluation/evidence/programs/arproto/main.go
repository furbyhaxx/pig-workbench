// Minimal GNU/BSD/SysV ar lister: proves the format needs no dependency.
package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	r := bufio.NewReader(f)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "!<arch>\n" {
		fmt.Println("not ar")
		os.Exit(1)
	}
	var gnuNames []byte
	for {
		h := make([]byte, 60)
		if _, err := io.ReadFull(r, h); err == io.EOF {
			return
		} else if err != nil {
			fmt.Println("ERR", err)
			os.Exit(1)
		}
		if string(h[58:60]) != "`\n" {
			fmt.Println("ERR bad header terminator")
			os.Exit(1)
		}
		name := strings.TrimRight(string(h[0:16]), " ")
		size, err := strconv.ParseInt(strings.TrimSpace(string(h[48:58])), 10, 64)
		if err != nil || size < 0 {
			fmt.Println("ERR size")
			os.Exit(1)
		}
		body := io.LimitReader(r, size)
		switch {
		case name == "//":
			gnuNames, _ = io.ReadAll(body)
			name = ""
		case name == "/" || name == "/SYM64/" || name == "__.SYMDEF" || name == "__.SYMDEF SORTED":
			name = ""
		case strings.HasPrefix(name, "#1/"):
			n, _ := strconv.Atoi(name[3:])
			b := make([]byte, n)
			io.ReadFull(body, b)
			name = string(bytes.TrimRight(b, "\x00"))
		case strings.HasPrefix(name, "/"):
			off, err := strconv.Atoi(name[1:])
			if err != nil || off >= len(gnuNames) {
				fmt.Println("ERR long-name offset")
				os.Exit(1)
			}
			end := bytes.Index(gnuNames[off:], []byte("/\n"))
			name = string(gnuNames[off : off+end])
		default:
			name = strings.TrimSuffix(name, "/")
		}
		n, _ := io.Copy(io.Discard, body)
		if name != "" {
			fmt.Printf("%s %d\n", name, n)
		}
		if size%2 == 1 {
			r.ReadByte()
		}
	}
}
