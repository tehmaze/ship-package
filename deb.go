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
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/blakesmith/ar"
	"github.com/kr/text"
)

var (
	defaultDebSection  = "utils"
	defaultDebPriority = "optional"
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
		Section:      defaultDebSection,
		Priority:     defaultDebPriority,
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

func (d *Deb) Name() string {
	return fmt.Sprintf("%s_%s_%s.deb", d.Package, d.Version, d.Architecture)
}

func (d *Deb) ParseMeta(meta PackageMeta) error {
	d.Maintainer = meta.Email
	d.Homepage = meta.Homepage
	d.Description = meta.Summary
	d.LongDescription = meta.Description
	return nil
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
		long)
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

	leafs := leafs{}
	for _, leaf := range d.tree {
		leafs = append(leafs, leaf)
	}
	sort.Sort(leafs)
	for _, leaf := range leafs {
		md5buf.WriteString(fmt.Sprintf("%x  %s\n", leaf.Checksum(digest), leaf.name))
		if err := addTarDir(now, out, path.Dir(leaf.name), dirs); err != nil {
			return nil, nil, 0, fmt.Errorf("can't write header of %s to data.tar.gz: %v", path.Dir(leaf.name), err)
		}
		header := tar.Header{
			Name:     leaf.name,
			Mode:     0644,
			ModTime:  now,
			Size:     int64(len(leaf.data)),
			Typeflag: tar.TypeReg,
		}
		if filepath.IsAbs(header.Name) {
			header.Name = "." + header.Name
		}
		if err := out.WriteHeader(&header); err != nil {
			return nil, nil, 0, fmt.Errorf("can't write header of %s to data.tar.gz: %v", leaf.name, err)
		}
		n, err := out.Write(leaf.data)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("can't write data of %s to data.tar.gz: %v", leaf.name, err)
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
		Name:     "./control",
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
		Name:     "./md5sums",
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

func addTarDir(now time.Time, w *tar.Writer, name string, dirs map[string]bool) error {
	if !dirs[name] {
		var (
			full   = name
			parent = path.Dir(name)
		)
		if name != "/" && name != parent {
			if !dirs[parent] {
				if err := addTarDir(now, w, parent, dirs); err != nil {
					return err
				}
			}
			if !strings.HasSuffix(full, "/") {
				full += "/"
			}
		}

		header := tar.Header{
			Name:     "." + full,
			Mode:     0755,
			ModTime:  now,
			Typeflag: tar.TypeDir,
		}
		if err := w.WriteHeader(&header); err != nil {
			return err
		}
		dirs[name] = true
	}
	return nil
}
