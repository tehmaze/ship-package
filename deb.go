package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/blakesmith/ar"
	"github.com/kr/text"
)

var (
	DefaultDebSection  = "utils"
	DefaultDebPriority = "optional"
)

var control = `Package: %s
Version: %s
Architecture: %s
Maintainer: %s
Installed-Size: %d
Conflicts: %s
Depends: %s
Section: %s
Priority: %s
Homepage: %s
Description: %s
%s
`

type Deb struct {
	Package         string
	Version         string
	Section         string
	Priority        string
	Architecture    string
	Conflicts       []string
	Depends         []string
	Homepage        string
	Maintainer      string
	Description     string
	LongDescription string
	tree            tree
}

func NewDeb(name, version string) *Deb {
	d := &Deb{
		Package:      name,
		Version:      version,
		Conflicts:    make([]string, 0),
		Depends:      make([]string, 0),
		Section:      DefaultDebSection,
		Priority:     DefaultDebPriority,
		Architecture: runtime.GOARCH,
		tree:         make(tree),
	}
	if d.Architecture == "386" {
		d.Architecture = "i386"
	}
	return d
}

func (d *Deb) Add(name string, mode os.FileMode, data []byte) {
	d.tree[name] = leaf{name: name, mode: mode, data: data}
}

func (d *Deb) control(size int64) string {
	var (
		long = d.LongDescription
	)
	if long != "" {
		long = text.Indent(text.Wrap(long, 76), "  ")
	}
	return fmt.Sprintf(control,
		d.Package,
		d.Version,
		d.Architecture,
		d.Maintainer,
		size,
		strings.Join(d.Conflicts, ", "),
		strings.Join(d.Depends, ", "),
		d.Section,
		d.Priority,
		d.Homepage,
		d.Description,
		d.LongDescription)
}

func (d *Deb) WriteTo(out io.Writer) error {
	var (
		now = time.Now()
		deb = ar.NewWriter(out)
	)

	dataTarball, md5sums, size, err := d.createDataTarball(now)
	if err != nil {
		return err
	}
	controlTarball, err := d.createControlTarball(now, size, md5sums)
	if err != nil {
		return err
	}

	if err := deb.WriteGlobalHeader(); err != nil {
		return fmt.Errorf("can't write ar header to deb file: %v", err)
	}
	if err := addArFile(now, deb, "debian-binary", []byte("2.0\n")); err != nil {
		return fmt.Errorf("can't pack debian-binary: %v", err)
	}
	if err := addArFile(now, deb, "control.tar.gz", controlTarball); err != nil {
		return fmt.Errorf("can't add control.tar.gz to deb: %v", err)
	}
	if err := addArFile(now, deb, "data.tar.gz", dataTarball); err != nil {
		return fmt.Errorf("can't add data.tar.gz to deb: %v", err)
	}

	return nil
}

func (d *Deb) createDataTarball(now time.Time) ([]byte, []byte, int64, error) {
	var (
		size   int64
		buf    = new(bytes.Buffer)
		zip    = gzip.NewWriter(buf)
		out    = tar.NewWriter(zip)
		md5buf = new(bytes.Buffer)
		digest = md5.New()
		dirs   = make(map[string]bool)
	)

	for name, leaf := range d.tree {
		md5buf.WriteString(fmt.Sprintf("%x  %s\n", leaf.Checksum(digest), name))
		dir := path.Dir(name)
		if !dirs[dir] {
			header := tar.Header{
				Name:     dir,
				Mode:     0755,
				ModTime:  now,
				Typeflag: tar.TypeDir,
			}
			if err := out.WriteHeader(&header); err != nil {
				return nil, nil, 0, fmt.Errorf("can't write header of %s to data.tar.gz: %v", dir, err)
			}
			dirs[dir] = true
		}
		header := tar.Header{
			Name:     name,
			Mode:     0644,
			ModTime:  now,
			Typeflag: tar.TypeReg,
		}
		if err := out.WriteHeader(&header); err != nil {
			return nil, nil, 0, fmt.Errorf("can't write header of %s to data.tar.gz: %v", name, err)
		}
		n, err := out.Write(leaf.data)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("can't write data of %s to data.tar.gz: %v", name, err)
		}
		size += int64(n)
	}

	if err := out.Close(); err != nil {
		return nil, nil, 0, fmt.Errorf("can't close data.tar.gz: %v", err)
	}
	if err := zip.Close(); err != nil {
		return nil, nil, 0, fmt.Errorf("can't close data.tar.gz compressor: %v", err)
	}

	return buf.Bytes(), md5buf.Bytes(), size, nil
}

func (d *Deb) createControlTarball(now time.Time, size int64, md5sums []byte) ([]byte, error) {
	var (
		data = []byte(d.control(size / 1024))
		buf  = new(bytes.Buffer)
		zip  = gzip.NewWriter(buf)
		out  = tar.NewWriter(zip)
	)

	header := tar.Header{
		Name:     "",
		Size:     int64(len(data)),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := out.WriteHeader(&header); err != nil {
		return nil, fmt.Errorf("can't write header of control file to control.tar.gz: %v", err)
	}
	if _, err := out.Write(data); err != nil {
		return nil, fmt.Errorf("can't write control file to control.tar.gz: %v", err)
	}

	header = tar.Header{
		Name:     "md5sums",
		Size:     int64(len(md5sums)),
		Mode:     0644,
		ModTime:  now,
		Typeflag: tar.TypeReg,
	}
	if err := out.WriteHeader(&header); err != nil {
		return nil, fmt.Errorf("can't write header of md5sums file to control.tar.gz: %v", err)
	}
	if _, err := out.Write(md5sums); err != nil {
		return nil, fmt.Errorf("can't write md5sums file to control.tar.gz: %v", err)
	}

	if err := out.Close(); err != nil {
		return nil, fmt.Errorf("closing control.tar.gz: %v", err)
	}
	if err := zip.Close(); err != nil {
		return nil, fmt.Errorf("closing control.tar.gz: %v", err)
	}

	return buf.Bytes(), nil
}

func addArFile(now time.Time, w *ar.Writer, name string, body []byte) error {
	header := ar.Header{
		Name:    name,
		Size:    int64(len(body)),
		Mode:    0644,
		ModTime: now,
	}
	if err := w.WriteHeader(&header); err != nil {
		return fmt.Errorf("can't write ar header: %v", err)
	}
	_, err := w.Write(body)
	return err
}
