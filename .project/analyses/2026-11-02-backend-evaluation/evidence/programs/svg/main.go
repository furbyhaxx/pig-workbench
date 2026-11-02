package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

func main() {
	for _, p := range os.Args[1:] {
		t := time.Now()
		icon, err := oksvg.ReadIcon(p, oksvg.WarnErrorMode)
		if err != nil {
			fmt.Println(p, "error:", err)
			continue
		}
		w, h := int(icon.ViewBox.W), int(icon.ViewBox.H)
		if w > 800 {
			h = h * 800 / w
			w = 800
		}
		icon.SetTarget(0, 0, float64(w), float64(h))
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		icon.Draw(rasterx.NewDasher(w, h, rasterx.NewScannerGV(w, h, img, img.Bounds())), 1)
		f, _ := os.Create("/tmp/wb-backends/oksvg-" + filepath.Base(p) + ".png")
		png.Encode(f, img)
		f.Close()
		fmt.Println(p, w, h, time.Since(t).Round(time.Millisecond))
	}
}
