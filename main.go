// Package main implements package writers for various distributions.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
)

var supportedFormats = map[string]bool{
	"deb": true,
	"rpm": true,
}

type Config struct {
	Package map[string]Package
}

type Manifest map[string]File

type File struct {
	Config bool
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
		os.Exit(2)
	}

	if len(c.Package) == 0 {
		fmt.Printf("error parsing %q: no packages defined\n", *configFile)
		os.Exit(2)
	}

	for name, pkg := range c.Package {
		fmt.Println("building", name)
		if err := pkg.Verify(); err != nil {
			fmt.Println("  error:", err)
			os.Exit(1)
		}
	}
}
