package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// typeMessage focuses the commit Summary and types text. In panel B the
// Summary is the message's first line; a value without a blank line is the
// whole message, so a short subject lands in u.cm.message unchanged.
func (h *harness) typeMessage(text string) {
	h.T.Helper()
	h.click("Summary")
	h.tt.Type(text)
	h.tt.Frame()
}

func TestManualCommitFromTheWindow(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "later.txt"), "later\n")
	h.open(dir)

	// Nothing staged, then an empty or blank message: Commit says why it
	// is unavailable before it is clicked, and clicking does nothing.
	h.want("Nothing staged yet")
	h.typeMessage("Written before staging")
	h.want("Nothing staged yet")
	if h.u.cm.message != "Written before staging" {
		t.Fatalf("message %q: typing is allowed before staging", h.u.cm.message)
	}
	h.click("Commit")
	if h.u.cm.hint != nil || h.u.cm.committing {
		t.Fatalf("a disabled Commit ran: %+v", h.u.cm)
	}
	h.click("Stage a.txt")
	h.u.cm.message = "  \n\t"
	h.tt.Frame()
	h.want("Add a message to commit")
	h.click("Commit")
	if h.u.cm.hint != nil {
		t.Fatalf("a disabled Commit ran: %+v", h.u.cm.hint)
	}
	if out := h.Git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("a commit was created")
	}
	h.u.cm.message = ""
	h.tt.Frame()

	h.typeMessage("Add a")
	// Panel B shows no ready-status line; a non-blank Summary enables the
	// commit (its ⌘↵ hint shows) and clears any earlier blocking hint.
	if h.u.commitBlock() != "" {
		t.Fatalf("commit still blocked: %q", h.u.commitBlock())
	}
	h.want("Commit")
	if h.tt.HasText("Add a message to commit") {
		t.Fatal("hint kept after editing the message")
	}
	snapshot(t, h.tt, "composer")
	h.click("Commit")
	if got := h.Git(dir, "log", "-1", "--format=%s"); got != "Add a\n" {
		t.Fatalf("subject %q", got)
	}
	if h.u.cm.message != "" || h.u.snap.Unborn() {
		t.Fatalf("after commit: message %q unborn %v", h.u.cm.message, h.u.snap.Unborn())
	}
	h.want("Committed "+h.u.cm.last.Short()+": Add a", "Untracked files", "later.txt")
}

func TestCommitFailureKeepsMessage(t *testing.T) {
	h := newHarness(t, 1280, 800)
	gittest.Write(t, h.Config, "[init]\n\tdefaultBranch = main\n")
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Git(dir, "add", "a.txt")
	h.open(dir)
	h.typeMessage("Keep this message")
	h.click("Commit")
	if h.u.cm.message != "Keep this message" {
		t.Fatalf("message lost: %q", h.u.cm.message)
	}
	found := false
	for _, s := range h.tt.Texts() {
		if strings.Contains(s, "git config --global user.name") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no identity guidance in %q", h.tt.Texts())
	}
}

func TestCommitIsNotRepeatedWhileRunning(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	g.Git(dir, "add", "a.txt")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	u.cm.message = "Once"
	tt.Frame()
	q.drain()
	tt.Click("Commit")
	tt.Click("Commit")
	if !u.cm.committing {
		t.Fatal("first Commit did not start")
	}
	q.drain()
	if n := strings.Count(g.Git(dir, "log", "--oneline"), "\n"); n != 1 {
		t.Fatalf("%d commits", n)
	}
}

func TestMessageBelongsToItsRepository(t *testing.T) {
	g := gittest.New(t)
	a, b := g.Repo(), g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	g.Git(a, "add", "a.txt")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain()
	u.cm.message = "Message for a"
	u.commit()
	u.OpenPath(b)
	q.run(1) // b opens while a's commit is pending
	if u.cm.message != "" || u.cm.committing {
		t.Fatalf("a's composer leaked into b: %+v", u.cm)
	}
	q.run(0) // a is closed, so its commit is refused and dropped
	if out := g.Git(a, "rev-list", "--all"); out != "" {
		t.Fatal("closed repository committed")
	}
	if u.cm.hint != nil || u.repo.Root() != b {
		t.Fatalf("stale result shown: %+v", u.cm)
	}
}

func TestTextTypedDuringCommitIsKept(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	g.Git(dir, "add", "a.txt")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	u.cm.message = "Add a"
	tt.Frame()
	if err := tt.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.commitBlock() != "busy" {
		t.Fatalf("block while committing %q", u.commitBlock())
	}
	u.cm.message = "Add a\n\nNext thoughts"
	q.drain()
	tt.Frame()
	if got := g.Git(dir, "log", "-1", "--format=%s"); got != "Add a\n" {
		t.Fatalf("subject %q", got)
	}
	if u.cm.message != "Add a\n\nNext thoughts" {
		t.Fatalf("new text lost: %q", u.cm.message)
	}
}

func TestMessageSurvivesNavigation(t *testing.T) {
	for _, width := range []int{1280, 600} {
		h := newHarness(t, width, 700)
		dir := h.Repo()
		gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
		h.Commit(dir, "Initial")
		gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
		h.open(dir)
		h.typeMessage("Keep me")
		h.showNav()
		h.click("All commits")
		h.click("Current changes")
		h.selectFile(workflow.Unstaged, "a.txt")
		if width < narrowWidth {
			h.click("Back")
		}
		if h.u.cm.message != "Keep me" {
			t.Fatalf("%d: message %q", width, h.u.cm.message)
		}
	}
}
