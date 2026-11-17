package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func (h *harness) commitIndex(subject string) int {
	h.T.Helper()
	for i, c := range h.u.hist.commits {
		if c.Subject == subject {
			return i
		}
	}
	h.T.Fatalf("no commit %q", subject)
	return -1
}

func TestHistoryFromListToFileDiff(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Root commit")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\nmore\n")
	gittest.Write(t, filepath.Join(dir, "docs", "b.md"), "b\n")
	h.Commit(dir, "Second commit\n\nExplains the change.")
	h.open(dir)
	h.click("All commits")
	// Entering History opens the newest commit and its first file.
	h.want("Second commit", "Root commit", "GitOwl Test")
	if h.tt.HasText("Select a commit to see its message and files.") {
		t.Fatal("History opened on an empty detail")
	}
	d := h.u.hist.detail
	if d == nil || len(d.Files) != 2 {
		t.Fatalf("detail %+v", d)
	}
	h.want("Second commit", "Explains the change.", "Parent", d.BaseHash[:7], "a.txt", "b.md")
	h.u.selectCommitFile(0)
	h.tt.Frame()
	if p := h.u.hist.diff; p == nil || p.state == nil || !strings.Contains(p.state.New.String(), "more") {
		t.Fatalf("first file diff %+v", p)
	}
	h.u.selectCommitFile(1)
	h.tt.Frame()
	if p := h.u.hist.diff; p.key.path != "docs/b.md" || !strings.Contains(p.state.New.String(), "b") {
		t.Fatalf("second file diff %+v", p)
	}
	snapshot(t, h.tt, "history")

	h.u.selectCommit(h.commitIndex("Root commit"))
	h.tt.Frame()
	h.want("Root commit")
}

func TestHistoryEmptyAndFollowsCommits(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	h.open(dir)
	h.click("All commits")
	h.want("No commits yet")
	h.click("Current changes")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stage a.txt")
	h.typeMessage("First from GitOwl")
	h.click("Commit")
	h.click("All commits")
	h.want("First from GitOwl")

	// A commit made outside GitOwl shows after refresh.
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	h.Commit(dir, "Made in a terminal")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Made in a terminal", "First from GitOwl")
}

func TestHistoryMergeShowsFirstParentBase(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Root")
	h.Git(dir, "switch", "-q", "-c", "topic")
	gittest.Write(t, filepath.Join(dir, "t.txt"), "t\n")
	h.Commit(dir, "Topic work")
	h.Git(dir, "switch", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "m.txt"), "m\n")
	h.Commit(dir, "Main work")
	h.Git(dir, "merge", "-q", "--no-ff", "-m", "Merge topic", "topic")
	h.open(dir)
	h.click("All commits")
	h.u.selectCommit(h.commitIndex("Merge topic"))
	h.tt.Frame()
	d := h.u.hist.detail
	h.want("Parents", d.BaseHash[:7])
	if len(d.Files) != 1 || d.Files[0].Path != "t.txt" {
		t.Fatalf("merge files %+v", d.Files)
	}
}

func TestStaleHistoryResultsAreDropped(t *testing.T) {
	g := gittest.New(t)
	a, b := g.Repo(), g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	g.Commit(a, "In a")
	gittest.Write(t, filepath.Join(a, "a.txt"), "a2\n")
	g.Commit(a, "In a again")
	gittest.Write(t, filepath.Join(b, "b.txt"), "b\n")
	g.Commit(b, "In b")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain() // open, history and branches of a
	u.selectCommit(0)
	u.selectCommit(1)
	q.run(1) // the second selection's detail first…
	q.run(0) // …the first one's, stale
	if u.hist.detail == nil || u.hist.detail.Subject != "In a" {
		t.Fatalf("detail %+v", u.hist.detail)
	}
	for len(q.jobs) > 0 {
		q.run(0)
	}
	// Switching repositories drops a's pending history and detail.
	u.selectCommit(0)
	u.OpenPath(b)
	q.runLast() // open b
	q.run(0)    // a's detail: dropped
	for len(q.jobs) > 0 {
		q.run(0)
	}
	if len(u.hist.commits) != 1 || u.hist.commits[0].Subject != "In b" || u.hist.detail != nil {
		t.Fatalf("history after switch %+v %+v", u.hist.commits, u.hist.detail)
	}
}
