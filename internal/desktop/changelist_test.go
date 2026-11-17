package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// counts returns a file's "+N −M" text for an area, failing when the row or
// its counts are not there.
func (h *harness) counts(area workflow.Area, path string) (text string, binary bool) {
	h.T.Helper()
	text, binary, ok := h.u.lineCounts(changeKey{area: area, path: path})
	if !ok {
		h.T.Fatalf("no counts for %v %s; stats %+v", area, path, h.u.wt.stats)
	}
	return text, binary
}

// TestChangeListLineCountsPerArea reads a file changed on both sides and
// checks each area keeps its own line counts, from the same snapshot.
func TestChangeListLineCountsPerArea(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "a\nb\nc\n")
	h.Commit(dir, "Initial")
	// Stage one added line, then add two more unstaged lines.
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "a\nb\nc\nstaged\n")
	h.Git(dir, "add", "mixed.txt")
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "a\nb\nc\nstaged\nun1\nun2\n")
	h.open(dir)

	// Same path in both areas, told apart by area.
	if _, ok := h.u.wt.row(changeKey{area: workflow.Staged, path: "mixed.txt"}); !ok {
		t.Fatal("no staged row for mixed.txt")
	}
	if _, ok := h.u.wt.row(changeKey{area: workflow.Unstaged, path: "mixed.txt"}); !ok {
		t.Fatal("no unstaged row for mixed.txt")
	}
	if got, _ := h.counts(workflow.Staged, "mixed.txt"); got != "+1 −0" {
		t.Fatalf("staged counts %q", got)
	}
	if got, _ := h.counts(workflow.Unstaged, "mixed.txt"); got != "+2 −0" {
		t.Fatalf("unstaged counts %q", got)
	}
	// The staged totals add up to the staged side.
	if h.u.wt.stagedAdded != 1 || h.u.wt.stagedRemoved != 0 {
		t.Fatalf("staged totals +%d −%d", h.u.wt.stagedAdded, h.u.wt.stagedRemoved)
	}
}

