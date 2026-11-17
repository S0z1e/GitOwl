package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/launch"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// launcher records what the window asks to open, without opening it.
type launcher struct {
	got  []launch.Request
	fail error
}

func (l *launcher) launch(r launch.Request) error {
	l.got = append(l.got, r)
	if l.fail != nil {
		return l.fail
	}
	if _, err := os.Stat(r.Path); err != nil {
		return err
	}
	return nil
}

func toolHarness(t *testing.T, p prefs.Preferences) (*harness, *launcher) {
	g := gittest.New(t)
	l := &launcher{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() },
		Launch: l.launch, Preferences: p,
		ChooseApplication: func(string) (string, error) { return "/Applications/Zed.app", nil }})
	return &harness{Env: g, u: u, tt: ui.NewTester(window(u), 1280, 800)}, l
}

func TestOpenWorktreeInTools(t *testing.T) {
	h, l := toolHarness(t, prefs.Default())
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	h.click("Terminal")
	h.click("Finder")
	if len(l.got) != 2 || l.got[0] != (launch.Request{Tool: launch.Terminal, Path: dir}) || l.got[1] != (launch.Request{Tool: launch.Finder, Path: dir}) {
		t.Fatalf("requests %+v", l.got)
	}
	h.click("Editor")
	if len(l.got) != 2 || !h.u.set.open || h.u.set.state.Category != "general" {
		t.Fatalf("no editor: %+v open %v", l.got, h.u.set.open)
	}
	h.want("Editor", "None")
	snapshot(t, h.tt, "settings-editor")
	h.u.set.open = false
	h.u.prefs.Editor = prefs.Editor{Name: "Zed", Path: "/Applications/Zed.app"}
	h.u.lastActionAt = time.Time{}
	h.tt.Frame()
	h.click("Editor")
	if r := l.got[len(l.got)-1]; r != (launch.Request{Tool: launch.Editor, App: "/Applications/Zed.app", Path: dir}) {
		t.Fatalf("editor request %+v", r)
	}
}

func TestOpenFileWithOddName(t *testing.T) {
	p := prefs.Default()
	p.Editor = prefs.Editor{Name: "Code", Path: "/Applications/Code.app"}
	h, l := toolHarness(t, p)
	dir := h.Repo()
	name := "dir $(touch x)/`id` \"q\" 'a'.txt"
	gittest.Write(t, filepath.Join(dir, name), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, name), "b\n")
	h.open(dir)
	h.selectFile(workflow.Unstaged, name)
	h.menu("Actions for "+name, "Open in Code")
	h.menu("Actions for "+name, "Show in Finder")
	want := filepath.Join(dir, name)
	if len(l.got) != 2 || l.got[0].Path != want || l.got[0].App != "/Applications/Code.app" || l.got[1] != (launch.Request{Tool: launch.Finder, Path: want}) {
		t.Fatalf("requests %+v", l.got)
	}
	if _, err := os.Stat(filepath.Join(dir, "dir $(touch x)", "x")); err == nil {
		t.Fatal("a name ran a command")
	}
}

func TestDeletedFileCannotOpen(t *testing.T) {
	h, l := toolHarness(t, prefs.Default())
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "gone.txt"), "a\n")
	h.Commit(dir, "Initial")
	os.Remove(filepath.Join(dir, "gone.txt"))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "gone.txt")
	h.click("Actions for gone.txt")
	if err := h.tt.ChooseMenuItem("Open with Default App"); err == nil {
		t.Fatal("opened a deleted file")
	}
	h.tt.CloseMenu()
	h.menu("Actions for gone.txt", "Show Worktree Folder in Finder")
	if len(l.got) != 1 || l.got[0].Path != dir {
		t.Fatalf("requests %+v", l.got)
	}
}

func TestLaunchFailureKeepsWork(t *testing.T) {
	p := prefs.Default()
	p.Editor = prefs.Editor{Name: "Gone", Path: "/Applications/Gone.app"}
	h, l := toolHarness(t, p)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	h.open(dir)
	h.u.cm.message = "kept"
	l.fail = errors.New("Unable to find application named 'Gone'")
	h.click("Editor")
	h.want("The app could not open it. Check that the app is still installed, or choose another editor in Settings.", "Unable to find application named 'Gone'")
	if h.u.repo == nil || h.u.cm.message != "kept" {
		t.Fatal("failure lost the repository or the message")
	}
	// A file removed after the list was read.
	l.fail = nil
	os.Remove(filepath.Join(dir, "a.txt"))
	h.u.openInEditor("a.txt")
	h.tt.Frame()
	h.want("That file is not in the working tree any more.")
}

func TestHistoryFileOpensWorkingCopy(t *testing.T) {
	h, l := toolHarness(t, prefs.Default())
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	h.click("All commits")
	h.u.selectCommit(0)
	h.tt.Frame()
	if err := h.tt.RightClick("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Open with Default App"); err != nil {
		t.Fatalf("%v (%q)", err, h.tt.Menu())
	}
	h.tt.Frame()
	if len(l.got) != 1 || l.got[0].Path != filepath.Join(dir, "a.txt") {
		t.Fatalf("requests %+v", l.got)
	}
}
