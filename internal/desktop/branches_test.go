package desktop

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func TestBranchManagementFromTheWindow(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	h.openBranchFolders()
	h.want("Branches", "main", "main, current branch")
	if _, ok := h.tt.Find("Current"); ok {
		t.Fatal("a Current label beside the name")
	}

	h.chooseBranchMenu(h.u.t("currentBranchName", "branch", "main"), "New branch from here")
	h.want("New Branch", "Starts at this branch: main.")
	h.click("Branch name")
	h.tt.Type("bad..name")
	h.tt.Frame()
	h.click("Create Branch")
	h.want("That is not a valid branch name. Avoid spaces, \"..\", a leading \"-\" and characters such as ~ ^ : ? * [.")
	h.u.br.form.name = "feature/one"
	h.tt.Frame()
	snapshot(t, h.tt, "new-branch")
	h.click("Create Branch")
	h.tt.Frame()
	h.openBranchFolders()
	h.branchAction("feature/one", "Checkout")
	if h.u.snap.Branch != "feature/one" {
		t.Fatalf("after create: branch=%s", h.u.snap.Branch)
	}
	h.want("feature/one", "main")
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "feature/one" {
		t.Fatalf("git branch %q", got)
	}

	// Commit on the new branch, then switch back: history follows.
	gittest.Write(t, filepath.Join(dir, "f.txt"), "f\n")
	h.u.Refresh()
	h.tt.Frame()
	h.click("Stage f.txt")
	h.typeMessage("Work on feature")
	h.click("Commit")
	h.branchAction("main", "Checkout")
	if h.u.snap.Branch != "main" {
		t.Fatalf("branch %s", h.u.snap.Branch)
	}
	// History reads again at the new HEAD. Its scope stays All branches, so
	// feature's commits can still be listed.
	if h.u.hist.branch != h.u.snap.Branch {
		t.Fatalf("history read at %q, the checkout is %q", h.u.hist.branch, h.u.snap.Branch)
	}

	// Deleting an unmerged branch is refused after confirmation.
	h.branchAction("feature/one", "Delete branch…")
	h.want("Delete branch feature/one?")
	h.click("Delete")
	h.want("Git did not delete the branch because its commits are not merged. GitOwl does not force-delete branches.")
	h.want("feature/one")

	// A merged branch is deleted.
	h.Git(dir, "branch", "merged")
	h.u.Refresh()
	h.tt.Frame()
	h.branchAction("merged", "Delete branch…")
	h.click("Delete")
	if strings.Contains(h.Git(dir, "branch"), "merged") {
		t.Fatal("merged branch not deleted")
	}
	snapshot(t, h.tt, "branches")
}

func TestBranchDeleteNeedsConfirmation(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.Git(dir, "branch", "keep")
	h.open(dir)
	h.branchAction("keep", "Delete branch…")
	h.click("Cancel")
	if !strings.Contains(h.Git(dir, "branch"), "keep") {
		t.Fatal("deleted without confirmation")
	}
}

func TestBlockedSwitchKeepsWork(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "main\n")
	h.Commit(dir, "Initial")
	h.Git(dir, "switch", "-q", "-c", "other")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "other\n")
	h.Commit(dir, "Other")
	h.Git(dir, "switch", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "precious\n")
	h.open(dir)
	h.branchAction("other", "Checkout")
	h.want("Git did not switch branches because local changes would be overwritten. Your changes are kept; commit them or move them aside, then switch again.", "Unstaged changes")
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "precious\n" {
		t.Fatalf("work lost: %q", data)
	}
	if h.u.snap.Branch != "main" {
		t.Fatal(h.u.snap.Branch)
	}
}

func TestBranchesInUnbornRepository(t *testing.T) {
	h := newHarness(t, 1280, 800)
	h.open(h.Repo())
	h.want("No commits yet")
	if len(h.u.br.data.Local) != 0 {
		t.Fatalf("listed a branch that does not exist: %+v", h.u.br.data.Local)
	}
}

func TestRemoteBranchesAreMarkedLocal(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.Git(dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	h.open(dir)
	h.openBranchFolders()
	h.want("Remotes", "origin/main")
}

func TestBranchResultAfterSwitchIsDropped(t *testing.T) {
	g := gittest.New(t)
	a, b := g.Repo(), g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	g.Commit(a, "Initial")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain()
	u.br.newName = "late"
	u.createBranch()
	u.OpenPath(b)
	q.runLast() // open b
	q.drain()   // a's create runs on a closed handle and is dropped
	if u.repo.Root() != b || u.br.busy != "" || len(u.br.data.Local) != 0 {
		t.Fatalf("stale branch result: %+v", u.br)
	}
	if strings.Contains(g.Git(a, "branch"), "late") {
		t.Fatal("closed repository was changed")
	}
}

// tabTo moves the keyboard focus to the control labeled label.
func (h *harness) tabTo(label string) {
	h.T.Helper()
	for range 120 {
		if h.tt.Focused(label) {
			return
		}
		h.tt.Key(0, ui.KeyTab)
		h.tt.Frame()
	}
	h.T.Fatalf("Tab never reached %q", label)
}

func TestBranchMenuFromTheKeyboard(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.Git(dir, "branch", "keep")
	h.open(dir)
	got := h.openBranchMenu("keep")
	if !slices.Contains(got, "Checkout") || !slices.Contains(got, "Delete branch…") {
		t.Fatalf("menu %q", got)
	}
	if err := h.tt.ChooseMenuItem("Checkout"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.u.snap.Branch != "keep" {
		t.Fatalf("branch %s", h.u.snap.Branch)
	}
	snapshot(t, h.tt, "branch-menu")
}

func TestBranchInAnotherWorktreeExplained(t *testing.T) {
	h := newHarness(t, 1280, 800)
	main, linked := twoWorktrees(t, h.Env)
	h.open(main)
	h.u.chrome.navFilter = "side"
	h.tt.Frame()
	h.branchAction("side", "Delete branch…")
	h.click("Delete")
	h.want(h.u.t("errorBranchInWorktree"))
	if h.u.notice == nil || !strings.Contains(h.u.notice.detail, linked) {
		t.Fatalf("missing other worktree in %+v", h.u.notice)
	}
	if strings.Contains(strings.Join(h.tt.Texts(), "\n"), "The current branch cannot be deleted") {
		t.Fatal("reported as the current branch")
	}
	if !strings.Contains(h.Git(main, "branch"), "side") {
		t.Fatal("branch deleted")
	}
}
