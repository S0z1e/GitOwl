package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// recentUI makes a UI with the given recent list, background work held in
// q so a test runs each read when it chooses, and a real workflow over the
// test's Git runner.
func recentUI(t *testing.T, g *gittest.Env, q *queued, recent ...string) *UI {
	t.Helper()
	p := prefs.Default()
	p.Recent = recent
	return New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go, Preferences: p})
}

// line reads one row's status line for root, in the light design set.
func line(u *UI, root string) recentLine {
	st, reading := u.recentStatusOf(root)
	return u.recentStatusLine(st, reading, kitDesignTokens(false))
}

// TestRecentStatusReadings covers the readable states a Recent row shows:
// clean, a file staged and unstaged counted once, an untracked file,
// detached HEAD, an unborn repository, and a plain non-repository folder.
func TestRecentStatusReadings(t *testing.T) {
	g := gittest.New(t)
	q := &queued{}

	clean := g.Repo()
	gittest.Write(t, filepath.Join(clean, "a.txt"), "a\n")
	g.Commit(clean, "Initial")

	both := g.Repo()
	gittest.Write(t, filepath.Join(both, "b.txt"), "one\n")
	g.Commit(both, "Initial")
	gittest.Write(t, filepath.Join(both, "b.txt"), "two\n")
	g.Git(both, "add", "b.txt")
	gittest.Write(t, filepath.Join(both, "b.txt"), "three\n")

	untracked := g.Repo()
	gittest.Write(t, filepath.Join(untracked, "c.txt"), "c\n")
	g.Commit(untracked, "Initial")
	gittest.Write(t, filepath.Join(untracked, "new.txt"), "n\n")

	detached := g.Repo()
	gittest.Write(t, filepath.Join(detached, "d.txt"), "d\n")
	g.Commit(detached, "Initial")
	head := g.Git(detached, "rev-parse", "HEAD")
	g.Git(detached, "checkout", "--detach", "HEAD")

	unborn := g.Repo()
	notRepo := t.TempDir()

	u := recentUI(t, g, q, clean, both, untracked, detached, unborn, notRepo)
	u.refreshRecentStatus()
	q.drain()

	if l := line(u, clean); !l.ok || l.count != "Clean" || l.branch != "main" {
		t.Fatalf("clean: %+v", l)
	}
	if l := line(u, both); !l.ok || l.count != "1 change" {
		t.Fatalf("staged+unstaged same file counted once: %+v", l)
	}
	if l := line(u, untracked); !l.ok || l.count != "1 change" {
		t.Fatalf("untracked: %+v", l)
	}
	if l := line(u, detached); !l.ok || l.branch != head[:7] {
		t.Fatalf("detached short %q: %+v", head[:7], l)
	}
	if l := line(u, unborn); !l.ok || l.branch != "No commits yet" {
		t.Fatalf("unborn: %+v", l)
	}
	if l := line(u, notRepo); l.ok || l.status != "Not a repository" {
		t.Fatalf("non-repository: %+v", l)
	}
}

// TestRecentStatusMissingAndReading shows "Not found" for a path that does
// not exist and a reading state before a reply lands, never Clean.
func TestRecentStatusMissingAndReading(t *testing.T) {
	g := gittest.New(t)
	q := &queued{}
	clean := g.Repo()
	gittest.Write(t, filepath.Join(clean, "a.txt"), "a\n")
	g.Commit(clean, "Initial")
	gone := filepath.Join(t.TempDir(), "gone")

	u := recentUI(t, g, q, clean, gone)
	u.refreshRecentStatus()
	// Before any reply, both rows read as reading, not Clean.
	for _, root := range []string{clean, gone} {
		if l := line(u, root); !l.reading || l.ok || l.count == "Clean" {
			t.Fatalf("reading state for %q: %+v", root, l)
		}
	}
	q.drain()
	if l := line(u, gone); l.ok || l.status != "Not found" {
		t.Fatalf("missing: %+v", l)
	}
	if l := line(u, clean); !l.ok {
		t.Fatalf("clean after drain: %+v", l)
	}
}

// TestRecentStatusStaleReplyDropped checks that a reply from before the
// list changed cannot revive a removed row or overwrite the current one.
func TestRecentStatusStaleReplyDropped(t *testing.T) {
	g := gittest.New(t)
	q := &queued{}
	a := g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	g.Commit(a, "Initial")
	b := g.Repo()
	gittest.Write(t, filepath.Join(b, "b.txt"), "b\n")
	g.Commit(b, "Initial")

	u := recentUI(t, g, q, a, b)
	u.refreshRecentStatus() // generation 1: reads for a and b are queued.

	// The list changes (a is removed) and the Landing re-reads before the
	// old reads apply: a new generation supersedes them.
	u.prefs.Recent = []string{b}
	u.refreshRecentStatus() // generation 2.
	q.drain()               // runs all queued reads, old and new.

	// a was removed: no status lingers for it.
	if _, ok := u.recent.status[a]; ok {
		t.Fatalf("stale reply revived removed row %q", a)
	}
	if l := line(u, b); !l.ok {
		t.Fatalf("current row b not read: %+v", l)
	}
}

// TestRecentStatusDoesNotBlockOpen opens a repository while its status
// reads are still queued: the open must not wait for them.
func TestRecentStatusDoesNotBlockOpen(t *testing.T) {
	g := gittest.New(t)
	q := &queued{}
	a := g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	g.Commit(a, "Initial")

	u := recentUI(t, g, q, a)
	u.refreshRecentStatus() // reads queued, not yet run.
	u.OpenPath(a)
	q.drain()
	if u.repo == nil || u.repo.Root() != a {
		t.Fatalf("open blocked by pending status reads: %+v", u.repo)
	}
}

// snapPair renders u at the design's 1360×820 in light and dark.
func snapPair(t *testing.T, u *UI, name string) {
	t.Helper()
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	snapshot(t, tt, name+"-light")
	tt.SetDark(true)
	tt.Frame()
	snapshot(t, tt, name+"-dark")
}
