package main

import (
	"fmt"
	"io"
	"os"

	"github.com/blakesmith/ar"
	"github.com/cavaliergopher/cpio"
	"github.com/kdomanski/iso9660"
)

func walkISO(f *iso9660.File, prefix string, out *[]string) {
	kids, _ := f.GetChildren()
	for _, k := range kids {
		*out = append(*out, prefix+k.Name())
		if k.IsDir() {
			walkISO(k, prefix+k.Name()+"/", out)
		}
	}
}

func main() {
	f, _ := os.Open("/tmp/wb-backends/arc/a.iso")
	img, err := iso9660.OpenImage(f)
	if err == nil {
		root, _ := img.RootDir()
		names := []string{}
		walkISO(root, "", &names)
		fmt.Println("iso", names)
	} else {
		fmt.Println("iso err", err)
	}
	c, _ := os.Open("/tmp/wb-backends/arc/a.cpio")
	r := cpio.NewReader(c)
	names := []string{}
	for {
		h, err := r.Next()
		if err != nil {
			if err != io.EOF {
				names = append(names, "ERR:"+err.Error())
			}
			break
		}
		names = append(names, fmt.Sprintf("%s(%v,%s)", h.Name, h.Mode&cpio.ModeType, h.Linkname))
	}
	fmt.Println("cpio", names)
	a, _ := os.Open("/tmp/wb-backends/arc/a.ar")
	ar1 := ar.NewReader(a)
	h, err := ar1.Next()
	fmt.Println("ar", h.Name, h.Size, err)
}
