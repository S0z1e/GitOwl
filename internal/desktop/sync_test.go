package desktop

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// clones makes a bare remote with one commit and two clones of it: mine,
// which the window opens, and theirs, which pushes as a colleague would.
func clones(t *testing.T, g *gittest.Env) (bare, mine, theirs string) {
	t.Helper()
	seed := g.Repo()
	gittest.Write(t, filepath.Join(seed, "a.txt"), "a\n")
	g.Commit(seed, "Initial")
	base, _ := filepath.EvalSymlinks(t.TempDir())
	bare, mine, theirs = filepath.Join(base, "remote.git"), filepath.Join(base, "mine"), filepath.Join(base, "theirs")
	g.Git(base, "clone", "-q", "--bare", seed, bare)
	g.Git(base, "clone", "-q", bare, mine)
	g.Git(base, "clone", "-q", bare, theirs)
	return bare, mine, theirs
}

func theyPush(t *testing.T, g *gittest.Env, theirs, file string) {
	t.Helper()
	gittest.Write(t, filepath.Join(theirs, file), file+"\n")
	g.Commit(theirs, "Their "+file)
	g.Git(theirs, "push", "-q", "origin", "HEAD")
}

func announced(tt *ui.Tester, text string) bool {
	return slices.ContainsFunc(tt.Announcements(), func(s string) bool { return strings.Contains(s, text) })
}

func TestFetchRunsAloneAndCancels(t *testing.T) {
	g := gittest.New(t)
	_, mine, _ := clones(t, g)
	gittest.Write(t, filepath.Join(mine, "a.txt"), "mine\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go,
		ChooseDirectory: func(string) (string, error) { t.Fatal("chose a repository during fetch"); return "", nil }})
	tt := ui.NewTester(window(u), 1280, 800)
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
	// Meanwhile nothing else changes the repository or leaves it.
	tt.Click("Fetch")
	tt.Click("Stage a.txt")
	u.chooseRepository()
	if u.sync.op != opFetch || u.stageAllBlock() != "busy" || u.commitBlock() != "busy" || !u.switchBlocked() {
		t.Fatalf("op %q jobs %d, stage all %q commit %q", u.sync.op, len(q.jobs), u.stageAllBlock(), u.commitBlock())
	}
	snapshot(t, tt, "fetching")
	if err := tt.Click("Fetching from origin… — Cancel"); err != nil {
		t.Fatalf("no cancel: %v (%q)", err, tt.Texts())
	}
	q.drain()
	tt.Frame()
	if u.sync.op != "" || u.notice != nil {
		t.Fatalf("after cancel: op %q notice %+v", u.sync.op, u.notice)
	}
	if !announced(tt, "Fetch cancelled") {
		t.Fatalf("no cancel note in %q", tt.Announcements())
	}
}

func TestFetchResultAfterSwitchIsDropped(t *testing.T) {
	g := gittest.New(t)
	_, mine, theirs := clones(t, g)
	other := g.Repo()
	theyPush(t, g, theirs, "b.txt")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(mine)
	q.drain()
	u.fetch("")
	u.OpenPath(other)
	q.run(1) // other opens first
	q.drain()
	if u.repo.Root() != other || u.sync.op != "" || u.snap.Behind != 0 {
		t.Fatalf("mine's fetch reached other: root %s op %q %+v", u.repo.Root(), u.sync.op, u.snap)
	}
}

func TestFetchWithoutRemoteExplains(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	if h.u.sync.info.DefaultFetch() != "" {
		t.Fatalf("unexpected remote %q", h.u.sync.info.DefaultFetch())
	}
	h.click("Fetch")
	if h.u.sync.op != "" || len(h.u.sync.info.Remotes) != 0 {
		t.Fatalf("fetch invented a remote: op %q remotes %v", h.u.sync.op, h.u.sync.info.Remotes)
	}
}

func TestPushUnavailableWhenDetached(t *testing.T) {
	h := newHarness(t, 1280, 800)
	_, mine, _ := clones(t, h.Env)
	h.Git(mine, "switch", "-q", "--detach")
	h.open(mine)
	h.tt.Frame()
	if h.u.pushBlock() != "errorPushDetached" {
		t.Fatal(h.u.pushBlock())
	}
	h.tt.Click("Push")
	h.tt.Frame()
	if h.u.sync.op != "" {
		t.Fatal("pushed a detached HEAD")
	}
}
