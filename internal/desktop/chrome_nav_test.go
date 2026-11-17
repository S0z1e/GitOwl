package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func chromeBranchRepo(t *testing.T, w, h int) (*harness, string) {
	t.Helper()
	hr, dir := chromeRepo(t, w, h)
	for _, b := range []string{"feature/ui/tree", "feature/api"} {
		hr.Git(dir, "branch", b)
	}
	head := strings.TrimSpace(hr.Git(dir, "rev-parse", "HEAD"))
	hr.Git(dir, "update-ref", "refs/remotes/origin/feature/remote-only", head)
	hr.Git(dir, "tag", "v1")
	hr.u.Refresh()
	hr.tt.Frame()
	return hr, dir
}

func TestChromeNavClickBranchStaysOnCurrentChanges(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	h.want("Current changes", "All commits", "Worktrees", "Branches", "Remotes", "Tags")
	if h.tt.HasText("Stashes") {
		t.Fatalf("empty stashes group stayed in %q", h.tt.Texts())
	}
	if h.u.middleTab != 0 {
		t.Fatalf("opened on tab %d", h.u.middleTab)
	}
	scope := histShown(h)
	h.click("tree")
	if h.u.chrome.navSel != "local:feature/ui/tree" {
		t.Fatalf("selected %q", h.u.chrome.navSel)
	}
	if h.u.middleTab != 0 {
		t.Fatalf("click left Current changes for tab %d", h.u.middleTab)
	}
	if histShown(h) != scope {
		t.Fatalf("click changed history scope to %v", histShown(h))
	}
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("click checked out %s", got)
	}
}

func TestChromeNavDoubleClickChecksOutBranch(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	h.click("tree")
	h.click("tree")
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "feature/ui/tree" || h.u.snap.Branch != "feature/ui/tree" {
		t.Fatalf("double click: git %s, window %s", got, h.u.snap.Branch)
	}
	h.want("feature/ui/tree, current branch")

	h.tt.SetFocused(true)
	h.click("api")
	h.press(ui.KeyEnter)
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "feature/api" {
		t.Fatalf("Enter: %s", got)
	}
}

func TestChromeNavSingleWorktreeHidesRows(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	name := filepath.Base(dir)
	h.want("Worktrees", "Current changes", "All commits")
	if _, ok := h.tt.Find(h.u.t("currentWorktreeName", "name", name)); ok {
		t.Fatalf("single worktree drew a row in %q", h.tt.Texts())
	}
}

