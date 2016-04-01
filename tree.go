package main

import (
	"hash"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"golang.org/x/tools/godoc/vfs"
)

type tree map[string]leaf

func NewTree() vfs.FileSystem {
	return make(tree)
}

func (t tree) String() string { return "tree" }
func (t tree) Close() error   { return nil }

func filename(p string) string {
	return strings.TrimPrefix(p, "/")
}

func (t tree) Open(p string) (vfs.ReadSeekCloser, error) {
	l, ok := t[filename(p)]
	if !ok {
		return nil, os.ErrNotExist
	}
	return l, nil
}

func (t tree) Lstat(p string) (os.FileInfo, error) {
	l, ok := t[filename(p)]
	if !ok {
		leafs, _ := t.ReadDir(p)
		if len(leafs) > 0 {
			return info{name: p, mode: 0755, dir: true}, nil
		}
	}
	return l.Stat()
}

func (t tree) Stat(p string) (os.FileInfo, error) {
	return t.Lstat(p)
}

func slashdir(p string) string {
	var d = path.Dir(p)
	if d == "." {
		return "/"
	} else if strings.HasPrefix(p, "/") {
		return d
	}
	return "/" + d
}

func (t tree) ReadDir(p string) ([]os.FileInfo, error) {
	p = path.Clean(p)
	var (
		e = make([]string, 0)
		i = make(map[string]os.FileInfo)
	)
	for lp, l := range t {
		var (
			ld         = slashdir(lp)
			dir        bool
			base, last string
		)
		for {
			if ld == p {
				base = last
				if !dir {
					base = path.Base(lp)
				}
				if i[base] == nil {
					var fi os.FileInfo
					if dir {
						fi = info{name: base, mode: 0755, dir: true}
					} else {
						fi = l.stat()
					}
					e = append(e, base)
					i[base] = fi
				}
			} else if ld == "/" {
				break
			} else {
				dir = true
				last = path.Base(ld)
				ld = path.Dir(ld)
			}
		}
	}

	if len(e) == 0 {
		return nil, os.ErrNotExist
	}

	sort.Strings(e)
	var l []os.FileInfo
	for _, d := range e {
		l = append(l, i[d])
	}
	return l, nil
}

type leaf struct {
	io.ReadSeeker
	name string
	mode os.FileMode
	data []byte
}

func (l leaf) Close() error               { return nil }
func (l leaf) stat() os.FileInfo          { return info{name: l.name, mode: l.mode} }
func (l leaf) Stat() (os.FileInfo, error) { return l.stat(), nil }

func (l leaf) Checksum(h hash.Hash) []byte {
	h.Reset()
	h.Write(l.data)
	return h.Sum(nil)
}

type info struct {
	name string
	size int64
	mode os.FileMode
	dir  bool
}

func (i info) IsDir() bool        { return i.dir }
func (i info) ModTime() time.Time { return time.Time{} }
func (i info) Mode() os.FileMode  { return i.mode }
func (i info) Name() string       { return i.name }
func (i info) Size() int64        { return i.size }
func (i info) Sys() interface{}   { return nil }

type leafs []leaf

func (l leafs) Len() int {
	return len(l)
}

func (l leafs) Less(i, j int) bool {
	return l[i].name < l[j].name
}

func (l leafs) Swap(i, j int) {
	t := l[i]
	l[i] = l[j]
	l[j] = t
}
