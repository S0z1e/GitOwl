package desktop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// window is the product shell: the custom-drawn chrome, not Onevra's
// plain header. Tests that still call window() see the same surfaces as
// chromeWindow.
func window(u *UI) func(*ui.Context) {
	return chromeWindow(u)
}

// harness runs a UI whose background work is synchronous, so tests see each
// action's result after the next frame.
type harness struct {
	*gittest.Env
	u  *UI
	tt *ui.Tester
}

func newHarness(t *testing.T, w, h int) *harness {
	g := gittest.New(t)
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	return &harness{Env: g, u: u, tt: ui.NewTester(window(u), w, h)}
}

// branchAction chooses item in the menu of branch's row.
func (h *harness) branchAction(branch, item string) {
	h.T.Helper()
	label := branch
	if h.u.snap.Branch == branch {
		label = h.u.t("currentBranchName", "branch", branch)
	}
	h.chooseBranchMenu(label, item)
}

// menu opens the menu of the button labeled button and chooses path in it.
func (h *harness) menu(button string, path ...string) {
	h.T.Helper()
	if err := h.tt.Click(button); err != nil {
		h.T.Fatalf("open menu %q: %v (texts %q)", button, err, h.tt.Texts())
	}
	if err := h.tt.ChooseMenuItem(path...); err != nil {
		h.T.Fatalf("menu %q: %v (items %q)", button, err, h.tt.Menu())
	}
	h.tt.Frame()
}

func (h *harness) open(path string) {
	h.T.Helper()
	h.u.OpenPath(path)
	h.tt.Frame()
	if h.u.repo == nil {
		h.T.Fatalf("open failed: %+v", h.u.notice)
	}
}

func (h *harness) want(texts ...string) {
	h.T.Helper()
	for _, s := range texts {
		if !h.tt.HasText(s) {
			h.T.Fatalf("missing %q in %q", s, h.tt.Texts())
		}
	}
}

// showNav opens the compact navigation drawer when Current changes is off screen.
func (h *harness) showNav() {
	h.T.Helper()
	if _, ok := h.tt.Find(sidebarShowLabel(h.u)); ok {
		h.click(sidebarShowLabel(h.u))
		h.tt.Advance(time.Second) // the drawer slides in before its rows show
	}
}

func TestEmptyWindowOffersOpenRepository(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}})
	tt := ui.NewTester(window(u), 1280, 800)
	for _, want := range []string{"GitOwl", "No repository open", "Open Repository…", "Settings"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	// One welcome, not three empty columns.
	for _, s := range []string{"Changes", "History", "Diff", "Branches", "Open a repository to review its changes."} {
		if tt.HasText(s) {
			t.Fatalf("empty window shows %q", s)
		}
	}
	snapshot(t, tt, "empty-light")
	tt.SetDark(true)
	snapshot(t, tt, "empty-dark")
}

func TestOpenShowsRepositorySummary(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "n\n")
	h.open(dir)
	// The title bar names the repository, its path and branch; the sidebar
	// does not repeat them or the change counts.
	h.want(filepath.Base(dir), dir, "main", "Branches")
	seen := 0
	for _, s := range h.tt.Texts() {
		if s == dir {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("the path shows %d times in %q", seen, h.tt.Texts())
	}
	// Empty staged is omitted; areas with files name the count in the list.
	h.want("Unstaged changes", "Untracked files", "a.txt", "new.txt")
	snapshot(t, h.tt, "opened")
}

func TestOpenUnbornAndInvalid(t *testing.T) {
	h := newHarness(t, 1280, 800)
	h.u.OpenPath(t.TempDir())
	h.tt.Frame()
	h.want("That folder is not inside a Git repository.")
	h.open(h.Repo())
	h.want("main", "No commits yet")
}