func TestChromeNavFilterHidesEmptyGroups(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	gittest.Write(t, filepath.Join(dir, "wip.txt"), "wip\n")
	h.Git(dir, "stash", "push", "-u", "-m", "parked work")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Stashes")

	if err := h.tt.Click("Filter"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("NOMATCH")
	h.tt.Frame()
	h.want("Current changes", "All commits")
	for _, gone := range []string{"Worktrees", "Branches", "Remotes", "Tags", "Stashes"} {
		if h.tt.HasText(gone) {
			t.Fatalf("filter left %q in %q", gone, h.tt.Texts())
		}
	}
	for _, leaf := range []string{"tree", "api", "v1", "feature/ui/tree"} {
		if _, ok := h.tt.Find(leaf); ok {
			t.Fatalf("filter left row %q in %q", leaf, h.tt.Texts())
		}
	}

	h.u.chrome.navFilter = ""
	h.tt.Frame()
	h.want("Worktrees", "Branches", "Remotes", "Tags", "Stashes")
	for _, leaf := range []string{"tree", "v1"} {
		if _, ok := h.tt.Find(leaf); !ok {
			t.Fatalf("clearing filter hid %q in %q", leaf, h.tt.Texts())
		}
	}
}

func TestChromeNavFilterMatchesFolderName(t *testing.T) {
	h, _ := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	if err := h.tt.Click("Filter"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("FeAt")
	h.tt.Frame()
	h.want("Branches", "Remotes")
	for _, leaf := range []string{"feature", "tree", "api", "remote-only"} {
		if _, ok := h.tt.Find(leaf); !ok {
			t.Fatalf("folder filter hid %q in %q", leaf, h.tt.Texts())
		}
	}
	if _, ok := h.tt.Find("main, current branch"); ok {
		t.Fatal("unrelated current branch stayed visible")
	}
	if !h.tt.HasText("Current changes") || !h.tt.HasText("All commits") {
		t.Fatalf("filter hid the mode rows: %q", h.tt.Texts())
	}
}

func TestChromeNavTagsToggleWithoutCheckout(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.tt.SetFocused(true)
	h.click("Tags")
	h.press(ui.KeyLeft)
	if shown(h, "v1") {
		t.Fatal("closed tags still list v1")
	}
	h.press(ui.KeyRight)
	if !shown(h, "v1") {
		t.Fatal("right did not reopen tags")
	}
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("toggling tags checked out %s", got)
	}
}

func TestChromeNavMultipleWorktreesOpenOnActivate(t *testing.T) {
	hr, _ := chromeHarness(t, 1280, 800)
	main, linked := twoWorktrees(t, hr.Env)
	hr.open(main)
	hr.openBranchFolders()
	hr.want("Worktrees", filepath.Base(main), "linked", "Main", "side")
	if _, ok := hr.tt.Find("New Worktree…"); ok {
		t.Fatal("drew New Worktree")
	}
	if shown(hr, "Current") {
		t.Fatalf("drew a Current label in %q", hr.tt.Texts())
	}
	hr.click("linked")
	if hr.u.repo.Root() != main || hr.u.chrome.navSel != "worktree:"+linked {
		t.Fatalf("click opened %s (selected %s)", hr.u.repo.Root(), hr.u.chrome.navSel)
	}
	hr.click("linked")
	if hr.u.repo.Root() != linked || hr.u.snap.Branch != "side" {
		t.Fatalf("opened %s on %s", hr.u.repo.Root(), hr.u.snap.Branch)
	}
	hr.want(hr.u.t("currentWorktreeName", "name", "linked"))
}

func TestChromeNavRemoteAndTagDoNotCheckout(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	scope := histShown(h)
	h.click("remote-only")
	h.click("remote-only")
	h.tt.SetFocused(true)
	h.press(ui.KeyEnter)
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" || h.u.notice != nil {
		t.Fatalf("remote: on %s notice %+v", got, h.u.notice)
	}
	if _, ok := h.tt.Find("Actions for origin/feature/remote-only"); ok {
		t.Fatal("remote branch has a menu")
	}
	if h.u.middleTab != 0 || histShown(h) != scope {
		t.Fatalf("remote changed surface tab %d scope %v", h.u.middleTab, histShown(h))
	}

	h.click("v1")
	h.click("v1")
	h.press(ui.KeyEnter)
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("tag checked out %s", got)
	}
	if h.u.middleTab != 0 || histShown(h) != scope {
		t.Fatalf("tag changed surface tab %d scope %v", h.u.middleTab, histShown(h))
	}
}

func TestChromeNavFoldersFromTheKeyboard(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	h.tt.SetFocused(true)
	h.click("feature")
	h.press(ui.KeyLeft)
	if shown(h, "tree") || shown(h, "api") {
		t.Fatal("closed folder still lists its branches")
	}
	h.press(ui.KeyRight)
	if !shown(h, "ui") || !shown(h, "api") {
		t.Fatalf("right did not reopen feature: %q", h.tt.Texts())
	}
	h.press(ui.KeyRight)
	if h.u.chrome.navSel != "local:folder:feature/ui" {
		t.Fatalf("selected %q", h.u.chrome.navSel)
	}
	h.press(ui.KeyDown)
	if h.u.chrome.navSel != "local:feature/ui/tree" {
		t.Fatalf("selected %q", h.u.chrome.navSel)
	}
	h.press(ui.KeyLeft)
	h.press(ui.KeyLeft)
	if shown(h, "tree") {
		t.Fatal("ui folder still open")
	}
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("keyboard checked out %s", got)
	}
}

