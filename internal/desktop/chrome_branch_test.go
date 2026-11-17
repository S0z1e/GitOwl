package desktop

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func chromeTopicRepo(t *testing.T) (*harness, string) {
	t.Helper()
	h, dir := chromeRepo(t, 1280, 800)
	h.Git(dir, "branch", "topic")
	h.u.Refresh()
	h.tt.Frame()
	return h, dir
}

func (h *harness) openBranchMenu(name string) []string {
	h.T.Helper()
	if err := h.tt.RightClick(name); err != nil {
		h.T.Fatalf("open %q: %v (texts %q)", name, err, h.tt.Texts())
	}
	return h.tt.Menu()
}

func (h *harness) chooseBranchMenu(name, item string) {
	h.T.Helper()
	h.openBranchMenu(name)
	if err := h.tt.ChooseMenuItem(item); err != nil {
		h.T.Fatalf("%v (menu %q)", err, h.tt.Menu())
	}
	h.tt.Frame()
}

func TestChromeBranchMenuGroupsInDesignOrder(t *testing.T) {
	h, _ := chromeTopicRepo(t)
	got := h.openBranchMenu("topic")
	want := []string{
		"topic",
		"-",
		"Checkout",
		"-",
		"New branch from here",
		"New tag",
		"Open pull request",
		"-",
		"Merge topic into main",
		"Rebase main onto topic",
		"Compare",
		"-",
		"Push",
		"Pull",
		"-",
		"Rename",
		"Delete branch…",
		"Copy branch name",
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

func TestChromeBranchMenuCheckoutSwitches(t *testing.T) {
	h, dir := chromeTopicRepo(t)
	h.chooseBranchMenu("topic", "Checkout")
	if got := strings.TrimSpace(h.Git(dir, "branch", "--show-current")); got != "topic" || h.u.snap.Branch != "topic" {
		t.Fatalf("checkout: git %s window %s", got, h.u.snap.Branch)
	}
}

func TestChromeBranchMenuNewBranchFromHere(t *testing.T) {
	h, dir := chromeTopicRepo(t)
	h.click("topic")
	h.chooseBranchMenu("topic", "New branch from here")
	h.want("New Branch", "Starts at this branch: topic.", "Branch name", "Cancel", "Create Branch")
	h.tt.Type("from-topic")
	h.tt.Frame()
	h.click("Create Branch")
	h.tt.Frame()
	if !strings.Contains(h.Git(dir, "branch"), "from-topic") {
		t.Fatalf("branch missing: %s", h.Git(dir, "branch"))
	}
}

func TestChromeBranchMenuNewTagReusesDialog(t *testing.T) {
	h, dir := chromeTopicRepo(t)
	h.openBranchFolders()
	h.chooseBranchMenu("topic", "New tag")
	h.want("New Branch", "Tags this branch: topic.", "Tag name", "Create Tag")
	if h.tt.HasText("Annotation") || h.tt.HasText("Message") {
		t.Fatalf("tag dialog grew an annotation field: %q", h.tt.Texts())
	}
	h.tt.Type("v-topic")
	h.tt.Frame()
	h.click("Create Tag")
	h.tt.Frame()
	if !strings.Contains(h.Git(dir, "tag"), "v-topic") {
		t.Fatalf("tag missing: %s", h.Git(dir, "tag"))
	}
	h.want("v-topic")
}

func TestChromeBranchMenuRenamePrefillsName(t *testing.T) {
	h, dir := chromeTopicRepo(t)
	h.chooseBranchMenu("topic", "Rename")
	h.want("Renames topic.", "Branch name", "Rename Branch")
	if h.u.br.form.name != "topic" {
		t.Fatalf("prefill %q", h.u.br.form.name)
	}
	h.u.br.form.name = "renamed"
	h.click("Rename Branch")
	h.tt.Frame()
	if !strings.Contains(h.Git(dir, "branch"), "renamed") || strings.Contains(h.Git(dir, "branch"), "topic") {
		t.Fatalf("rename: %s", h.Git(dir, "branch"))
	}
}

func TestChromeBranchMenuDeleteConfirms(t *testing.T) {
	h, dir := chromeTopicRepo(t)
	h.chooseBranchMenu("topic", "Delete branch…")
	h.want("Delete branch topic?", h.u.t("deleteBranchBody"), "Cancel", "Delete")
	h.click("Delete")
	h.tt.Frame()
	if strings.Contains(h.Git(dir, "branch"), "topic") {
		t.Fatalf("topic stayed: %s", h.Git(dir, "branch"))
	}
}

func TestChromeBranchMenuDialogKeepsTabAndReturnsFocus(t *testing.T) {
	h, _ := chromeTopicRepo(t)
	h.click("topic")
	h.chooseBranchMenu("topic", "New branch from here")
	h.tt.Advance(time.Second)
	h.tt.Frame()
	seen := h.cycleInDialog("New Branch")
	if !seen["Branch name"] || !seen["Cancel"] || !seen["Create Branch"] {
		t.Fatalf("dialog controls %v", seen)
	}
	h.closeAndBack("New Branch", "topic")
}

func TestChromeBranchMenuMergeConflictShowsOnCurrentChanges(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "base\n")
	h.Commit(dir, "Base")
	h.Git(dir, "switch", "-q", "-c", "feature")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "feature\n")
	h.Commit(dir, "Feature")
	h.Git(dir, "switch", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "main\n")
	h.Commit(dir, "Main")
	h.u.Refresh()
	h.tt.Frame()
	h.chooseBranchMenu("feature", "Merge feature into main")
	h.want("Merge feature into main?", h.u.t("mergeBody"), "Merge")
	h.click("Merge")
	h.tt.Frame()
	if h.u.middleTab != 0 {
		t.Fatalf("conflict left tab %d", h.u.middleTab)
	}
	h.want("Conflicted")
	h.click("Pull")
	if !h.tt.HasText("Merge") || !h.tt.HasText("Rebase") || !h.tt.HasText("Fast-forward only") {
		t.Fatalf("popover Merge/Rebase left the pull strategies: %q", h.tt.Texts())
	}
}

