package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func TestStageAllAndUnstageAllFromTheWindow(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "two\n")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "new\n")
	nested := filepath.Join(dir, "nested")
	gittest.Write(t, filepath.Join(nested, "x.txt"), "x\n")
	h.Git(nested, "init", "-q")
	h.open(dir)
	h.selectFile(workflow.Unstaged, "a.txt")

	h.click("Stage All")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\nnew.txt\n" {
		t.Fatalf("index %q", got)
	}
	// The nested repository is left alone, and the window says so.
	h.want("Staged changes", "1 change was not staged: conflicts and nested repositories are left for Git.")
	if s := h.u.wt.selected; s == nil || s.area != workflow.Staged || s.path != "a.txt" {
		t.Fatalf("selection %+v", s)
	}
	snapshot(t, h.tt, "stage-all")

	h.click("Unstage All")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index after unstage all %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "new.txt")); string(data) != "new\n" {
		t.Fatal("unstage all changed a file")
	}
	h.want("Unstaged changes", "a.txt")
	if s := h.u.wt.selected; s == nil || s.area != workflow.Unstaged {
		t.Fatalf("selection after unstage all %+v", s)
	}
	if out := h.Git(dir, "rev-list", "--count", "HEAD"); out != "1\n" {
		t.Fatal("committed")
	}
}

func TestBulkStagingBlocksOtherChanges(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	tt.Frame()
	if err := tt.Click("Stage All"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// While it runs: no second bulk action, no file staging, no commit.
	tt.Click("Stage All")
	tt.Click("Stage a.txt")
	u.cm.message = "m"
	u.commit()
	tt.Frame()
	if u.wt.bulk == "" || u.commitBlock() != "busy" || u.draftBlock() != "busy" {
		t.Fatalf("bulk %q commit %q draft %q jobs %d", u.wt.bulk, u.commitBlock(), u.draftBlock(), len(q.jobs))
	}
	q.drain()
	tt.Frame()
	if u.wt.bulk != "" || len(u.snap.Staged) != 2 {
		t.Fatalf("after: bulk %q staged %d", u.wt.bulk, len(u.snap.Staged))
	}
	// Nothing is left to stage: the action says why.
	if u.stageAllBlock() != "errorNothingToStage" {
		t.Fatal(u.stageAllBlock())
	}
}

func TestBulkResultAfterSwitchIsDropped(t *testing.T) {
	g := gittest.New(t)
	a, b := g.Repo(), g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(b, "b.txt"), "b\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain()
	u.bulkStage(false)
	u.OpenPath(b)
	q.run(1)
	q.run(0) // a's Stage All finishes after b opened: a is closed.
	if u.repo.Root() != b || len(u.snap.Staged) != 0 || u.wt.bulk != "" {
		t.Fatalf("a's result reached b: %+v bulk %q", u.snap, u.wt.bulk)
	}
	if out := g.Git(a, "diff", "--cached", "--name-only"); out != "" {
		t.Fatalf("closed repository changed: %q", out)
	}
}
