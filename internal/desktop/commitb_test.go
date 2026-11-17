package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// stageOne opens a repository with a single staged file ready to commit,
// so a test can type a message and commit at once.
func stageOne(h *harness) string {
	h.T.Helper()
	dir := h.Repo()
	gittest.Write(h.T, filepath.Join(dir, "a.txt"), "a\n")
	h.Git(dir, "add", "a.txt")
	h.open(dir)
	return dir
}

// TestSummaryAndDescriptionCompose checks the design's two fields: the
// commit's message is the Summary alone, or the Summary, a blank line and
// the Description when the Description has content (commit-b.md).
func TestSummaryAndDescriptionCompose(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := stageOne(h)
	h.click("Summary")
	h.tt.Type("Add a feature")
	h.tt.Frame()
	h.click("Description")
	h.tt.Type("Why it matters.\nMore detail.")
	h.tt.Frame()
	want := "Add a feature\n\nWhy it matters.\nMore detail."
	if h.u.cm.message != want {
		t.Fatalf("message %q, want %q", h.u.cm.message, want)
	}
	h.click("Commit")
	if got := strings.TrimRight(h.Git(dir, "log", "-1", "--format=%B"), "\n"); got != want {
		t.Fatalf("committed message %q, want %q", got, want)
	}
}

// TestSummaryCountsUnicodeAndNeverBlocks counts the Summary by Unicode
// character (rune), shows n / 72, and lets a Summary past 72 still commit:
// the count only changes color (commit-b.md).
func TestSummaryCountsUnicodeAndNeverBlocks(t *testing.T) {
	h := newHarness(t, 1280, 800)
	stageOne(h)
	// Chinese and an emoji: four runes, not their byte length.
	h.click("Summary")
	h.tt.Type("测试🚀x")
	h.tt.Frame()
	if n := len([]rune(h.u.cm.summary)); n != 4 {
		t.Fatalf("summary has %d runes, want 4", n)
	}
	h.want("4 / 72")

	// A Summary past 72 runes still commits; the count only warns. A fresh
	// repository keeps the Summary field empty before typing the long one.
	h2 := newHarness(t, 1280, 800)
	dir := stageOne(h2)
	long := strings.Repeat("词", 80)
	h2.click("Summary")
	h2.tt.Type(long)
	h2.tt.Frame()
	if n := len([]rune(h2.u.cm.summary)); n != 80 {
		t.Fatalf("summary has %d runes, want 80", n)
	}
	h2.want("80 / 72")
	if h2.u.commitBlock() != "" {
		t.Fatalf("a long Summary blocked the commit: %q", h2.u.commitBlock())
	}
	h2.click("Commit")
	if got := h2.Git(dir, "log", "-1", "--format=%s"); got != long+"\n" {
		t.Fatalf("subject %q", got)
	}
}

// TestBlankSummaryDisablesCommit keeps Commit disabled while the Summary is
// blank, even with whitespace, and names why under the button.
func TestBlankSummaryDisablesCommit(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := stageOne(h)
	h.u.cm.summary, h.u.cm.description, h.u.cm.composed = "   ", "body", "nope"
	h.tt.Frame()
	if h.u.commitBlock() != "errorEmptyMessage" {
		t.Fatalf("blank Summary did not block: %q", h.u.commitBlock())
	}
	h.want("Add a message to commit")
	h.click("Commit")
	if out := h.Git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("a blank-Summary commit ran")
	}
}

// TestCmdReturnCommitsOnce fires ⌘Return twice while the commit is still
// pending and leaves exactly one commit: the shortcut shares the button's
// over-run protection.
func TestCmdReturnCommitsOnce(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	g.Git(dir, "add", "a.txt")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	tt.Frame()
	if err := tt.Click("Summary"); err != nil {
		t.Fatalf("%v (texts %q)", err, tt.Texts())
	}
	tt.Type("Once")
	tt.Frame()
	q.drain()
	// Two ⌘Return in a row with the first still pending.
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	if !u.cm.committing {
		t.Fatal("⌘Return did not start a commit")
	}
	q.drain()
	if n := strings.Count(g.Git(dir, "log", "--oneline"), "\n"); n != 1 {
		t.Fatalf("%d commits", n)
	}
}

// TestDraftsStayWithTheirWorktree keeps each worktree's Summary and
// Description to itself: switching worktrees and back restores exactly what
// was typed, and the other worktree never sees it. It drives the real
// OpenPath / saveSession / restoreSession path.
func TestDraftsStayWithTheirWorktree(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	// A linked worktree on a second branch.
	other := filepath.Join(t.TempDir(), "wt-two")
	h.Git(dir, "worktree", "add", "-q", "-b", "two", other)
	// Both worktrees need a change so the commit panel is shown.
	gittest.Write(t, filepath.Join(dir, "a.txt"), "main change\n")
	gittest.Write(t, filepath.Join(other, "a.txt"), "other change\n")
	h.open(dir)
	h.u.Refresh()
	h.tt.Frame()
	if len(h.u.trees.list) != 2 {
		t.Fatalf("worktree list has %d entries", len(h.u.trees.list))
	}

	h.click("Summary")
	h.tt.Type("Main draft")
	h.tt.Frame()
	h.click("Description")
	h.tt.Type("Main body")
	h.tt.Frame()

	// Switch to the other worktree: a clean, separate composer.
	linked := h.u.trees.list[0]
	if linked.Current {
		linked = h.u.trees.list[1]
	}
	h.u.openWorktree(linked)
	h.tt.Frame()
	if h.u.snap.Branch != "two" {
		t.Fatalf("did not switch worktree: on %q", h.u.snap.Branch)
	}
	if h.u.cm.summary != "" || h.u.cm.description != "" || h.u.cm.message != "" {
		t.Fatalf("the main draft leaked: %+v", h.u.cm)
	}
	h.click("Summary")
	h.tt.Type("Other draft")
	h.tt.Frame()

	// Back to the main worktree: its own words return.
	main := h.u.trees.list[0]
	if !main.Main {
		main = h.u.trees.list[1]
	}
	h.u.openWorktree(main)
	h.tt.Frame()
	if h.u.snap.Branch != "main" {
		t.Fatalf("did not return to main: on %q", h.u.snap.Branch)
	}
	if h.u.cm.summary != "Main draft" || h.u.cm.description != "Main body" {
		t.Fatalf("main draft not restored: %+v", h.u.cm)
	}
	if h.u.cm.message != "Main draft\n\nMain body" {
		t.Fatalf("message not recomposed: %q", h.u.cm.message)
	}
}
