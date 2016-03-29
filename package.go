package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gogits/git-module"
	"github.com/mcuadros/go-version"
)

type Package struct {
	Manifest Manifest
	Path     string
	Repo     string
	Branch   string
	Version  string
	Formats  []string
}

func (pkg *Package) Verify() error {
	var err error

	if pkg.Path == "" {
		if pkg.Path, err = os.Getwd(); err != nil {
			return err
		}
	} else if !filepath.IsAbs(pkg.Path) {
		if pkg.Path, err = filepath.Abs(pkg.Path); err != nil {
			return err
		}
	}

	if pkg.Repo == "" {
		pkg.Repo = pkg.Path
	}
	if pkg.Branch == "" {
		pkg.Branch = "master"
	}

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
	return tags[0], nil
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
