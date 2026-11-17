package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// Native smoke tests start MyGo's runtime and a real, hidden window. They
// run only with GITOWL_NATIVE_SMOKE=1 in a desktop session:
//
//	GITOWL_NATIVE_SMOKE=1 go test ./internal/desktop -run '^TestNative' -count=1
func TestMain(m *testing.M) {
	if os.Getenv("GITOWL_NATIVE_SMOKE") != "1" {
		os.Exit(m.Run()) // Ordinary tests never open windows.
	}
	dir, err := os.MkdirTemp("", "gitowl-smoke-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mygo.App.SetName("GitOwl smoke")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory) // No Dock icon.
	mygo.App.SetPath(mygo.PathUserData, dir)
	mygo.App.OnWindowAllClosed(func() {})
	result := make(chan int, 1)
	mygo.App.WhenReady(func() { go func() { result <- m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := <-result
	os.RemoveAll(dir)
	os.Exit(code)
}

// onMain runs fn on the UI thread and waits for it.
func onMain(fn func()) {
	done := make(chan struct{})
	mygo.RunOnMain(func() { fn(); close(done) })
	<-done
}

// eventually polls cond on the UI thread until it holds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		ok := false
		onMain(func() { ok = cond() })
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestNativeWindowRunsTheWorkflow(t *testing.T) {
	if os.Getenv("GITOWL_NATIVE_SMOKE") != "1" {
		t.Skip("requires GITOWL_NATIVE_SMOKE=1 in a desktop session")
	}
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "left.txt"), "left\n")

	var win *mygo.Window
	var u *UI
	onMain(func() {
		u = New(Options{
			Workflow:  workflow.New(g.Runner),
			Preferred: mygo.App.PreferredLanguages(),
			Post:      func(fn func()) { win.Update(fn) },
		})
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: "GitOwl smoke", Width: 1280, Height: 800, Hidden: true,
			TitleBarStyle:        mygo.TitleBarHiddenInset,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
			Content: ui.View(func(c *ui.Context) {
				c.SetTheme(KitTheme(c))
				u.Chrome(c)
			}),
		})
		u.OpenPath(dir)
	})
	defer onMain(func() { win.Destroy() })

	eventually(t, "the repository to open", func() bool { return u.repo != nil && len(u.snap.Untracked) == 2 })
	onMain(func() { u.toggleStaged(u.snap.Untracked[0]) })
	eventually(t, "staging", func() bool { return len(u.snap.Staged) == 1 && u.wt.busy == nil })
	onMain(func() {
		u.cm.message = "Add a from the native window"
		u.commit()
	})
	eventually(t, "the commit", func() bool { return !u.snap.Unborn() && !u.cm.committing })
	eventually(t, "history", func() bool { return len(u.hist.commits) == 1 })
	if got := g.Git(dir, "log", "-1", "--format=%s"); strings.TrimSpace(got) != "Add a from the native window" {
		t.Fatalf("commit %q", got)
	}
	onMain(func() {
		if len(u.snap.Untracked) != 1 || u.snap.Untracked[0].Path != "left.txt" {
			t.Errorf("left over %+v", u.snap.Untracked)
		}
	})

	// Stage All, then a linked worktree made with Git, opened and left:
	// each keeps its own message.
	onMain(func() { u.bulkStage(false) })
	eventually(t, "Stage All", func() bool { return len(u.snap.Staged) == 1 && u.wt.bulk == "" })
	linked := filepath.Join(t.TempDir(), "linked")
	g.Git(dir, "worktree", "add", "-q", "-b", "side", linked)
	onMain(func() {
		u.cm.message = "Main words"
		u.Refresh()
	})
	eventually(t, "the worktree list", func() bool { return len(u.trees.list) == 2 })
	onMain(func() { u.openWorktree(u.trees.list[1]) })
	eventually(t, "the linked worktree", func() bool { return u.repo != nil && u.snap.Branch == "side" && len(u.trees.list) == 2 })
	onMain(func() { u.openWorktree(u.trees.list[0]) })
	eventually(t, "the return", func() bool { return u.snap.Branch == "main" && u.cm.message == "Main words" })
	if got := g.Git(linked, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("linked index %q", got)
	}

	// Next iteration: one hunk staged, a fetch from a file remote, closing
	// to the welcome screen and opening again with the words kept.
	g.Git(dir, "commit", "-q", "-m", "Left")
	var lines strings.Builder
	for i := 1; i <= 30; i++ {
		lines.WriteString("line\n")
	}
	gittest.Write(t, filepath.Join(dir, "f.txt"), lines.String())
	g.Commit(dir, "Add f")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "first\n"+strings.Repeat("line\n", 28)+"last\n")
	remote := filepath.Join(t.TempDir(), "remote.git")
	g.Git(dir, "clone", "-q", "--bare", dir, remote)
	g.Git(dir, "remote", "add", "origin", remote)
	onMain(func() { u.Refresh() })
	eventually(t, "the edit", func() bool { return len(u.snap.Unstaged) == 1 && u.sync.loaded })
	onMain(func() {
		u.wt.selected = &changeKey{area: workflow.Unstaged, path: "f.txt"}
		u.updateWorkingTree()
	})
	eventually(t, "the diff", func() bool { return u.wt.diff != nil && u.wt.diff.info.HunkCount() == 2 })
	onMain(func() { u.applyHunk(1) })
	eventually(t, "the hunk", func() bool { return len(u.snap.Staged) == 1 && u.wt.busy == nil })
	if got := g.Git(dir, "show", ":f.txt"); !strings.HasSuffix(got, "last\n") || strings.HasPrefix(got, "first") {
		t.Fatalf("index %q", got)
	}
	onMain(func() { u.fetch("origin") })
	eventually(t, "the fetch", func() bool { return u.sync.op == "" && !u.sync.info.FetchedAt.IsZero() })
	onMain(func() {
		u.cm.message = "Kept across close"
		u.CloseRepository()
	})
	eventually(t, "the welcome screen", func() bool { return u.repo == nil })
	onMain(func() { u.openRecent(dir) })
	eventually(t, "the reopen", func() bool { return u.repo != nil && u.cm.message == "Kept across close" })

	// UX mainstream: stage a hunk from the hunk bar's choice, commit it,
	// read it in History (opened on the newest commit and its first
	// file), publish and push it, and pull a colleague's commit; the sync
	// strip shows only while the working tree is not clean.
	eventually(t, "the reopened diff", func() bool { return len(u.snap.Staged) == 1 })
	onMain(func() {
		u.cm.message = "Stage the last line"
		u.commit()
	})
	eventually(t, "the hunk commit", func() bool { return !u.cm.committing && len(u.snap.Staged) == 0 })
	if got := g.Git(dir, "show", "HEAD:f.txt"); !strings.HasSuffix(got, "last\n") || strings.HasPrefix(got, "first") {
		t.Fatalf("committed %q", got)
	}
	// The hidden window draws no frames, so the step History's builder
	// takes each frame on entering it is taken while waiting.
	onMain(func() { u.middleTab = 1 })
	eventually(t, "History on the newest commit", func() bool {
		u.defaultCommit()
		return u.hist.detail != nil && u.hist.detail.Subject == "Stage the last line" && u.hist.diff != nil && u.hist.diff.key.path == "f.txt"
	})
	eventually(t, "the strip while f.txt is changed", func() bool {
		return u.sync.loaded && workflow.PullBlock(u.snap, u.sync.info) != ""
	})
	onMain(func() { u.openPublish() })
	eventually(t, "the publish dialog", func() bool { return u.sync.publishOpen && u.sync.publishRemote == "origin" })
	onMain(func() {
		u.sync.publishOpen = false
		u.push(workflow.PushTarget{Remote: "origin", Branch: "main"}, true)
	})
	eventually(t, "the push", func() bool { return u.sync.op == "" && u.snap.Upstream == "origin/main" })
	if got := g.Git(remote, "log", "-1", "--format=%s", "main"); strings.TrimSpace(got) != "Stage the last line" {
		t.Fatalf("remote has %q", got)
	}
	theirs := filepath.Join(t.TempDir(), "theirs")
	g.Git(dir, "clone", "-q", remote, theirs)
	gittest.Write(t, filepath.Join(theirs, "theirs.txt"), "t\n")
	g.Commit(theirs, "Their commit")
	g.Git(theirs, "push", "-q", "origin", "HEAD")
	g.Git(dir, "checkout", "--", "f.txt")
	onMain(func() { u.Refresh() })
	eventually(t, "a clean tree", func() bool { return u.snap.Clean() && workflow.PullBlock(u.snap, u.sync.info) == "" })
	onMain(func() { u.fetch("origin") })
	eventually(t, "the fetch", func() bool { return u.sync.op == "" && u.snap.Behind == 1 })
	onMain(func() { u.pull() })
	eventually(t, "the pull", func() bool { return u.sync.op == "" && u.snap.Behind == 0 })
	if got := g.Git(dir, "log", "-1", "--format=%s"); strings.TrimSpace(got) != "Their commit" || u.sync.err != nil {
		t.Fatalf("after pull %q, %+v", got, u.sync.err)
	}
	eventually(t, "History on their commit", func() bool { return len(u.hist.commits) > 0 && u.hist.commits[0].Subject == "Their commit" })
}
