// Archive coverage harness: lists members and reads the first regular file of every
// fixture with each candidate backend, reporting member count, bytes read, time and errors.
package main

import (
	"bufio"
	"bytes"
	"os/exec"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"strings"
	"time"

	nxcab "github.com/NextronSystems/go-cab"
	abcab "github.com/abemedia/go-cabinet"
	"github.com/blakesmith/ar"
	egar "github.com/erikgeiser/ar"
	cgcpio "github.com/cavaliergopher/cpio"
	asar "github.com/depthbomb/go-asar"
	gfcab "github.com/go-filesystems/cab"
	gfcpio "github.com/go-filesystems/cpio"
	gfrpm "github.com/go-filesystems/rpm"
	filesystem "github.com/go-filesystems/interface"
	gfiso "github.com/go-filesystems/iso9660"
	"github.com/kdomanski/iso9660"
	"github.com/koron-go/lha"
	"github.com/mholt/archives"
	rpmutils "github.com/sassoftware/go-rpmutils"
	urcpio "github.com/u-root/u-root/pkg/cpio"
)

type result struct {
	members int
	read    int64
	names   []string
}

type backend struct {
	name string
	fn   func(path string) (result, error)
}

func main() {
	byExt := map[string][]backend{
		"cpio": {{"go-filesystems/cpio", gfCpio}, {"cavaliergopher/cpio", cgCpio}, {"u-root/cpio", urCpio}},
		"cab":  {{"go-filesystems/cab", gfCab}, {"NextronSystems/go-cab", nxCab}, {"abemedia/go-cabinet", abCab}},
		"rpm":  {{"go-filesystems/rpm", gfRpm}, {"sassoftware/go-rpmutils", saRpm}},
		"lzh":  {{"koron-go/lha", koLha}},
		"lha":  {{"koron-go/lha", koLha}},
		"asar": {{"depthbomb/go-asar", dbAsar}},
		"deb":  {{"blakesmith/ar+archives", debAr}},
		"ar":   {{"blakesmith/ar", plainAr}, {"erikgeiser/ar", egAr}},
		"iso":  {{"kdomanski/iso9660", kdIso}, {"go-filesystems/iso9660", gfIso}},
		"7z":   {{"mholt/archives", mhArchive}},
		"zip":  {{"mholt/archives", mhArchive}},
		"xar":  {{"mholt/archives", mhIdentify}},
		"arj":  {{"mholt/archives", mhIdentify}},
		"rar":  {{"mholt/archives", mhArchive}, {"mholt/archives Rar{Name,FS}", mhRarVolumes}},
	}
	for _, p := range os.Args[1:] {
		ext := strings.TrimPrefix(filepath.Ext(p), ".")
		if strings.HasPrefix(filepath.Base(p), "n.txt.") {
			ext = "stream"
			byExt[ext] = []backend{{"mholt/archives", mhStream}}
		}
		for _, b := range append(byExt[ext], backend{"external 7zz", sevenZip}) {
			if only := os.Getenv("ONLY"); only != "" && only != b.name {
				continue
			}
			start := time.Now()
			r, err := safe(b.fn, p)
			d := time.Since(start).Round(time.Millisecond)
			status := "ok"
			if err != nil {
				status = "ERR " + oneLine(err.Error())
			}
			names := r.names
			if len(names) > 4 {
				names = append(names[:4], "…")
			}
			var ru syscall.Rusage
			syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
			fmt.Printf("%-52s %-24s members=%-4d read=%-7d %6s rss=%dMB %s %v\n", filepath.Base(p), b.name, r.members, r.read, d, ru.Maxrss/1024, status, names)
		}
	}
}

func safe(fn func(string) (result, error), p string) (r result, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("PANIC: %v", x)
		}
	}()
	return fn(p)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 90 {
		s = s[:90]
	}
	return s
}

func openRA(p string) (*os.File, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, 0, err
	}
	st, _ := f.Stat()
	return f, st.Size(), nil
}

