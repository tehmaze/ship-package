package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"
)

var (
	rpmMagic        = [4]byte{0xed, 0xab, 0xee, 0xdb}
	defaultRPMGroup = "Applications/Internet"

	// See rpmrc.in
	rpmArch = map[string]uint16{
		"386":   1,
		"amd64": 1,
		"arm":   12,
	}

	// See rpmrc.in
	rpmOS = map[string]uint16{
		"linux":   1,
		"freebsd": 8,
		"darwin":  21,
	}
)

const (
	binaryRPM = 0x0000
	sourceRPM = 0x0001
)

type RPM struct {
	Package     string
	Version     string
	Group       string
	Arch        string
	Conflicts   []string
	Requires    []string
	URL         string
	Vendor      string
	Summary     string
	Description string
	tree        tree
	header      *RPMHeader
}

func NewRPM(name, version string) (*RPM, error) {
	r := &RPM{
		Package:   name,
		Version:   version,
		Conflicts: make([]string, 0),
		Requires:  make([]string, 0),
		Group:     defaultRPMGroup,
		tree:      make(tree),
	}

	switch runtime.GOARCH {
	case "386":
		r.Arch = "i386"
	case "amd64":
		r.Arch = "x86_64"
	default:
		return nil, fmt.Errorf("rpm: unsupported architecture %q", runtime.GOARCH)
	}

	var err error
	if r.header, err = newRPMHeader(r.Name(), runtime.GOARCH, runtime.GOOS); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RPM) Add(name string, mode os.FileMode, data []byte) {
	r.tree[name] = leaf{name: name, mode: mode, data: data}
}

func (r *RPM) Name() string {
	return fmt.Sprintf("%s-%s.%s.rpm", r.Package, r.Version, r.Arch)
}

func (r *RPM) ParseMeta(meta PackageMeta) error {
	r.Vendor = meta.Author
	r.URL = meta.Homepage
	r.Summary = meta.Summary
	r.Description = meta.Description
	return nil
}

func (r *RPM) WriteTo(w io.Writer) error {
	if err := r.header.WriteTo(w); err != nil {
		return fmt.Errorf("rpm: error writing header: %v", err)
	}

	return nil
}

type RPMHeader struct {
	Magic         [4]byte
	Major, Minor  byte
	Type          uint16
	Arch          uint16
	Name          [66]byte
	OS            uint16
	SignatureType uint16
	Reserved      [16]byte
}

func newRPMHeader(name, arch, os string) (*RPMHeader, error) {
	h := &RPMHeader{
		Major: 3,
		Minor: 0,
		Type:  binaryRPM,
	}

	copy(h.Magic[:], rpmMagic[:])
	copy(h.Name[:65], []byte(name))

	var ok bool
	if h.Arch, ok = rpmArch[arch]; !ok {
		return nil, fmt.Errorf("rpm: unsupported architecture %s", arch)
	}
	if h.OS, ok = rpmOS[os]; !ok {
		return nil, fmt.Errorf("rpm: unsupported operating system %s", os)
	}

	return h, nil
}

func (h *RPMHeader) WriteTo(w io.Writer) error {
	buf := new(bytes.Buffer)
	buf.Write(h.Magic[:])
	buf.Write([]byte{h.Major, h.Minor})
	buf.Write([]byte{uint8(h.Type >> 8), uint8(h.Type)})
	buf.Write([]byte{uint8(h.Arch >> 8), uint8(h.Arch)})
	buf.Write(h.Name[:])
	buf.Write([]byte{uint8(h.OS >> 8), uint8(h.OS)})
	buf.Write([]byte{uint8(h.SignatureType >> 8), uint8(h.SignatureType)})
	buf.Write(h.Reserved[:])
	_, err := w.Write(buf.Bytes())
	return err
}

var _ Archive = (*RPM)(nil)
