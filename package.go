package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gogits/git-module"
	"github.com/mcuadros/go-version"
)

type Package struct {
	Manifest Manifest
	Meta     PackageMeta
	Name     string
	Path     string
	Repo     string
	Branch   string
	Version  string
	Generate []string
	Formats  []string
	Ignore   []string
	ignore   []*regexp.Regexp
}

func (pkg *Package) Build() error {
	if pkg.Generate != nil {
		for _, run := range pkg.Generate {
			base, args := command(run)
			cmd := exec.Command(base, args...)
			out := new(bytes.Buffer)
			cmd.Stdout = out
			err := cmd.Run()
			if err != nil {
				return fmt.Errorf("error running %q: %v", run, err)
			}
		}
	}

	for _, format := range pkg.Formats {
		var (
			out Archive
			err error
		)
		switch format {
		case "deb":
			out = NewDeb(pkg.Name, pkg.Version)
		case "rpm":
			out, err = NewRPM(pkg.Name, pkg.Version)
		default:
			return fmt.Errorf("ship: unsupported format %q", format)
		}
		if err != nil {
			return err
		}
		if err = pkg.build(out); err != nil {
			return err
		}
	}

	return nil
}

func (pkg *Package) build(out Archive) error {
	if pkg.Manifest == nil || len(pkg.Manifest) == 0 {
		return errors.New("empty manifest")
	}

	if err := out.ParseMeta(pkg.Meta); err != nil {
		return err
	}

	var (
		f   *os.File
		fi  os.FileInfo
		err error
	)

	for pattern, rawTarget := range pkg.Manifest {
		target, err := pkg.parseTarget(rawTarget)
		if err != nil {
			return errors.New(pattern + ": " + err.Error())
		}

		source, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		if len(source) == 0 {
			return errors.New(pattern + ": did not match any files")
		}

		for _, src := range source {
			if fi, err = os.Stat(src); err != nil {
				return err
			}
			var dst = filepath.Join(target.Target, src)
			if err := pkg.add(out, dst, src, fi.Mode()); err != nil {
				return err
			}
		}
	}

	fmt.Printf("           %s\n", out.Name())
	if f, err = os.Create(out.Name()); err != nil {
		return err
	}

	if err = out.WriteTo(f); err != nil {
		return err
	}

	if err = f.Close(); err != nil {
		return err
	}

	return nil
}

func (pkg *Package) add(out Archive, dst, src string, mode os.FileMode) error {
	var (
		fi  os.FileInfo
		err error
	)
	if !filepath.IsAbs(src) {
		if src, err = filepath.Abs(src); err != nil {
			return err
		}
	}
	if pkg.ignored(src) {
		fmt.Printf("< ignore > %s\n", dst)
		return nil
	}
	if fi, err = os.Stat(src); err != nil {
		return err
	}
	if fi.IsDir() {
		return filepath.Walk(src, func(childSrc string, fi os.FileInfo, err2 error) error {
			if pkg.ignored(childSrc) || childSrc == src {
				return nil
			}
			var childDst = dst + childSrc[len(src):]
			return pkg.add(out, childDst, childSrc, fi.Mode())
		})
	}
	fmt.Printf("%s %s\n", mode.String(), dst)
	var (
		f *os.File
		b []byte
	)
	if f, err = os.Open(src); err != nil {
		return err
	}
	defer f.Close()
	if b, err = ioutil.ReadAll(f); err != nil {
		return err
	}
	out.Add(dst, mode, b)
	return nil
}

func (pkg *Package) ignored(name string) bool {
	for _, re := range pkg.ignore {
		if re.MatchString(name) {
			return true
		}
	}
	return false
}

func (pkg *Package) parseTarget(raw json.RawMessage) (*Target, error) {
	var target = new(Target)
	if err := json.Unmarshal(raw, target); err == nil {
		return target, nil
	}
	if err := json.Unmarshal(raw, &target.Target); err == nil {
		return target, nil
	}
	return nil, errors.New("can't unmarshal target")
}

