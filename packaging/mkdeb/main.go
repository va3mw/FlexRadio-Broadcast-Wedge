// Command mkdeb writes the broadcastwedge .deb without needing dpkg, so the
// package can be built on Windows.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type file struct {
	name string // path inside the archive
	mode int64
	data []byte // nil for a directory
}

func main() {
	arch := flag.String("arch", "arm64", "Debian architecture")
	version := flag.String("version", "", "package version")
	bin := flag.String("bin", "", "compiled broadcastwedged")
	out := flag.String("out", "", "output .deb")
	src := flag.String("src", "packaging/debian", "directory with control, maintainer scripts and unit")
	flag.Parse()
	if *version == "" || *bin == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	read := func(path string) []byte {
		b, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		return b
	}
	// Scripts with CRLF line endings fail on Linux with "not found".
	text := func(name string) []byte {
		return bytes.ReplaceAll(read(*src+"/"+name), []byte("\r\n"), []byte("\n"))
	}

	exe, unit := read(*bin), text("broadcastwedge.service")
	data := []file{
		{"./usr/", 0o755, nil},
		{"./usr/bin/", 0o755, nil},
		{"./usr/bin/broadcastwedged", 0o755, exe},
		{"./usr/lib/", 0o755, nil},
		{"./usr/lib/systemd/", 0o755, nil},
		{"./usr/lib/systemd/system/", 0o755, nil},
		{"./usr/lib/systemd/system/broadcastwedge.service", 0o644, unit},
	}
	ctl := strings.NewReplacer(
		"@VERSION@", *version,
		"@ARCH@", *arch,
		"@SIZE@", fmt.Sprint((len(exe)+len(unit)+1023)/1024),
	).Replace(string(text("control")))
	control := []file{
		{"./control", 0o644, []byte(ctl)},
		{"./postinst", 0o755, text("postinst")},
		{"./prerm", 0o755, text("prerm")},
		{"./postrm", 0o755, text("postrm")},
	}

	now := time.Now()
	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	member(&deb, "debian-binary", []byte("2.0\n"), now)
	member(&deb, "control.tar.gz", tarball(control, now), now)
	member(&deb, "data.tar.gz", tarball(data, now), now)
	if err := os.WriteFile(*out, deb.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *out, deb.Len())
}

func tarball(files []file, now time.Time) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		h := &tar.Header{Name: f.name, Mode: f.mode, ModTime: now, Uname: "root", Gname: "root", Format: tar.FormatGNU}
		if f.data == nil {
			h.Typeflag = tar.TypeDir
		} else {
			h.Typeflag, h.Size = tar.TypeReg, int64(len(f.data))
		}
		if err := tw.WriteHeader(h); err != nil {
			log.Fatal(err)
		}
		if _, err := tw.Write(f.data); err != nil {
			log.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		log.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

// member appends one file to an ar archive.
func member(w *bytes.Buffer, name string, data []byte, now time.Time) {
	fmt.Fprintf(w, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", name, now.Unix(), 0, 0, "100644", len(data))
	w.Write(data)
	if len(data)%2 == 1 {
		w.WriteByte('\n')
	}
}