func TestChromeBranchMenuPullDoesNotOpenPopover(t *testing.T) {
	h, _, mine, theirs := chromeClones(t, 1280, 800)
	theyPush(t, h.Env, theirs, "b.txt")
	h.open(mine)
	h.tt.Frame()
	h.click("Fetch")
	current := h.u.t("currentBranchName", "branch", "main")
	h.chooseBranchMenu(current, "Pull")
	if h.tt.HasText("Autostash local changes") || h.tt.HasText("Fast-forward only") {
		t.Fatalf("menu Pull opened the popover: %q", h.tt.Texts())
	}
	if h.Git(mine, "rev-parse", "HEAD") != h.Git(theirs, "rev-parse", "HEAD") {
		t.Fatal("menu Pull did not move HEAD")
	}
}

func TestChromeBranchMenuPushDoesNotOpenPopover(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	gittest.Write(t, filepath.Join(mine, "ahead.txt"), "a\n")
	h.Commit(mine, "Ahead")
	h.u.Refresh()
	h.tt.Frame()
	current := h.u.t("currentBranchName", "branch", "main")
	h.chooseBranchMenu(current, "Push")
	if h.tt.HasText("Set as upstream for main") || h.tt.HasText("Force with lease") {
		t.Fatalf("menu Push opened the popover: %q", h.tt.Texts())
	}
	if strings.TrimSpace(h.Git(mine, "rev-parse", "HEAD")) != strings.TrimSpace(h.Git(mine, "rev-parse", "origin/main")) {
		t.Fatal("menu Push did not update the remote")
	}
}

func TestChromeBranchMenuPullPushDisabledWhenNothingToMove(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	current := h.u.t("currentBranchName", "branch", "main")
	h.openBranchMenu(current)
	if err := h.tt.ChooseMenuItem("Pull"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Pull should be disabled when even: %v (menu %q)", err, h.tt.Menu())
	}
	h.openBranchMenu(current)
	if err := h.tt.ChooseMenuItem("Push"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("Push should be disabled when even: %v (menu %q)", err, h.tt.Menu())
	}
	if h.u.sync.pop.pullOpen || h.u.sync.pop.pushOpen {
		t.Fatal("disabled Sync opened a popover")
	}
}

func TestChromeBranchMenuCompareReplacesMainAndLeavesFromNav(t *testing.T) {
	h, dir := chromeTopicRepo(t)
	h.Git(dir, "switch", "-q", "topic")
	gittest.Write(t, filepath.Join(dir, "extra.txt"), "extra\n")
	h.Commit(dir, "On topic")
	h.Git(dir, "switch", "-q", "main")
	h.u.Refresh()
	h.tt.Frame()
	h.chooseBranchMenu("topic", "Compare")
	h.want("Compare main and topic", "Unified", "Split", "Wrap lines", "extra.txt")
	if _, ok := h.tt.Find("Stage extra.txt"); ok {
		t.Fatal("compare drew a staging checkbox")
	}
	if h.tt.HasText("Generate Draft") {
		t.Fatal("compare kept the commit box")
	}
	h.click("Current changes")
	if h.u.cmp.open {
		t.Fatal("Current changes did not leave compare")
	}
	if !h.tt.HasText("Filter files") {
		t.Fatalf("did not return to Current changes: %q", h.tt.Texts())
	}
}

func TestChromeBranchMenuOpenPullRequestUsesPopoverBase(t *testing.T) {
	var opened string
	g := gittest.New(t)
	u := New(Options{
		Workflow:  workflow.New(g.Runner),
		Preferred: []string{"en-US"},
		Go:        func(fn func()) { fn() },
		OpenURL:   func(url string) error { opened = url; return nil },
	})
	h := &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), 1280, 800)}
	_, mine, _ := clones(t, g)
	h.Git(mine, "branch", "topic")
	h.Git(mine, "remote", "set-url", "origin", "https://github.com/acme/gitowl.git")
	h.open(mine)
	h.tt.Frame()
	h.chooseBranchMenu("topic", "Open pull request")
	if opened != "https://github.com/acme/gitowl/compare/main...topic" {
		t.Fatalf("opened %q", opened)
	}
	if h.tt.HasText("Host") || h.tt.HasText("Title") {
		t.Fatalf("opened a pull-request form: %q", h.tt.Texts())
	}
}

func TestChromeBranchMenuCopyNameWritesClipboard(t *testing.T) {
	h, _ := chromeTopicRepo(t)
	h.chooseBranchMenu("topic", "Copy branch name")
	if got := h.tt.Clipboard(); got != "topic" {
		t.Fatalf("clipboard %q", got)
	}
}
