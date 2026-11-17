package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// twoWorktrees makes a repository with a commit history and a linked
// worktree on branch side, created outside GitOwl.
func twoWorktrees(t *testing.T, g *gittest.Env) (main, linked string) {
	t.Helper()
	main = g.Repo()
	for i := range 40 {
		gittest.Write(t, filepath.Join(main, "h.txt"), strings.Repeat("x", i+1)+"\n")
		g.Commit(main, "Commit "+itoa(i))
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	linked = filepath.Join(dir, "linked")
	g.Git(main, "worktree", "add", "-q", "-b", "side", linked)
	return main, linked
}

func TestFailedOpenKeepsTheWorktree(t *testing.T) {
	h := newHarness(t, 1280, 800)
	main, linked := twoWorktrees(t, h.Env)
	h.open(main)
	h.u.cm.message = "Keep me"
	h.tt.Frame()
	os.RemoveAll(linked) // gone behind GitOwl's back
	h.u.OpenPath(linked)
	h.tt.Frame()
	if h.u.repo.Root() != main || h.u.cm.message != "Keep me" || h.u.notice == nil {
		t.Fatalf("after failed open: %s %q %+v", h.u.repo.Root(), h.u.cm.message, h.u.notice)
	}
	h.want("That location does not exist.")
}

func TestLateResultsStayWithTheirOpening(t *testing.T) {
	g := gittest.New(t)
	main, linked := twoWorktrees(t, g)
	gittest.Write(t, filepath.Join(main, "m.txt"), "m\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(main)
	q.drain()
	u.selectRow(1)
	// A→B→A with main's reads still in flight: none of them apply to the
	// new opening of main or to linked.
	u.Refresh()
	u.toggleStaged(u.snap.Untracked[0])
	u.OpenPath(linked)
	q.runLast()
	u.OpenPath(main)
	q.runLast()
	reopened := u.repo
	q.drain()
	if u.repo != reopened || u.repo.Root() != main {
		t.Fatalf("showing %s", u.repo.Root())
	}
	if u.wt.busy != nil || u.loading != 0 {
		t.Fatalf("late results applied: busy %v loading %d", u.wt.busy, u.loading)
	}
	// The stage of the earlier opening did not run: its handle was closed.
	if out := g.Git(main, "status", "--porcelain"); out != "?? m.txt\n" {
		t.Fatalf("status %q", out)
	}
}

func TestSessionRestoreWaitsForHistory(t *testing.T) {
	g := gittest.New(t)
	main, linked := twoWorktrees(t, g)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 500)
	u.OpenPath(main)
	q.drain()
	u.middleTab = 1
	tt.Frame()
	u.selectCommit(30)
	u.hist.list.ScrollTo(25, ui.Start)
	q.drain()
	tt.Frame()
	hash := u.hist.selected
	top, _ := u.hist.list.Visible()
	u.OpenPath(linked)
	q.drain()
	u.OpenPath(main)
	q.runLast() // main opens; its history is still being read
	if u.hist.loaded || u.hist.selected != hash {
		t.Fatalf("restore before history: loaded %v selected %q", u.hist.loaded, u.hist.selected)
	}
	q.drain()
	tt.Frame()
	if u.hist.selected != hash || u.hist.cursor != 30 || u.hist.detail == nil || u.middleTab != 1 {
		t.Fatalf("after history: %q cursor %d detail %v", u.hist.selected, u.hist.cursor, u.hist.detail != nil)
	}
	if got, _ := u.hist.list.Visible(); got != top {
		t.Fatalf("history scroll %d, was %d", got, top)
	}
}

func TestCreatedWorktreeThatFailsToOpen(t *testing.T) {
	g := gittest.New(t)
	main, _ := twoWorktrees(t, g)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(main)
	q.drain()
	u.openCreateWorktree()
	u.trees.newBranch = "made"
	u.suggestPath()
	path := u.trees.path
	u.createWorktree()
	q.run(0) // created; the open is queued
	// The new directory goes away before it opens.
	os.RemoveAll(path)
	q.drain()
	tt.Frame()
	if u.repo.Root() != main || u.createdWorktree != path {
		t.Fatalf("root %s created %q", u.repo.Root(), u.createdWorktree)
	}
	if u.notice == nil {
		t.Fatalf("no notice after a failed open: texts %q", tt.Texts())
	}
	// GitOwl removed nothing: the branch stays.
	if out := g.Git(main, "branch", "--list", "made"); out == "" {
		t.Fatal("branch removed")
	}
}
