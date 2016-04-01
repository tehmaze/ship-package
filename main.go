// Package main implements package writers for various distributions.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

var supportedFormats = map[string]bool{
	"deb": true,
	"rpm": true,
}

type Config struct {
	Package map[string]Package
	Meta    Meta
}

type Manifest map[string]json.RawMessage

type Target struct {
	Config bool
	Target string
	Mode   string
}

func showError(s string, err error) {
	syntax, ok := err.(*json.SyntaxError)
	if !ok {
		return
	}

	start, end := strings.LastIndex(s[:syntax.Offset], "\n")+1, len(s)
	if idx := strings.Index(s[start:], "\n"); idx >= 0 {
		end = start + idx
	}
	if start >= end {
		fmt.Printf("Error at end of file")
		return
	}
	line, pos := strings.Count(s[:start], "\n"), int(syntax.Offset)-start-1
	fmt.Printf("Error in line %d: %s \n", line, err)
	fmt.Printf("%s\n%s^", s[start:end], strings.Repeat(" ", pos))
}

func main() {
	configFile := flag.String("config", "ship.json", "Ship config")
	flag.Parse()

	f, err := os.Open(*configFile)
	if err != nil {
		fmt.Printf("error opening %q: %v\n", *configFile, err)
		os.Exit(1)
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		fmt.Printf("error reading %q: %v\n", *configFile, err)
		os.Exit(2)
	}

	c := new(Config)
	if err := json.Unmarshal(b, c); err != nil {
		fmt.Printf("error parsing %q: %v\n", *configFile, err)
		showError(string(b), err)
		os.Exit(2)
	}

	if len(c.Package) == 0 {
		fmt.Printf("error parsing %q: no packages defined\n", *configFile)
		os.Exit(2)
	}

	for name, pkg := range c.Package {
		if err := pkg.Verify(name, c.Meta); err != nil {
			fmt.Println("  error:", err)
			os.Exit(1)
		}
		fmt.Println("building", name, pkg.Version)
		if err := pkg.Build(); err != nil {
			fmt.Println("  error:", err)
			os.Exit(1)
		}
	}
}