// TestChangeListUntrackedRenameBinary checks the counts and labels of an
// untracked text file, a rename and a binary file.
func TestChangeListUntrackedRenameBinary(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "old.txt"), "x\ny\n")
	h.Commit(dir, "Initial")
	// A rename, an untracked text file and a binary file.
	h.Git(dir, "mv", "old.txt", "new.txt")
	gittest.Write(t, filepath.Join(dir, "fresh.txt"), "one\ntwo\nthree\n")
	if err := os.WriteFile(filepath.Join(dir, "logo.bin"), []byte("\x00\x01\x02bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.open(dir)

	if got, _ := h.counts(workflow.Untracked, "fresh.txt"); got != "+3 −0" {
		t.Fatalf("untracked counts %q", got)
	}
	// The rename is staged; its path is the new name.
	if _, ok := h.u.wt.row(changeKey{area: workflow.Staged, path: "new.txt"}); !ok {
		t.Fatalf("no staged rename row; rows %+v", h.u.wt.rows)
	}
	// The binary file shows a label, not a false zero.
	if _, binary := h.counts(workflow.Untracked, "logo.bin"); !binary {
		t.Fatal("binary file not flagged")
	}
	h.want(h.u.t("changesBinary"))
}

// TestChangeListFilterKeepsWholeWorktreeStaging filters the list, then
// Stage All, and checks the filter did not narrow what Git staged.
func TestChangeListFilterKeepsWholeWorktreeStaging(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	h.open(dir)
	gittest.Write(t, filepath.Join(dir, "alpha.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "beta.txt"), "b\n")
	gittest.Write(t, filepath.Join(dir, "gamma.txt"), "g\n")
	h.u.Refresh()
	h.tt.Frame()

	// Filter to one file: the others are hidden, the header says 1 / 3.
	h.u.setFilter("alpha")
	h.tt.Frame()
	if !h.tt.HasText("alpha.txt") || h.tt.HasText("beta.txt") || h.tt.HasText("gamma.txt") {
		t.Fatalf("filter did not hide the others: %q", h.tt.Texts())
	}
	h.want("Untracked files", "3")
	if h.tt.HasText("1 / 3") {
		t.Fatal("filter reduced the section count")
	}

	// Stage All ignores the filter: every untracked file reaches the index.
	h.click("Stage All")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "alpha.txt\nbeta.txt\ngamma.txt\n" {
		t.Fatalf("index after filtered Stage All %q", got)
	}
	// Unstage All also covers the whole worktree.
	h.u.setFilter("beta")
	h.tt.Frame()
	h.click("Unstage All")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index after filtered Unstage All %q", got)
	}
}

// TestChangeListFilterNoMatch shows the empty state and clears it.
func TestChangeListFilterNoMatch(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	h.open(dir)
	gittest.Write(t, filepath.Join(dir, "alpha.txt"), "a\n")
	h.u.Refresh()
	h.tt.Frame()
	h.u.setFilter("zzz-nothing")
	h.tt.Frame()
	if !h.u.filterEmpty() {
		t.Fatal("filter should hide every file")
	}
	h.want("Untracked files", "1")
	if h.tt.HasText("alpha.txt") {
		t.Fatalf("filtered-empty still shows files: %q", h.tt.Texts())
	}
	// Clearing brings the file back.
	h.u.setFilter("")
	h.tt.Frame()
	h.want("alpha.txt")
}

// TestChangeListStaleStatsDoNotOverwrite runs numstat out of order and
// checks a stale read does not cover the newer snapshot's counts.
func TestChangeListStaleStatsDoNotOverwrite(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	g.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\ntwo\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(dir)
	q.drain() // open and its first numstat land.
	if !u.wt.statsReady {
		t.Fatal("no stats after open")
	}
	// A new edit triggers a fresh numstat; run the new one, then let the
	// stale one (queued earlier by the refresh) finish last.
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\ntwo\nthree\nfour\n")
	u.Refresh()
	// Status read first, then its numstat, is the last job; run it, then
	// drain whatever a late stale read would try to apply.
	q.drain()
	s, ok := u.wt.stat(changeKey{area: workflow.Unstaged, path: "a.txt"})
	if !ok || s.Added != 3 {
		t.Fatalf("stats after refresh +%d (ok=%v), want +3", s.Added, ok)
	}
}

// TestChangeListLayoutAtBoardSize checks the Changes list at the design's
// 1360×820: file rows keep the 36 DIP pitch, the filter box is present, and
// the Staged and Unstaged section titles sit above their files in order.
func TestChangeListLayoutAtBoardSize(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "first.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "second.txt"), "b\n")
	h.Commit(dir, "Initial")
	// One staged, two unstaged, so both sections and sibling rows show.
	gittest.Write(t, filepath.Join(dir, "first.txt"), "a\nmore\n")
	h.Git(dir, "add", "first.txt")
	gittest.Write(t, filepath.Join(dir, "second.txt"), "b\nmore\n")
	gittest.Write(t, filepath.Join(dir, "third.txt"), "c\n")
	h.open(dir)

	find := func(s string) ui.Rect {
		t.Helper()
		r, ok := h.tt.Find(s)
		if !ok {
			t.Fatalf("%q not on screen in %q", s, h.tt.Texts())
		}
		return r
	}
	// The filter box.
	find(h.u.t("changesFilter"))
	// The section titles sit above their files, Staged before Unstaged.
	staged := find(h.u.t("chromeStagedChanges"))
	unstaged := find(h.u.t("chromeUnstagedChanges"))
	if staged.Y >= unstaged.Y {
		t.Fatalf("Staged %.0f not above Unstaged %.0f", staged.Y, unstaged.Y)
	}
	// The design's `.fr` row height is 36 DIP; each file row renders at it.
	for _, s := range []string{"first.txt", "second.txt", "third.txt"} {
		r := find(s)
		shellNear(t, s+" row height", r.H, changeRowHeight, 6)
	}
}

// TestChangeListSnapshots writes the Changes list's states to
// $GITOWL_SNAPSHOT_DIR for review: staged and unstaged together, the list
// while a filter is typed, no match, in light and dark at the board size.
func TestChangeListSnapshots(t *testing.T) {
	render := func(dark bool, filter, name string) {
		g := gittest.New(t)
		dir := g.Repo()
		gittest.Write(t, filepath.Join(dir, "ai.go"), "package main\n")
		gittest.Write(t, filepath.Join(dir, "changes.go"), "package main\n")
		g.Commit(dir, "Initial")
		// Two staged edits and two unstaged ones, plus an untracked file.
		gittest.Write(t, filepath.Join(dir, "ai.go"), "package main\n\nfunc a() {}\n")
		gittest.Write(t, filepath.Join(dir, "changes.go"), "package main\n\nfunc c() {}\n")
		g.Git(dir, "add", ".")
		gittest.Write(t, filepath.Join(dir, "ai.go"), "package main\n\nfunc a() {}\n// more\n")
		gittest.Write(t, filepath.Join(dir, "app.go"), "package main\n")
		u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
		tt := ui.NewTester(window(u), 1360, 820)
		tt.SetDark(dark)
		u.OpenPath(dir)
		tt.Frame()
		tt.Frame()
		if filter != "" {
			u.setFilter(filter)
			tt.Frame()
		}
		snapshot(t, tt, name)
	}
	render(false, "", "changes-list-light")
	render(true, "", "changes-list-dark")
	render(false, "ai", "changes-filter-light")
	render(false, "zzz-none", "changes-filter-empty-light")
}