func gfCpio(p string) (result, error) {
	f, n, err := openRA(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	recs, err := gfcpio.Records(f, n)
	var r result
	for _, rec := range recs {
		r.members++
		r.names = append(r.names, rec.Name)
		if r.read == 0 && rec.Mode.IsRegular() && rec.Size > 0 {
			b, _ := io.ReadAll(io.NewSectionReader(f, rec.Offset, rec.Size))
			r.read = int64(len(b))
		}
	}
	return r, err
}

func cgCpio(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	rd := cgcpio.NewReader(f)
	var r result
	for {
		h, err := rd.Next()
		if err == io.EOF {
			return r, nil
		}
		if err != nil {
			return r, err
		}
		r.members++
		r.names = append(r.names, h.Name)
		if r.read == 0 && h.Mode.IsRegular() && h.Size > 0 {
			r.read, _ = io.Copy(io.Discard, rd)
		}
	}
}

func urCpio(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	var r result
	for _, name := range []string{"newc", "odc", "bin"} {
		rr, ferr := tryURoot(f, name)
		if ferr == nil {
			return rr, nil
		}
		r, err = rr, ferr
		f.Seek(0, 0)
	}
	return r, err
}

func tryURoot(f *os.File, format string) (result, error) {
	rf, err := urcpio.Format(format)
	if err != nil {
		return result{}, err
	}
	recs, err := urcpio.ReadAllRecords(rf.Reader(f))
	var r result
	for _, rec := range recs {
		r.members++
		r.names = append(r.names, rec.Name)
		if r.read == 0 && rec.FileSize > 0 && rec.ReaderAt != nil {
			b, _ := io.ReadAll(io.NewSectionReader(rec.ReaderAt, 0, int64(rec.FileSize)))
			r.read = int64(len(b))
		}
	}
	return r, err
}

func walkGF(fs filesystem.Filesystem, dir string, r *result, read func(string) ([]byte, error)) error {
	ents, err := fs.ListDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.Name() == "." || e.Name() == ".." {
			continue
		}
		p := strings.TrimSuffix(dir, "/") + "/" + e.Name()
		r.members++
		r.names = append(r.names, p)
		if e.FileType() == 2 || e.FileType() == 4 { // directory in either DT_ or ext-style numbering
			if sub := walkGF(fs, p, r, read); sub == nil {
				continue
			}
		}
		if r.read == 0 {
			if b, err := read(p); err == nil {
				r.read = int64(len(b))
			}
		}
	}
	return nil
}

