package main

import (
	"io"
	"os"
)

type Archive interface {
	Add(string, os.FileMode, []byte)
	Name() string
	ParseMeta(PackageMeta) error
	WriteTo(io.Writer) error
}
