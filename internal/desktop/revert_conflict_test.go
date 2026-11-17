package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
)

// conflictFixture makes three commits that edit the same line, so reverting
// the middle one conflicts with HEAD. It returns the harness, the repo dir
// and the full hash of the middle commit (the revert target).
func conflictFixture(t *testing.T) (*harness, string, string) {
	t.Helper()
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "a\n")
	h.Commit(dir, "A")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "b\n")
	h.Commit(dir, "B")
	mid := strings.TrimSpace(h.Git(dir, "rev-parse", "HEAD"))
	gittest.Write(t, filepath.Join(dir, "f.txt"), "c\n")
	h.Commit(dir, "C")
	h.open(dir)
	h.toHistory()
	// Select the middle commit "B".
	h.u.selectCommit(h.commitIndex("B"))
	h.tt.Frame()
	return h, dir, mid
}

func TestRevertConflictShowsBarAndAbort(t *testing.T) {
	h, dir, _ := conflictFixture(t)
	head := strings.TrimSpace(h.Git(dir, "rev-parse", "HEAD"))
	h.chooseCommitMenu("B", "Revert")
	h.click("Revert")
	if !h.u.rev.status.Active {
		t.Fatal("conflict did not leave a revert in progress")
	}
	h.u.middleTab = 0
	h.tt.Frame()
	h.want("Conflicted", "f.txt")
	if _, ok := h.tt.Find("Continue"); ok {
		t.Fatalf("drew a Continue/Abort bar in %q", h.tt.Texts())
	}
	if _, ok := h.tt.Find("Abort"); ok {
		t.Fatalf("drew a Continue/Abort bar in %q", h.tt.Texts())
	}
	snapshot(t, h.tt, "revert-conflict-light")
	h.tt.SetDark(true)
	h.tt.Frame()
	snapshot(t, h.tt, "revert-conflict-dark")
	h.tt.SetDark(false)
	h.tt.Frame()
	h.u.abortRevert()
	h.tt.Frame()
	if h.u.rev.status.Active {
		t.Fatal("still in progress after abort")
	}
	if got := strings.TrimSpace(h.Git(dir, "rev-parse", "HEAD")); got != head {
		t.Fatalf("HEAD moved: %s want %s", got, head)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != "c\n" {
		t.Fatalf("file after abort %q", data)
	}
	if st := strings.TrimSpace(h.Git(dir, "status", "--porcelain")); st != "" {
		t.Fatalf("worktree not clean after abort: %q", st)
	}
}

func TestRevertSurvivesReopenAndBlocksCommit(t *testing.T) {
	h, dir, mid := conflictFixture(t)
	h.chooseCommitMenu("B", "Revert")
	h.click("Revert")
	if !h.u.rev.status.Active {
		t.Fatal("expected a conflict")
	}
	// Reopening the same repository recognises the revert in progress.
	h.u.repo = nil
	h.open(dir)
	if !h.u.rev.status.Active || h.u.rev.status.Target != mid {
		t.Fatalf("revert not recovered on reopen: %+v", h.u.rev.status)
	}
	// Commit is blocked while the revert is in progress.
	if u := h.u; u.commitBlock() != "errorRevertInProgress" {
		t.Fatalf("commit block %q", u.commitBlock())
	}
	// Pull is blocked too.
	if got := h.u.pullBlock(); got != "errorRevertInProgress" && got != "busy" && got != "errorNoRemote" {
		t.Fatalf("pull block %q", got)
	}
}