func gfCab(p string) (result, error) {
	f, n, err := openRA(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	fs, err := gfcab.Open(f, n)
	if err != nil {
		return result{}, err
	}
	var r result
	return r, func() error { err := walkGF(fs, "/", &r, fs.ReadFile); return err }()
}

func nxCab(p string) (result, error) {
	f, n, err := openRA(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	c, err := nxcab.Open(f, n)
	if err != nil {
		return result{}, err
	}
	var r result
	for _, file := range c.Files {
		r.members++
		r.names = append(r.names, file.Stat().Name())
		if r.read == 0 {
			rc, err := file.Open()
			if err != nil {
				return r, err
			}
			r.read, err = io.Copy(io.Discard, rc)
			rc.Close()
			if err != nil {
				return r, err
			}
		}
	}
	return r, nil
}

func abCab(p string) (result, error) {
	f, _, err := openRA(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	c, err := abcab.NewReader(f)
	if err != nil {
		return result{}, err
	}
	var r result
	for _, file := range c.Files {
		r.members++
		r.names = append(r.names, file.Name)
		if r.read == 0 {
			rc, err := file.Open()
			if err != nil {
				return r, err
			}
			r.read, err = io.Copy(io.Discard, rc)
			rc.Close()
			if err != nil {
				return r, err
			}
		}
	}
	return r, nil
}

func gfRpm(p string) (result, error) {
	f, n, err := openRA(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	fs, err := gfrpm.Open(f, n)
	if err != nil {
		return result{}, err
	}
	var r result
	return r, walkGF(fs, "/", &r, fs.ReadFile)
}

func saRpm(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	rpm, err := rpmutils.ReadRpm(f)
	if err != nil {
		return result{}, err
	}
	pr, err := rpm.PayloadReaderExtended()
	if err != nil {
		return result{}, err
	}
	var r result
	for {
		fi, err := pr.Next()
		if err == io.EOF {
			return r, nil
		}
		if err != nil {
			return r, err
		}
		r.members++
		r.names = append(r.names, fi.Name())
		if r.read == 0 && fi.Size() > 0 && !pr.IsLink() {
			r.read, _ = io.Copy(io.Discard, pr)
		}
	}
}

func koLha(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	rd := lha.NewReader(f)
	var r result
	for {
		h, err := rd.NextHeader()
		if err != nil {
			return r, err
		}
		if h == nil {
			return r, nil
		}
		r.members++
		r.names = append(r.names, h.Dir+h.Name+"("+h.Method+")")
		if h.Method == "-lhd-" {
			continue
		}
		var buf bytes.Buffer
		n, err := rd.Decode(&buf)
		if err != nil {
			return r, err
		}
		if r.read == 0 {
			r.read = int64(n)
		}
	}
}

func dbAsar(p string) (result, error) {
	a, err := asar.Open(p)
	if err != nil {
		return result{}, err
	}
	defer a.Close()
	var r result
	for _, e := range a.Entries() {
		r.members++
		r.names = append(r.names, e.Path)
	}
	sort.Strings(r.names)
	return r, nil
}

func debAr(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	rd := ar.NewReader(f)
	var r result
	for {
		h, err := rd.Next()
		if err == io.EOF {
			return r, nil
		}
		if err != nil {
			return r, err
		}
		r.members++
		r.names = append(r.names, h.Name)
		if strings.HasPrefix(h.Name, "data.tar") {
			body, _ := io.ReadAll(rd)
			format, stream, err := archives.Identify(context.Background(), h.Name, bytes.NewReader(body))
			if err != nil {
				return r, err
			}
			ex := format.(archives.Extractor)
			err = ex.Extract(context.Background(), stream, func(ctx context.Context, fi archives.FileInfo) error {
				r.names = append(r.names, "data:"+fi.NameInArchive)
				if r.read == 0 && fi.Mode().IsRegular() {
					rc, _ := fi.Open()
					r.read, _ = io.Copy(io.Discard, rc)
					rc.Close()
				}
				return nil
			})
			if err != nil {
				return r, err
			}
		}
	}
}

func plainAr(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	rd := ar.NewReader(f)
	var r result
	for {
		h, err := rd.Next()
		if err == io.EOF {
			return r, nil
		}
		if err != nil {
			return r, err
		}
		r.members++
		r.names = append(r.names, h.Name)
		if r.read == 0 {
			r.read, _ = io.Copy(io.Discard, rd)
		}
	}
}

func kdIso(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	img, err := iso9660.OpenImage(f)
	if err != nil {
		return result{}, err
	}
	root, err := img.RootDir()
	if err != nil {
		return result{}, err
	}
	var r result
	var walk func(d *iso9660.File, prefix string, depth int) error
	walk = func(d *iso9660.File, prefix string, depth int) error {
		if depth > 16 {
			return fmt.Errorf("depth cap at %s", prefix)
		}
		kids, err := d.GetChildren()
		if err != nil {
			return err
		}
		for _, k := range kids {
			r.members++
			r.names = append(r.names, prefix+k.Name())
			if k.IsDir() {
				if err := walk(k, prefix+k.Name()+"/", depth+1); err != nil {
					return err
				}
			} else if r.read == 0 {
				r.read, _ = io.Copy(io.Discard, k.Reader())
			}
		}
		return nil
	}
	return r, walk(root, "", 0)
}

func mhIdentify(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	format, _, err := archives.Identify(context.Background(), p, f)
	if err != nil {
		return result{}, err
	}
	return result{names: []string{fmt.Sprintf("%T", format)}}, nil
}

func mhArchive(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	format, stream, err := archives.Identify(context.Background(), p, f)
	if err != nil {
		return result{}, err
	}
	ex, ok := format.(archives.Extractor)
	if !ok {
		return result{}, fmt.Errorf("%T is not an extractor", format)
	}
	var r result
	err = ex.Extract(context.Background(), stream, func(ctx context.Context, fi archives.FileInfo) error {
		r.members++
		r.names = append(r.names, fi.NameInArchive)
		if r.read == 0 && fi.Mode().IsRegular() {
			rc, err := fi.Open()
			if err != nil {
				return err
			}
			r.read, err = io.Copy(io.Discard, rc)
			rc.Close()
			return err
		}
		return nil
	})
	return r, err
}

func mhStream(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	format, stream, err := archives.Identify(context.Background(), "", f)
	if err != nil {
		return result{}, err
	}
	dec, ok := format.(archives.Decompressor)
	if !ok {
		return result{}, fmt.Errorf("%T is not a decompressor", format)
	}
	rc, err := dec.OpenReader(stream)
	if err != nil {
		return result{}, err
	}
	defer rc.Close()
	n, err := io.Copy(io.Discard, rc)
	return result{members: 1, read: n, names: []string{fmt.Sprintf("%T", format)}}, err
}

func egAr(p string) (result, error) {
	f, err := os.Open(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	rd, err := egar.NewReader(f)
	if err != nil {
		return result{}, err
	}
	var r result
	for {
		h, err := rd.Next()
		if err == io.EOF {
			return r, nil
		}
		if err != nil {
			return r, err
		}
		r.members++
		r.names = append(r.names, h.Name)
		if r.read == 0 {
			r.read, _ = io.Copy(io.Discard, rd)
		}
	}
}

func mhRarVolumes(p string) (result, error) {
	rar := archives.Rar{Name: filepath.Base(p), FS: os.DirFS(filepath.Dir(p))}
	var r result
	err := rar.Extract(context.Background(), nil, func(ctx context.Context, fi archives.FileInfo) error {
		r.members++
		r.names = append(r.names, fi.NameInArchive)
		if fi.Mode().IsRegular() {
			rc, err := fi.Open()
			if err != nil {
				return err
			}
			n, err := io.Copy(io.Discard, rc)
			rc.Close()
			r.read += n
			return err
		}
		return nil
	})
	return r, err
}

// sevenZip lists with `7zz l -slt` and verifies every member decodes with `7zz t`.
func sevenZip(p string) (result, error) {
	bin := os.Getenv("SEVENZIP")
	if bin == "" {
		return result{}, fmt.Errorf("SEVENZIP unset")
	}
	out, err := exec.Command(bin, "l", "-slt", "-p-", p).CombinedOutput()
	var r result
	sc := bufio.NewScanner(bytes.NewReader(out))
	inList := false
	for sc.Scan() {
		line := sc.Text()
		if line == "----------" {
			inList = true
		}
		if inList && strings.HasPrefix(line, "Path = ") {
			r.members++
			r.names = append(r.names, strings.TrimPrefix(line, "Path = "))
		}
	}
	if err != nil {
		return r, fmt.Errorf("list: %v: %s", err, lastLine(out))
	}
	tout, err := exec.Command(bin, "t", "-p-", p).CombinedOutput()
	if err != nil {
		return r, fmt.Errorf("test: %v: %s", err, lastLine(tout))
	}
	r.read = -1
	return r, nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" && !strings.HasPrefix(l, "Sub items") {
			return l
		}
	}
	return ""
}

func gfIso(p string) (result, error) {
	f, n, err := openRA(p)
	if err != nil {
		return result{}, err
	}
	defer f.Close()
	fs, err := gfiso.Open(f, n)
	if err != nil {
		return result{}, err
	}
	var r result
	return r, walkGF(fs, "/", &r, fs.ReadFile)
}
