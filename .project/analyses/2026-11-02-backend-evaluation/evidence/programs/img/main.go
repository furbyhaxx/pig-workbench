package main

import (
	"fmt"
	"image"
	"os"
	"time"

	_ "github.com/gen2brain/heic"
	_ "golang.org/x/image/webp"
)

func main() {
	for _, p := range os.Args[1:] {
		f, _ := os.Open(p)
		t := time.Now()
		cfg, format, err := image.DecodeConfig(f)
		f.Seek(0, 0)
		img, _, err2 := image.Decode(f)
		f.Close()
		b := image.Rectangle{}
		if img != nil {
			b = img.Bounds()
		}
		fmt.Println(p, format, cfg.Width, cfg.Height, err, err2, b, time.Since(t).Round(time.Millisecond))
	}
}
