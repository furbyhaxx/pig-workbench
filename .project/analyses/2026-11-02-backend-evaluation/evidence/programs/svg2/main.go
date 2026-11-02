package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers"
)

func main() {
	for _, p := range os.Args[1:] {
		t := time.Now()
		f, _ := os.Open(p)
		c, err := canvas.ParseSVG(f)
		f.Close()
		if err != nil {
			fmt.Println(p, "error:", err)
			continue
		}
		out := "/tmp/wb-backends/canvas-" + filepath.Base(p) + ".png"
		err = renderers.Write(out, c, canvas.DPMM(3))
		fmt.Println(p, err, time.Since(t).Round(time.Millisecond))
	}
}