func (pkg *Package) Verify(name string, meta Meta) error {
	var err error

	if pkg.Name == "" {
		pkg.Name = name
	}

	if pkg.Path == "" {
		if pkg.Path, err = os.Getwd(); err != nil {
			return err
		}
	} else if !filepath.IsAbs(pkg.Path) {
		if pkg.Path, err = filepath.Abs(pkg.Path); err != nil {
			return err
		}
	}
	if pkg.Meta.Author == "" {
		pkg.Meta.Author = meta.Author
	}
	if pkg.Meta.Email == "" {
		pkg.Meta.Email = meta.Email
	}
	if pkg.Meta.Homepage == "" {
		pkg.Meta.Homepage = meta.Homepage
	}

	if pkg.Repo == "" {
		pkg.Repo = pkg.Path
	}
	if pkg.Branch == "" {
		pkg.Branch = "master"
	}
	if pkg.Formats == nil || len(pkg.Formats) == 0 {
		pkg.Formats = make([]string, 0)
		for format := range supportedFormats {
			pkg.Formats = append(pkg.Formats, format)
		}
	}
	sort.Strings(pkg.Formats)

	switch pkg.Version {
	case "git":
		if pkg.Version, err = pkg.gitVersion(); err != nil {
			return err
		}
		break

	case "git-tag":
		if pkg.Version, err = pkg.gitTagVersion(); err != nil {
			return err
		}
		break

	case "":
		return errors.New("empty version and no version detection method specified")
	}

	if pkg.Ignore != nil && len(pkg.Ignore) > 0 {
		for _, glob := range pkg.Ignore {
			var (
				expr = "^"
				re   *regexp.Regexp
			)
			for _, char := range glob {
				switch char {
				case '*':
					expr += ".*"
				case '?':
					expr += "."
				case '.':
					expr += "\\."
				default:
					expr += string(char)
				}
			}
			expr += "$"
			if re, err = regexp.Compile(expr); err != nil {
				return fmt.Errorf("%s: invalid: %v\n", glob, err)
			}
			pkg.ignore = append(pkg.ignore, re)
		}
	}

	return nil
}

func (pkg *Package) gitTagVersion() (string, error) {
	repo, err := git.OpenRepository(pkg.Repo)
	if err != nil {
		return "", fmt.Errorf("can't get git repository at %s: %v", pkg.Repo, err)
	}
	tags, err := repo.GetTags()
	if err != nil {
		return "", fmt.Errorf("can't get git tags: %v", err)
	}
	if len(tags) == 0 {
		return "", fmt.Errorf("not git tags in repository %s", pkg.Repo)
	}
	version.Sort(tags)
	return tags[len(tags)-1], nil
}

func (pkg *Package) gitVersion() (string, error) {
	repo, err := git.OpenRepository(pkg.Repo)
	if err != nil {
		return "", fmt.Errorf("can't get git repository at %s: %v", pkg.Repo, err)
	}

	commit, err := repo.GetBranchCommit(pkg.Branch)
	if err != nil {
		return "", fmt.Errorf("can't get branch %s at %s: %v", pkg.Branch, pkg.Repo, err)
	}

	count, err := commit.CommitsCount()
	if err != nil {
		return "", fmt.Errorf("can't get commit count of %s at %s: %v", pkg.Branch, pkg.Repo, err)
	}

	return fmt.Sprintf("%d", count), nil
}

func command(run string) (string, []string) {
	var fields = strings.Fields(run)
	if len(fields) > 1 {
		return fields[0], fields[1:]
	}
	return fields[0], []string{}
}

type PackageMeta struct {
	Meta
	Summary     string
	Description string
	DebConflict []string `json:"deb-conflict"`
	DebRequires []string `json:"deb-requires"`
	RPMConflict []string `json:"rpm-conflict"`
	RPMRequires []string `json:"rpm-requires"`
}
