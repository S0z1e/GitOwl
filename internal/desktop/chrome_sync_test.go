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

func chromeClones(t *testing.T, w, h int) (*harness, string, string, string) {
	t.Helper()
	hr, _ := chromeHarness(t, w, h)
	bare, mine, theirs := clones(t, hr.Env)
	return hr, bare, mine, theirs
}

func TestChromeFetchRunsWithoutOpeningAPopover(t *testing.T) {
	h, _, mine, theirs := chromeClones(t, 1280, 800)
	theyPush(t, h.Env, theirs, "b.txt")
	h.open(mine)
	h.tt.Frame()
	h.click("Fetch")
	if h.tt.HasText("Merge") || h.tt.HasText("Autostash local changes") || h.tt.HasText("Add remote") {
		t.Fatalf("Fetch opened a popover: %q", h.tt.Texts())
	}
	if h.u.snap.Behind != 1 || h.u.sync.op != "" {
		t.Fatalf("after fetch: behind %d op %q", h.u.snap.Behind, h.u.sync.op)
	}
	if !announced(h.tt, "Fetched from origin") {
		t.Fatalf("no confirmation in %q", h.tt.Announcements())
	}
}

func TestChromePullOpensPopoverWhenUpToDate(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	if h.u.snap.HasAheadBehind && (h.u.snap.Ahead != 0 || h.u.snap.Behind != 0) {
		t.Fatalf("not even: %+v", h.u.snap)
	}
	h.click("Pull")
	if !h.tt.HasText("Merge") || !h.tt.HasText("Rebase") || !h.tt.HasText("Fast-forward only") {
		t.Fatalf("Pull did not open a popover: %q", h.tt.Texts())
	}
	if h.u.sync.op != "" {
		t.Fatal("Pull ran instead of opening a popover")
	}
	reason := h.u.t("pullUpToDate", "branch", "main", "upstream", "origin/main")
	if !h.tt.HasText(reason) {
		t.Fatalf("disabled reason missing in %q", h.tt.Texts())
	}
	if err := h.tt.Click(reason); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.u.sync.op != "" {
		t.Fatal("disabled Pull ran")
	}
}

func TestChromePushOpensPopoverWhenUpToDate(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.click("Push")
	if !h.tt.HasText("Set as upstream for main") || !h.tt.HasText("Push tags") || !h.tt.HasText("Force with lease") {
		t.Fatalf("Push did not open a popover: %q", h.tt.Texts())
	}
	if h.u.sync.op != "" {
		t.Fatal("Push ran instead of opening a popover")
	}
	reason := h.u.t("pushUpToDate", "target", "origin/main")
	if !h.tt.HasText(reason) {
		t.Fatalf("disabled reason missing in %q", h.tt.Texts())
	}
}

func TestChromeNarrowFetchPullPushKeepTheSameActions(t *testing.T) {
	h, _, mine, theirs := chromeClones(t, 720, 800)
	theyPush(t, h.Env, theirs, "b.txt")
	h.open(mine)
	h.tt.Frame()
	h.click("Fetch")
	if h.tt.HasText("Merge") {
		t.Fatalf("narrow Fetch opened a popover: %q", h.tt.Texts())
	}
	if h.u.snap.Behind != 1 {
		t.Fatalf("narrow Fetch behind %d", h.u.snap.Behind)
	}
	h.click("Pull")
	if !h.tt.HasText("Merge") || !h.tt.HasText("Autostash local changes") {
		t.Fatalf("narrow Pull did not open: %q", h.tt.Texts())
	}
}

func TestChromePullPopoverPreselectsUpstreamAndDefaults(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.click("Pull")
	p := h.u.sync.pop
	if p.kind != "pull" || p.remote != "origin" || p.branch != "main" {
		t.Fatalf("preselect %+v", p)
	}
	if p.strategy != workflow.PullMerge {
		t.Fatalf("strategy %v", p.strategy)
	}
	if !p.autostash {
		t.Fatal("Autostash must be set true; Go's zero value is false")
	}
	if p.prune {
		t.Fatal("Prune defaulted on")
	}
	h.want("origin", "Merge", "Rebase", "Fast-forward only", "Autostash local changes", "Prune deleted remote branches")
}