func TestChromeNavBusyBlocksCheckoutAndWorktree(t *testing.T) {
	h, dir := chromeBranchRepo(t, 1280, 800)
	h.openBranchFolders()
	h.tt.SetFocused(true)
	h.click("api")
	h.u.wt.bulk = "stage"
	h.press(ui.KeyEnter)
	h.click("api")
	h.u.wt.bulk = ""
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "main" {
		t.Fatalf("switched while busy: %s", got)
	}

	hr, _ := chromeHarness(t, 1280, 800)
	main, linked := twoWorktrees(t, hr.Env)
	hr.open(main)
	hr.openBranchFolders()
	hr.u.wt.bulk = "stage"
	hr.click("linked")
	hr.click("linked")
	hr.u.wt.bulk = ""
	if hr.u.repo.Root() != main {
		t.Fatalf("opened %s while busy", hr.u.repo.Root())
	}
	_ = linked
}

func TestChromeNavWorktreeKeepsSession(t *testing.T) {
	hr, _ := chromeHarness(t, 1280, 500)
	main, _ := twoWorktrees(t, hr.Env)
	gittest.Write(t, filepath.Join(main, "keep.txt"), "keep\n")
	hr.open(main)
	hr.openBranchFolders()
	hr.u.cm.message = "Main message"
	hr.selectFile(workflow.Untracked, "keep.txt")
	hr.click("linked")
	hr.click("linked")
	if hr.u.cm.message != "" || hr.u.wt.selected != nil {
		t.Fatal("main's session showed in linked")
	}
	hr.u.cm.message = "Linked message"
	hr.click("All commits")
	if hr.u.middleTab != 1 {
		t.Fatal("did not stay on All commits")
	}
	hr.click(filepath.Base(main))
	hr.click(filepath.Base(main))
	hr.tt.Frame()
	if hr.u.cm.message != "Main message" || hr.u.middleTab != 0 {
		t.Fatalf("main: %q tab %d", hr.u.cm.message, hr.u.middleTab)
	}
	if s := hr.u.wt.selected; s == nil || s.path != "keep.txt" {
		t.Fatalf("selection %+v", s)
	}

	os.Remove(filepath.Join(main, "keep.txt"))
	hr.click("linked")
	hr.click("linked")
	mainRow := filepath.Base(main)
	hr.click(mainRow)
	hr.click(mainRow)
	if hr.u.wt.selected != nil || hr.u.cm.message != "Main message" {
		t.Fatalf("stale selection %+v", hr.u.wt.selected)
	}
}

// openBranchFolders opens every folder of the Branches and Remotes trees. A
// folder starts closed unless it holds the checked-out branch, so a test that
// reaches a branch inside a folder opens that folder first.
func (h *harness) openBranchFolders() {
	h.T.Helper()
	if h.u.chrome.navClosed == nil {
		h.u.chrome.navClosed = map[string]bool{}
	}
	open := func(kind string, names []string) {
		for _, name := range names {
			parts := strings.Split(name, "/")
			for i := 1; i < len(parts); i++ {
				h.u.chrome.navClosed[kind+"folder:"+strings.Join(parts[:i], "/")] = false
			}
		}
	}
	var local, remote []string
	for _, b := range h.u.br.data.Local {
		local = append(local, b.Name)
	}
	for _, r := range h.u.br.data.Remote {
		remote = append(remote, r.Name)
	}
	open("local:", local)
	open("remote:", remote)
	for _, key := range []string{"group:worktrees", "group:remotes", "group:tags", "group:stashes"} {
		h.u.chrome.navClosed[key] = false
	}
	h.tt.Frame()
}

// histShown is what History draws: the one selected branch, or "" for All branches.
func histShown(h *harness) string {
	return fmt.Sprint(h.u.hist.shownRef)
}
