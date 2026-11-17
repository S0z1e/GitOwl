package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func chromeHistoryMenuRepo(t *testing.T) (*harness, string) {
	t.Helper()
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Older")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	h.Commit(dir, "Newest")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	return h, dir
}

func (h *harness) commitBySubject(subject string) workflow.CommitInfo {
	h.T.Helper()
	for _, c := range h.u.hist.commits {
		if c.Subject == subject {
			return c
		}
	}
	h.T.Fatalf("no commit %q", subject)
	return workflow.CommitInfo{}
}

func (h *harness) openCommitMenu(subject string) []string {
	h.T.Helper()
	if err := h.tt.RightClick(subject); err != nil {
		h.T.Fatalf("open %q: %v (texts %q)", subject, err, h.tt.Texts())
	}
	return h.tt.Menu()
}

func (h *harness) chooseCommitMenu(subject, item string) {
	h.T.Helper()
	h.openCommitMenu(subject)
	if err := h.tt.ChooseMenuItem(item); err != nil {
		h.T.Fatalf("%v (menu %q)", err, h.tt.Menu())
	}
	h.tt.Frame()
}

func TestChromeCommitMenuGroupsInDesignOrder(t *testing.T) {
	h, _ := chromeHistoryMenuRepo(t)
	c := h.commitBySubject("Older")
	got := h.openCommitMenu("Older")
	want := []string{
		"Older",
		"-",
		"Checkout commit",
		"-",
		"New branch",
		"New tag",
		"-",
		"Merge " + c.Short() + " into main",
		"Rebase main onto " + c.Short(),
		"Interactive rebase",
		"Cherry-pick",
		"-",
		"Revert",
		"Reset",
		"-",
		"Save as patch",
		"Compare to working dir",
		"-",
		"Copy hash",
		"Copy info",
	}
	if len(got) != len(want) {
		t.Fatalf("menu %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("menu %q", got)
		}
	}
}

func TestChromeCommitMenuInteractiveRebaseStaysUnavailable(t *testing.T) {
	h, _ := chromeHistoryMenuRepo(t)
	h.openCommitMenu("Older")
	if err := h.tt.ChooseMenuItem("Interactive rebase"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Interactive rebase should be disabled: %v (menu %q)", err, h.tt.Menu())
	}
	if hint := h.u.t("commitInteractiveRebaseHint"); hint != "This screen does not exist yet." {
		t.Fatalf("hint %q", hint)
	}
	for _, s := range []string{"Reorder commits", "Continue rebase", "Abort rebase"} {
		if h.tt.HasText(s) {
			t.Fatalf("showed a rebase list %q in %q", s, h.tt.Texts())
		}
	}
}

func TestChromeCommitMenuCheckoutDetachesAtHash(t *testing.T) {
	h, dir := chromeHistoryMenuRepo(t)
	c := h.commitBySubject("Older")
	h.chooseCommitMenu("Older", "Checkout commit")
	h.want("Check out this commit?", h.u.t("commitCheckoutBody"), "Cancel", "Check out commit")
	h.click("Check out commit")
	if !h.u.snap.Detached || h.u.snap.Head != c.Hash {
		t.Fatalf("not detached at Older: %+v", h.u.snap)
	}
	if got := strings.TrimSpace(h.Git(dir, "rev-parse", "HEAD")); got != c.Hash {
		t.Fatalf("HEAD %s, want %s", got, c.Hash)
	}
	h.want("Detached at " + c.Short())
	if h.tt.HasText("Detached HEAD explained") || h.tt.HasText("What is a detached HEAD") {
		t.Fatalf("opened a detached-HEAD page: %q", h.tt.Texts())
	}
}