func TestChromePullMovesIncomingCommits(t *testing.T) {
	h, _, mine, theirs := chromeClones(t, 1280, 800)
	theyPush(t, h.Env, theirs, "b.txt")
	h.open(mine)
	h.click("Fetch")
	h.click("Pull")
	if !h.tt.HasText("2 commits behind origin/main") && !h.tt.HasText("1 commit behind origin/main") {
		t.Fatalf("behind count missing in %q", h.tt.Texts())
	}
	run := h.u.t("pullRun", "target", "origin/main")
	if err := h.tt.Click(run); err != nil {
		t.Fatalf("run pull: %v (%q)", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.Git(mine, "rev-parse", "HEAD") != h.Git(theirs, "rev-parse", "HEAD") {
		t.Fatal("not pulled")
	}
	if !announced(h.tt, "Pulled 1 commit into main from origin/main") {
		t.Fatalf("announcements %q", h.tt.Announcements())
	}
}

func TestChromePushMovesOutgoingCommits(t *testing.T) {
	h, bare, mine, _ := chromeClones(t, 1280, 800)
	gittest.Write(t, filepath.Join(mine, "a.txt"), "mine\n")
	h.Commit(mine, "Mine")
	h.open(mine)
	h.tt.Frame()
	h.click("Push")
	if !h.tt.HasText("↑1") && !h.tt.HasText("1 commit ahead of origin/main") {
		t.Fatalf("ahead missing in %q", h.tt.Texts())
	}
	run := h.u.t("pushTo", "branch", "main", "target", "origin/main")
	if err := h.tt.Click(run); err != nil {
		t.Fatalf("run push: %v (%q)", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.Git(bare, "rev-parse", "main") != h.Git(mine, "rev-parse", "HEAD") {
		t.Fatal("not pushed")
	}
	if !announced(h.tt, "Pushed main to origin/main") {
		t.Fatalf("announcements %q", h.tt.Announcements())
	}
}

func TestChromePushAheadCountOnlyForUpstream(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	gittest.Write(t, filepath.Join(mine, "a.txt"), "mine\n")
	h.Commit(mine, "Mine")
	h.open(mine)
	h.tt.Frame()
	h.click("Push")
	if !h.tt.HasText("↑1") {
		t.Fatalf("upstream push hid ↑1 in %q", h.tt.Texts())
	}
	h.click("New branch on origin…")
	h.tt.Frame()
	h.tt.Type("topic")
	h.tt.Frame()
	h.click("Create Branch")
	if h.u.sync.pop.branch != "topic" {
		t.Fatalf("selected %q", h.u.sync.pop.branch)
	}
	if h.tt.HasText("Tracking · ↑1") {
		t.Fatalf("tracking ↑1 shown for a non-upstream target: %q", h.tt.Texts())
	}
}

func TestChromeNewBranchOnOriginSelectsNameWithoutPrefill(t *testing.T) {
	h, bare, mine, _ := chromeClones(t, 1280, 800)
	gittest.Write(t, filepath.Join(mine, "a.txt"), "mine\n")
	h.Commit(mine, "Mine")
	h.open(mine)
	h.tt.Frame()
	h.click("Push")
	before := h.u.sync.pop.branch
	h.click("New branch on origin…")
	h.tt.Frame()
	if h.u.sync.pop.name == h.u.snap.Branch && h.u.snap.Branch != "" {
		t.Fatal("name field filled with the local branch")
	}
	h.want("This name becomes the remote branch on origin. Push creates it.", "Branch name", "Cancel", "Create Branch")
	h.click("Cancel")
	if h.u.sync.pop.branch != before {
		t.Fatalf("cancel changed selection to %q", h.u.sync.pop.branch)
	}

	h.click("New branch on origin…")
	h.tt.Frame()
	h.tt.Type("feature")
	h.tt.Frame()
	h.click("Create Branch")
	if h.u.sync.pop.branch != "feature" || !h.u.sync.pop.newBranch {
		t.Fatalf("confirm %+v", h.u.sync.pop)
	}
	if strings.Contains(h.Git(bare, "show-ref"), "refs/heads/feature") {
		t.Fatal("created the remote branch before Push")
	}
	run := h.u.t("pushTo", "branch", "main", "target", "origin/feature")
	if err := h.tt.Click(run); err != nil {
		t.Fatalf("push new branch: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.Git(bare, "rev-parse", "feature") != h.Git(mine, "rev-parse", "HEAD") {
		t.Fatal("Push did not create the remote branch")
	}
}

func TestChromeAddRemoteAppearsInThePopover(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.click("Push")
	h.click("Add remote")
	h.tt.Frame()
	h.want("Adds a remote to this repository.", "Remote name", "URL", "Cancel", "Add Remote")
	h.tt.Type("fork")
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	other := h.Repo()
	h.tt.Type(other)
	h.tt.Frame()
	h.click("Add Remote")
	if !h.u.sync.info.Has("fork") {
		t.Fatalf("remotes %+v", h.u.sync.info.Remotes)
	}
	if !h.tt.HasText("fork") {
		t.Fatalf("fork missing from remote row: %q", h.tt.Texts())
	}
	if h.u.sync.pop.remote != "fork" {
		t.Fatalf("selected remote %q", h.u.sync.pop.remote)
	}
}

func TestChromeAddRemoteRedactsCredentials(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.click("Pull")
	h.click("Add remote")
	h.tt.Frame()
	h.tt.Type("secret")
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	h.tt.Type("https://user:s3cret@example.invalid/r.git")
	h.tt.Frame()
	h.click("Add Remote")
	joined := strings.Join(h.tt.Texts(), "\n")
	if strings.Contains(joined, "s3cret") {
		t.Fatalf("credential on screen: %q", h.tt.Texts())
	}
}

func TestChromeNewBranchDialogKeepsTabAndReturnsFocus(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.click("Push")
	if err := h.tt.Click("New branch on origin…"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	seen := h.cycleInDialog("New branch on origin")
	if !seen["Branch name"] || !seen["Cancel"] || !seen["Create Branch"] {
		t.Fatalf("dialog controls %v", seen)
	}
	h.closeAndBack("New branch on origin", "New branch on origin…")
}

func TestChromeAddRemoteDialogKeepsTabAndReturnsFocus(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.click("Pull")
	if err := h.tt.Click("Add remote"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	seen := h.cycleInDialog("Add remote")
	if !seen["Remote name"] || !seen["URL"] || !seen["Cancel"] || !seen["Add Remote"] {
		t.Fatalf("dialog controls %v", seen)
	}
	h.closeAndBack("Add remote", "Add remote")
}

func TestChromeFetchShowsCancelAndToasts(t *testing.T) {
	g := gittest.New(t)
	_, mine, _ := clones(t, g)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(chromeWindow(u), 1280, 800)
	u.OpenPath(mine)
	q.drain()
	tt.Frame()
	if err := tt.Click("Fetch"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.sync.op != opFetch || !u.writing() {
		t.Fatalf("op %q", u.sync.op)
	}
	if err := tt.Click("Fetching from origin… — Cancel"); err != nil {
		t.Fatalf("no cancel: %v (%q)", err, tt.Texts())
	}
	q.drain()
	tt.Frame()
	if u.sync.op != "" {
		t.Fatalf("after cancel: op %q", u.sync.op)
	}
	if !announced(tt, "Fetch cancelled") {
		t.Fatalf("no cancel note in %q", tt.Announcements())
	}
}

func TestChromePullUsesAutostashOnADirtyWorktree(t *testing.T) {
	h, _, mine, theirs := chromeClones(t, 1280, 800)
	theyPush(t, h.Env, theirs, "b.txt")
	gittest.Write(t, filepath.Join(mine, "a.txt"), "dirty\n")
	h.open(mine)
	h.click("Fetch")
	h.click("Pull")
	if !h.u.sync.pop.autostash {
		t.Fatal("popover did not set Autostash")
	}
	run := h.u.t("pullRun", "target", "origin/main")
	if err := h.tt.Click(run); err != nil {
		t.Fatalf("dirty pull: %v %q", err, h.tt.Texts())
	}
	h.tt.Frame()
	if h.Git(mine, "rev-parse", "HEAD") != h.Git(theirs, "rev-parse", "HEAD") {
		t.Fatal("autostash pull did not move HEAD")
	}
	if got := h.Git(mine, "status", "--porcelain"); !strings.Contains(got, "a.txt") {
		t.Fatalf("autostash dropped local edits: %q", got)
	}
}
