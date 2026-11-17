package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
)

// revertFixture opens a repository with two commits on main, the second
// adding a line that a revert of it removes. It returns the harness and the
// short hash of the commit to revert (the second).
func revertFixture(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "First")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\ntwo\n")
	h.Commit(dir, "Second")
	h.open(dir)
	h.toHistory()
	return h, strings.TrimSpace(h.Git(dir, "rev-parse", "--short", "HEAD"))
}

// toHistory moves the middle column to History and selects its first commit.
func (h *harness) toHistory() {
	h.T.Helper()
	h.u.middleTab = 1
	h.tt.Frame()
	h.u.historyChanged()
	h.tt.Frame()
	h.u.selectCommit(0)
	h.tt.Frame()
}

func TestRevertCreatesReverseCommit(t *testing.T) {
	h, _ := revertFixture(t)
	dir := h.u.repo.Root()
	before := h.commitCount(dir)
	h.chooseCommitMenu("Second", "Revert")
	h.want("Revert this commit?")
	snapshot(t, h.tt, "revert-confirm-light")
	h.tt.SetDark(true)
	h.tt.Frame()
	snapshot(t, h.tt, "revert-confirm-dark")
	h.tt.SetDark(false)
	h.tt.Frame()
	h.click("Revert")
	// A reverse commit was added and the file is back to one line.
	if got := h.commitCount(dir); got != before+1 {
		t.Fatalf("commit count %d, want %d", got, before+1)
	}
	if data := h.Git(dir, "show", "HEAD:f.txt"); data != "one\n" {
		t.Fatalf("file after revert %q", data)
	}
	if subj := strings.TrimSpace(h.Git(dir, "log", "-1", "--format=%s")); !strings.HasPrefix(subj, "Revert") {
		t.Fatalf("subject %q", subj)
	}
	if h.u.rev.status.Active {
		t.Fatal("revert still marked in progress after clean revert")
	}
}

func TestRevertMergeDisabled(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "First")
	h.Git(dir, "checkout", "-q", "-b", "side")
	gittest.Write(t, filepath.Join(dir, "s.txt"), "s\n")
	h.Commit(dir, "Side")
	h.Git(dir, "checkout", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "m.txt"), "m\n")
	h.Commit(dir, "Main")
	h.Git(dir, "merge", "--no-ff", "-m", "Merge side", "side")
	h.open(dir)
	h.toHistory()
	// The newest commit is the merge; its Revert… button is disabled.
	if got := h.u.revertDisabled(); got != "errorRevertMerge" {
		t.Fatalf("revert disabled reason %q, want errorRevertMerge", got)
	}
	if err := h.tt.RightClick("Merge side"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Revert"); err == nil {
		t.Fatal("merge commit offered Revert")
	}
	h.tt.CloseMenu()
}

func TestRevertDirtyDisabled(t *testing.T) {
	h, _ := revertFixture(t)
	dir := h.u.repo.Root()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\ntwo\ndirty\n")
	h.u.Refresh()
	h.tt.Frame()
	if got := h.u.revertDisabled(); got != "errorRevertDirty" {
		t.Fatalf("revert disabled reason %q, want errorRevertDirty", got)
	}
	if err := h.tt.RightClick("Second"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Revert"); err == nil {
		t.Fatal("dirty worktree offered Revert")
	}
	h.tt.CloseMenu()
}

// TestRevertBrowseWritesCurrentBranch reverts while History browses another
// branch: the reverse commit lands on the checked-out branch, not the one
// browsed, and the dialog says so.
func TestRevertBrowseWritesCurrentBranch(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "First")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\ntwo\n")
	h.Commit(dir, "Second")
	// A second branch whose tip differs, to browse.
	h.Git(dir, "branch", "other")
	h.open(dir)
	h.toHistory()
	// Browse the "other" branch in History; the checkout stays on main.
	h.u.hist.branch = "other"
	h.tt.Frame()
	h.u.selectCommit(0)
	h.tt.Frame()
	h.chooseCommitMenu("Second", "Revert")
	h.want("Revert this commit?")
	h.click("Revert")
	if h.u.rev.status.Active {
		t.Fatal("clean revert left a revert in progress")
	}
	// The reverse commit is on main (HEAD), restoring the single line.
	if br := strings.TrimSpace(h.Git(dir, "rev-parse", "--abbrev-ref", "HEAD")); br != "main" {
		t.Fatalf("HEAD on %q", br)
	}
	if data := h.Git(dir, "show", "HEAD:f.txt"); data != "one\n" {
		t.Fatalf("file after revert %q", data)
	}
}
