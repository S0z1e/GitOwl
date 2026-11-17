package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type fakeShell struct{ opened, shown []string }

func (f *fakeShell) OpenPath(p string) error   { f.opened = append(f.opened, p); return nil }
func (f *fakeShell) ShowItemInFolder(p string) { f.shown = append(f.shown, p) }

// Paths that a shell would read as commands stay one argument each.
func TestArgsKeepPathWhole(t *testing.T) {
	for _, p := range []string{"/tmp/a b", "/tmp/$(touch x)", "/tmp/`id`", "/tmp/q\"uote's", "/tmp/new\nline", "/tmp/-a"} {
		args := Args(Request{Tool: Editor, App: "/Applications/Visual Studio Code.app", Path: p})
		if !slices.Equal(args, []string{"-a", "/Applications/Visual Studio Code.app", p}) {
			t.Fatalf("editor args %q", args)
		}
		if args := Args(Request{Tool: Terminal, Path: p}); !slices.Equal(args, []string{"-a", "Terminal", p}) {
			t.Fatalf("terminal args %q", args)
		}
	}
	if Args(Request{Tool: Editor, Path: "/x"}) != nil || Args(Request{Tool: Finder, Path: "/x"}) != nil {
		t.Fatal("default app and Finder go to the shell")
	}
}

func TestRunUsesShellAndChecksPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f $(x).txt")
	os.WriteFile(file, nil, 0o644)
	sh := &fakeShell{}
	if err := Run(Request{Tool: Finder, Path: file}, sh); err != nil || !slices.Equal(sh.shown, []string{file}) {
		t.Fatalf("finder file: %v %q", err, sh.shown)
	}
	if err := Run(Request{Tool: Finder, Path: dir}, sh); err != nil || !slices.Equal(sh.opened, []string{dir}) {
		t.Fatalf("finder folder: %v %q", err, sh.opened)
	}
	if err := Run(Request{Tool: Editor, Path: file}, sh); err != nil || len(sh.opened) != 2 {
		t.Fatalf("default app: %v %q", err, sh.opened)
	}
	if err := Run(Request{Tool: Editor, Path: "rel"}, sh); err != ErrNotAbsolute {
		t.Fatal(err)
	}
	if err := Run(Request{Tool: Editor, Path: filepath.Join(dir, "gone")}, sh); !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
}

// The real open command reports an application that does not exist,
// without starting anything.
func TestRunMissingApplication(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("/usr/bin/open"); err != nil {
		t.Skip("no /usr/bin/open")
	}
	err := Run(Request{Tool: Editor, App: "/Applications/No Such GitOwl Test App.app", Path: t.TempDir()}, &fakeShell{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no such gitowl test app") && !strings.Contains(err.Error(), "Unable") && !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing app: %v", err)
	}
}
