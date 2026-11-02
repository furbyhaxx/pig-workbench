package main

import (
	_ "embed"
	"fmt"

	_ "codeberg.org/readeck/go-readability/v2"
	_ "github.com/JohannesKaufmann/html-to-markdown/v2"
	_ "github.com/aymanbagabas/go-pty"
	_ "github.com/fsnotify/fsnotify"
	_ "github.com/google/pprof/profile"
	_ "github.com/itchyny/gojq"
	_ "github.com/kdomanski/iso9660"
	_ "github.com/klippa-app/go-pdfium/webassembly"
	_ "github.com/mholt/archives"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "golang.org/x/arch/arm64/arm64asm"
	_ "golang.org/x/arch/x86/x86asm"
	_ "golang.org/x/image/webp"
)

//go:embed svgrender.wasm
var svg []byte

func main() { fmt.Println(len(svg)) }
