// Package launch opens GitOwl's targets in other applications: an editor,
// Terminal or Finder. Paths are passed as arguments of their own, never
// through a shell, so no file name can run a command.
package launch

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Tool is the kind of application a request opens.
type Tool uint8

const (
	// Editor opens Path in App, or in the system's default app for it when
	// App is empty.
	Editor Tool = iota
	// Terminal opens a Terminal window in the folder Path.
	Terminal
	// Finder shows Path in Finder: a folder's contents, or a file selected
	// in its folder.
	Finder
)

// Request is one thing to open.
type Request struct {
	Tool Tool
	// App is the .app bundle for Editor; empty uses the default app.
	App string
	// Path is an absolute path.
	Path string
}

// ErrNotAbsolute rejects a relative path, which open would read as an
// option or relative to GitOwl's directory.
var ErrNotAbsolute = errors.New("launch: path is not absolute")

// Shell is what MyGo provides to open paths with the desktop.
type Shell interface {
	OpenPath(path string) error
	ShowItemInFolder(path string)
}

// Args returns the /usr/bin/open arguments for r, or nil when the request
// goes to the Shell instead.
func Args(r Request) []string {
	switch {
	case r.Tool == Editor && r.App != "":
		return []string{"-a", r.App, r.Path}
	case r.Tool == Terminal:
		return []string{"-a", "Terminal", r.Path}
	}
	return nil
}

// Run opens r with /usr/bin/open or sh.
func Run(r Request, sh Shell) error {
	if !strings.HasPrefix(r.Path, "/") {
		return ErrNotAbsolute
	}
	if _, err := os.Stat(r.Path); err != nil {
		return err
	}
	if args := Args(r); args != nil {
		out, err := exec.Command("/usr/bin/open", args...).CombinedOutput()
		if err != nil {
			if msg := strings.TrimSpace(string(out)); msg != "" {
				return errors.New(msg)
			}
			return err
		}
		return nil
	}
	if r.Tool == Finder {
		if st, err := os.Stat(r.Path); err == nil && !st.IsDir() {
			sh.ShowItemInFolder(r.Path)
			return nil
		}
	}
	return sh.OpenPath(r.Path)
}