func TestChromeCommitMenuMixedResetKeepsWorktree(t *testing.T) {
	h, dir := chromeHistoryMenuRepo(t)
	older := h.commitBySubject("Older")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "kept\n")
	h.u.Refresh()
	h.tt.Frame()
	h.chooseCommitMenu("Older", "Reset")
	h.want("Reset to this commit?", h.u.t("commitResetBody"), "Cancel", "Reset")
	if h.tt.HasText("soft") || h.tt.HasText("hard") || h.tt.HasText("mixed") {
		t.Fatalf("reset offered a mode: %q", h.tt.Texts())
	}
	h.click("Reset")
	if got := strings.TrimSpace(h.Git(dir, "rev-parse", "HEAD")); got != older.Hash {
		t.Fatalf("HEAD %s, want Older", got)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "a.txt")); err != nil || string(data) != "kept\n" {
		t.Fatalf("worktree lost: %v %q", err, data)
	}
}

func TestChromeCommitMenuRevertConflictShowsMarkersWithoutContinue(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "f.txt"), "a\n")
	h.Commit(dir, "A")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "b\n")
	h.Commit(dir, "B")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "c\n")
	h.Commit(dir, "C")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	h.chooseCommitMenu("B", "Revert")
	h.want("Revert this commit?", h.u.t("commitRevertBody"), "Cancel", "Revert")
	h.click("Revert")
	if h.u.middleTab != 0 {
		t.Fatalf("conflict left tab %d", h.u.middleTab)
	}
	h.want("Conflicted")
	if _, ok := h.tt.Find("Continue"); ok {
		t.Fatalf("showed Continue after revert conflict in %q", h.tt.Texts())
	}
	if _, ok := h.tt.Find("Abort"); ok {
		t.Fatalf("showed Abort after revert conflict in %q", h.tt.Texts())
	}
}

func TestChromeCommitMenuNewBranchAndNewTagReuseDialog(t *testing.T) {
	h, dir := chromeHistoryMenuRepo(t)
	c := h.commitBySubject("Older")
	h.chooseCommitMenu("Older", "New branch")
	h.want("New Branch", h.u.t("newBranchFromCommit", "hash", c.Short(), "subject", "Older"), "Branch name", "Cancel", "Create Branch")
	h.tt.Type("from-older")
	h.tt.Frame()
	h.click("Create Branch")
	if got := strings.TrimSpace(h.Git(dir, "rev-parse", "from-older")); got != c.Hash {
		t.Fatalf("branch at %s, want %s", got, c.Hash)
	}
	if h.u.snap.Detached || h.u.snap.Branch != "main" {
		t.Fatalf("new branch switched: %+v", h.u.snap)
	}

	h.click("All commits")
	h.chooseCommitMenu("Older", "New tag")
	h.want("New Branch", h.u.t("newTagFromCommit", "hash", c.Short(), "subject", "Older"), "Tag name", "Create Tag")
	if h.tt.HasText("Annotation") || h.tt.HasText("Message") {
		t.Fatalf("tag dialog grew an annotation field: %q", h.tt.Texts())
	}
	h.tt.Type("v-older")
	h.tt.Frame()
	h.click("Create Tag")
	if !strings.Contains(h.Git(dir, "tag"), "v-older") {
		t.Fatalf("tag missing: %s", h.Git(dir, "tag"))
	}
}

func TestChromeCommitMenuDialogKeepsTabAndReturnsFocus(t *testing.T) {
	h, _ := chromeHistoryMenuRepo(t)
	h.click("Older")
	h.chooseCommitMenu("Older", "New branch")
	h.tt.Advance(time.Second)
	h.tt.Frame()
	seen := h.cycleInDialog("New Branch")
	if !seen["Branch name"] || !seen["Cancel"] || !seen["Create Branch"] {
		t.Fatalf("dialog controls %v", seen)
	}
	h.closeAndBack("New Branch", "Older")
}

func TestChromeCommitMenuMergeAndRebaseConfirm(t *testing.T) {
	h, _ := chromeHistoryMenuRepo(t)
	c := h.commitBySubject("Older")
	h.chooseCommitMenu("Older", "Merge "+c.Short()+" into main")
	h.want("Merge "+c.Short()+" into main?", h.u.t("mergeBody"), "Cancel", "Merge")
	h.click("Cancel")
	h.chooseCommitMenu("Older", "Rebase main onto "+c.Short())
	h.want("Rebase main onto "+c.Short()+"?", h.u.t("rebaseBody"), "Cancel", "Rebase")
}

