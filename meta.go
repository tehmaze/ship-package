package main

import (
	"os"
	"os/user"
)

type Meta struct {
	Author   string
	Email    string
	Homepage string
}

func (m *Meta) Verify() error {
	var err error

	if m.Author == "" {
		var u *user.User
		if u, err = user.Current(); err != nil {
			return err
		}
		m.Author = u.Name
	}
	if m.Email == "" {
		var host string
		if host, err = os.Hostname(); err != nil {
			return err
		}
		m.Email = os.Getenv("USER") + "@" + host
	}

	return nil
}
