package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// queued runs background work only when the test says so, in any order.
type queued struct{ jobs []func() }

func (q *queued) Go(fn func()) { q.jobs = append(q.jobs, fn) }

// drain runs every job, including those queued meanwhile.
func (q *queued) drain() {
	for len(q.jobs) > 0 {
		q.run(0)
	}
}

// runLast runs the latest job.
func (q *queued) runLast() { q.run(len(q.jobs) - 1) }

// run runs job i and removes it.
func (q *queued) run(i int) {
	fn := q.jobs[i]
	q.jobs = append(q.jobs[:i], q.jobs[i+1:]...)
	fn()
}

func TestRefreshShowsExternalChangeInWindow(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	h.open(dir)
	h.want("Current changes")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Untracked files", "a.txt", "b.txt")
}

func TestStaleResultsDoNotOverwriteAnotherRepository(t *testing.T) {
	g := gittest.New(t)
	a, b := g.Repo(), g.Repo()
	gittest.Write(t, filepath.Join(a, "a1.txt"), "a\n")
	gittest.Write(t, filepath.Join(b, "b1.txt"), "b\n")
	gittest.Write(t, filepath.Join(b, "b2.txt"), "b\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)

	u.OpenPath(a)
	q.drain()
	u.Refresh() // A slow read of a…
	gittest.Write(t, filepath.Join(a, "a2.txt"), "a\n")
	gittest.Write(t, filepath.Join(a, "a3.txt"), "a\n")
	u.OpenPath(b) // …then the user switches to b.
	q.runLast()
	q.drain() // The read of a finishes last.
	tt.Frame()
	if u.repo.Root() != b || len(u.snap.Untracked) != 2 || u.snap.Root != b {
		t.Fatalf("showing %s with %d untracked", u.snap.Root, len(u.snap.Untracked))
	}
	if !tt.HasText("Untracked files") || !tt.HasText(b) || tt.HasText(a) {
		t.Fatalf("texts %q", tt.Texts())
	}

	q.drain()
	// Two opens in flight: the earlier request finishing last loses.
	u.OpenPath(a)
	u.OpenPath(b)
	q.run(1)
	q.run(0)
	if u.repo.Root() != b || u.opening {
		t.Fatalf("latest open lost: %s opening=%v", u.repo.Root(), u.opening)
	}
}

func TestReadsDoNotBlockTheInterface(t *testing.T) {
	g := gittest.New(t)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(g.Repo())
	tt.Frame() // The frame builds while the open is still pending.
	if !u.opening || len(q.jobs) != 1 {
		t.Fatal("open did not run in the background")
	}
	q.drain()
	u.Refresh()
	tt.Frame()
	if !u.Refreshing() {
		t.Fatal("refresh state not shown")
	}
	q.drain()
	if u.Refreshing() {
		t.Fatal("refresh did not finish")
	}
}