func TestChromeCommitMenuCherryPickOntoCurrentBranch(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "picked.txt"), "p\n")
	h.Commit(dir, "To pick")
	h.Git(dir, "revert", "-n", "HEAD")
	h.Git(dir, "commit", "-q", "-m", "Undo pick")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	if _, err := os.Stat(filepath.Join(dir, "picked.txt")); !os.IsNotExist(err) {
		t.Fatal("setup left picked.txt")
	}
	h.chooseCommitMenu("To pick", "Cherry-pick")
	h.want("Cherry-pick this commit?", h.u.t("commitCherryBody"), "Cherry-pick")
	h.click("Cherry-pick")
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("left main: %s", got)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "picked.txt")); err != nil || string(b) != "p\n" {
		t.Fatalf("cherry-pick did not apply: %v %q", err, b)
	}
}

func TestChromeCommitMenuRevertCreatesReverseCommit(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "First")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\ntwo\n")
	h.Commit(dir, "Second")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	before := h.Git(dir, "log", "-1", "--format=%s")
	h.chooseCommitMenu("Second", "Revert")
	h.want("Revert this commit?")
	if h.tt.HasText("Commit summary") && h.u.br.confirm.kind == "revert" {
		t.Fatalf("revert showed a message editor: %q", h.tt.Texts())
	}
	h.click("Revert")
	subj := strings.TrimSpace(h.Git(dir, "log", "-1", "--format=%s"))
	if !strings.HasPrefix(subj, "Revert") {
		t.Fatalf("subject %q (was %q)", subj, before)
	}
	if data := h.Git(dir, "show", "HEAD:f.txt"); data != "one\n" {
		t.Fatalf("file after revert %q", data)
	}
	if h.u.middleTab != 0 {
		t.Fatalf("revert left tab %d", h.u.middleTab)
	}
}

func TestChromeCommitMenuCompareLeavesFromNav(t *testing.T) {
	h, _ := chromeHistoryMenuRepo(t)
	h.chooseCommitMenu("Older", "Compare to working dir")
	h.want("Compare Older and working directory", "Unified", "Split")
	if h.tt.HasText("Generate Draft") {
		t.Fatal("compare kept the commit box")
	}
	h.click("Current changes")
	if h.u.cmp.open {
		t.Fatal("Current changes did not leave compare")
	}
	h.click("All commits")
	h.chooseCommitMenu("Older", "Compare to working dir")
	h.click("All commits")
	if h.u.cmp.open {
		t.Fatal("All commits did not leave compare")
	}
}

func TestChromeCommitMenuCopyHashAndCopyInfo(t *testing.T) {
	h, _ := chromeHistoryMenuRepo(t)
	c := h.commitBySubject("Older")
	h.chooseCommitMenu("Older", "Copy hash")
	if got := h.tt.Clipboard(); got != c.Hash {
		t.Fatalf("hash clipboard %q, want %q", got, c.Hash)
	}
	h.chooseCommitMenu("Older", "Copy info")
	want := commitCopyInfo(c, h.u.t("commitWhen", "when", c.When))
	if got := h.tt.Clipboard(); got != want {
		t.Fatalf("info clipboard %q, want %q", got, want)
	}
	if strings.Count(want, "\n") != 3 {
		t.Fatalf("copy info is not four lines: %q", want)
	}
}

func TestChromeCommitMenuSaveAsPatch(t *testing.T) {
	var saved string
	g := gittest.New(t)
	u := New(Options{
		Workflow:  workflow.New(g.Runner),
		Preferred: []string{"en-US"},
		Go:        func(fn func()) { fn() },
		ChooseSaveFile: func(title, suggested string) (string, error) {
			saved = filepath.Join(t.TempDir(), suggested)
			return saved, nil
		},
	})
	h := &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), 1280, 800)}
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "changed\n")
	h.Commit(dir, "Patch me")
	h.open(dir)
	h.click("All commits")
	h.chooseCommitMenu("Patch me", "Save as patch")
	body, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "changed") {
		t.Fatalf("patch %q", body)
	}
}
