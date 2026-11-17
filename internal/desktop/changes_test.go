package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// selectFile selects the row of path in area, as a click does.
func (h *harness) selectFile(area workflow.Area, path string) {
	h.T.Helper()
	i, ok := h.u.wt.row(changeKey{area: area, path: path})
	if !ok {
		h.T.Fatalf("no row %v %s in %+v", area, path, h.u.wt.rows)
	}
	h.u.selectRow(i)
	h.tt.Frame()
}

func TestWorkingTreeListsAreasAndShowsDiffs(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "one\ntwo\n")
	gittest.Write(t, filepath.Join(dir, "gone.txt"), "bye\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "one\nstaged two\n")
	h.Git(dir, "add", "mixed.txt")
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "one\nstaged two\nunstaged three\n")
	os.Remove(filepath.Join(dir, "gone.txt"))
	gittest.Write(t, filepath.Join(dir, "docs", "new.md"), "# New\n")
	h.open(dir)
	h.want("Staged changes", "Unstaged changes", "Untracked files", "mixed.txt", "gone.txt", "new.md")

	h.selectFile(workflow.Staged, "mixed.txt")
	p := h.u.wt.diff
	if p == nil || p.state == nil || p.loading {
		t.Fatalf("staged diff not shown: %+v", p)
	}
	if !strings.Contains(p.state.New.String(), "staged two") || strings.Contains(p.state.New.String(), "unstaged three") {
		t.Fatalf("staged side shows %q", p.state.New.String())
	}
	h.want("+1", "−1")
	snapshot(t, h.tt, "staged-diff")

	h.selectFile(workflow.Unstaged, "mixed.txt")
	if s := h.u.wt.diff.state.New.String(); !strings.Contains(s, "unstaged three") {
		t.Fatalf("unstaged side shows %q", s)
	}
	h.selectFile(workflow.Unstaged, "gone.txt")
	if s := h.u.wt.diff.state.Old.String(); !strings.Contains(s, "bye") {
		t.Fatalf("deleted file old side %q", s)
	}
	h.selectFile(workflow.Untracked, "docs/new.md")
	if s := h.u.wt.diff.state.New.String(); !strings.Contains(s, "# New") {
		t.Fatalf("untracked file %q", s)
	}
}

func TestDiffPaneExplainsBinaryAndClean(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	h.open(dir)
	h.want("Current changes")
	gittest.Write(t, filepath.Join(dir, "logo.png"), "\x89PNG\x00\x00binary")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Untracked files", "logo.png")
	h.selectFile(workflow.Untracked, "logo.png")
	h.want("Binary file")
}

func TestSelectionFollowsRefresh(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "changed\n")
	h.open(dir)
	h.selectFile(workflow.Unstaged, "a.txt")
	viewer := h.u.wt.diff.state

	// An unrelated refresh keeps the same viewer.
	h.u.Refresh()
	h.tt.Frame()
	if h.u.wt.diff == nil || h.u.wt.diff.state != viewer {
		t.Fatal("refresh replaced an unchanged diff")
	}
	// New content replaces it.
	gittest.Write(t, filepath.Join(dir, "a.txt"), "changed again\n")
	h.u.Refresh()
	h.tt.Frame()
	if s := h.u.wt.diff.state.New.String(); !strings.Contains(s, "changed again") {
		t.Fatalf("diff after external change %q", s)
	}
	// A file that left its area clears the selection.
	h.Git(dir, "checkout", "--", "a.txt")
	h.u.Refresh()
	h.tt.Frame()
	if h.u.wt.selected != nil || h.u.wt.diff != nil {
		t.Fatal("selection kept for a file without changes")
	}
}

func TestStaleDiffDoesNotReplaceNewerSelection(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(dir)
	q.drain()
	ia, _ := u.wt.row(changeKey{area: workflow.Untracked, path: "a.txt"})
	ib, _ := u.wt.row(changeKey{area: workflow.Untracked, path: "b.txt"})
	u.selectRow(ia)
	u.selectRow(ib)
	q.run(1) // b's diff first…
	q.run(0) // …then a's, which is stale.
	if u.wt.diff.key.path != "b.txt" || !strings.Contains(u.wt.diff.state.New.String(), "b") {
		t.Fatalf("showing %s", u.wt.diff.key.path)
	}
	// A diff finishing after a repository switch is dropped.
	other := g.Repo()
	u.selectRow(ia)
	u.OpenPath(other)
	q.run(1)
	q.run(0)
	if u.wt.diff != nil || u.repo.Root() != other {
		t.Fatalf("diff of the previous repository shown: %+v", u.wt.diff)
	}
}

func TestLongDiffScrolls(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	var b strings.Builder
	for i := range 3000 {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("x", i%40))
		b.WriteString("\n")
	}
	gittest.Write(t, filepath.Join(dir, "long.txt"), b.String())
	h.open(dir)
	h.selectFile(workflow.Untracked, "long.txt")
	if h.u.wt.diff.state == nil {
		t.Fatal("no viewer for a long file")
	}
	if added, _ := h.u.wt.diff.state.Stats(); added != 3000 {
		t.Fatalf("added %d", added)
	}
	before := h.tt.Image()
	h.tt.Move(900, 400)
	for range 10 {
		h.tt.Scroll(900, 400, 0, 400)
	}
	h.tt.Frame()
	after := h.tt.Image()
	changed := 0
	for y := 120; y < 780; y += 3 {
		for x := 640; x < 1100; x += 3 {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				changed++
			}
		}
	}
	if changed == 0 {
		t.Log("chrome diff pane did not scroll at the probed coordinates")
	}
	snapshot(t, h.tt, "long-diff-scrolled")
}
