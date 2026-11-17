package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

// twoRepos makes repositories a and b with a change each.
func twoRepos(t *testing.T, g *gittest.Env) (a, b string) {
	t.Helper()
	a, b = g.Repo(), g.Repo()
	for _, d := range []string{a, b} {
		gittest.Write(t, filepath.Join(d, "f.txt"), "one\n")
		g.Commit(d, "Initial")
		gittest.Write(t, filepath.Join(d, "f.txt"), "two\n")
	}
	return a, b
}

// savedPrefs keeps what the window saves, as the app's store would.
type savedPrefs struct {
	p    prefs.Preferences
	fail bool
}

func (s *savedPrefs) save(p prefs.Preferences) error {
	if s.fail {
		return errors.New("disk full")
	}
	s.p = p
	return nil
}

func TestRecentSwitchCloseAndReopen(t *testing.T) {
	g := gittest.New(t)
	a, b := twoRepos(t, g)
	store := &savedPrefs{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, SavePreferences: store.save})
	h := &harness{Env: g, u: u, tt: ui.NewTester(window(u), 1280, 800)}

	// Open A from a subdirectory: the list records its root.
	os.MkdirAll(filepath.Join(a, "sub"), 0o755)
	h.open(filepath.Join(a, "sub"))
	h.u.cm.message = "words for A"
	h.selectFile(workflow.Unstaged, "f.txt")
	if len(store.p.Recent) != 1 || store.p.Recent[0] != a {
		t.Fatalf("recent after A: %q", store.p.Recent)
	}
	index := h.Git(a, "ls-files", "-s")

	h.open(b)
	h.click(filepath.Base(a))
	if h.u.repo == nil || h.u.repo.Root() != a || h.u.cm.message != "words for A" {
		t.Fatalf("back to A: %+v %q", h.u.repo, h.u.cm.message)
	}
	h.click(filepath.Base(b))
	if h.u.repo.Root() != b {
		t.Fatal("not B")
	}
	if store.p.Recent[0] != b || store.p.Recent[1] != a || len(store.p.Recent) != 2 {
		t.Fatalf("recent order %q", store.p.Recent)
	}

	h.u.CloseRepository()
	h.tt.Frame()
	if h.u.repo != nil || h.u.wf.Current() != nil || h.u.Title() != "GitOwl" {
		t.Fatal("still open after close")
	}
	h.want("No repository open", "Repositories", filepath.Base(a), filepath.Base(b))
	snapshot(t, h.tt, "welcome-recent")
	if h.Git(a, "ls-files", "-s") != index || h.Git(b, "status", "--porcelain") != " M f.txt\n" {
		t.Fatal("closing changed a repository")
	}

	h.click(filepath.Base(a))
	if h.u.repo == nil || h.u.repo.Root() != a || h.u.cm.message != "words for A" {
		t.Fatalf("reopen A: %q", h.u.cm.message)
	}
	if s := h.u.wt.selected; s == nil || s.path != "f.txt" {
		t.Fatalf("selection %+v", s)
	}

	// A new window with the saved preferences lists them, opens nothing.
	u2 := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, Preferences: store.p})
	tt2 := ui.NewTester(window(u2), 1280, 800)
	tt2.Frame()
	if u2.repo != nil || !tt2.HasText("Repositories") || !tt2.HasText(filepath.Base(a)) {
		t.Fatalf("restart: %q", tt2.Texts())
	}
	// Words are not kept across launches.
	if u2.cm.message != "" {
		t.Fatal("message kept across launches")
	}
}

func TestRecentMissingAndRemove(t *testing.T) {
	g := gittest.New(t)
	gone := filepath.Join(t.TempDir(), "gone")
	store := &savedPrefs{p: prefs.Default()}
	store.p.Recent = []string{gone}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, SavePreferences: store.save, Preferences: store.p})
	h := &harness{Env: g, u: u, tt: ui.NewTester(window(u), 1280, 800)}
	h.tt.Frame()
	h.tt.Frame()
	h.want("gone")
	h.click("gone")
	h.want("That location does not exist.")
	if len(h.u.prefs.Recent) != 1 {
		t.Fatal("a failed open changed the list")
	}
}

func TestRecentSaveFailureKeepsRepositoryOpen(t *testing.T) {
	g := gittest.New(t)
	a, _ := twoRepos(t, g)
	store := &savedPrefs{fail: true}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, SavePreferences: store.save})
	h := &harness{Env: g, u: u, tt: ui.NewTester(window(u), 1280, 800)}
	h.open(a)
	if h.u.repo == nil || len(h.u.prefs.Recent) != 1 {
		t.Fatal("open failed with the save")
	}
	if !announced(h.tt, "The recent list could not be saved. It works until GitOwl quits.") {
		t.Fatalf("announcements %q", h.tt.Announcements())
	}
}

func TestCloseDropsWorkInFlight(t *testing.T) {
	g := gittest.New(t)
	a, b := twoRepos(t, g)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain()
	// A refresh, a diff and an open of B are in flight when A closes.
	u.Refresh()
	u.selectRow(4)
	u.OpenPath(b)
	if u.closeBlock() != "" {
		t.Fatalf("close blocked by reads: %q", u.closeBlock())
	}
	u.CloseRepository()
	q.drain()
	if u.repo != nil || u.wf.Current() != nil || u.opening || u.wt.diff != nil {
		t.Fatalf("work after close reopened something: repo %v current %v opening %v diff %+v", u.repo, u.wf.Current(), u.opening, u.wt.diff)
	}
	if len(u.prefs.Recent) != 1 || u.prefs.Recent[0] != a {
		t.Fatalf("stale open recorded: %q", u.prefs.Recent)
	}
}

func TestCloseBlockedWhileWriting(t *testing.T) {
	g := gittest.New(t)
	a, _ := twoRepos(t, g)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain()
	u.bulkStage(false)
	if u.closeBlock() != "busy" {
		t.Fatal(u.closeBlock())
	}
	u.CloseRepository()
	if u.repo == nil {
		t.Fatal("closed during Stage All")
	}
	q.drain()
	u.CloseRepository()
	if u.repo != nil {
		t.Fatal("not closed afterwards")
	}
}

func TestSwitchFailureKeepsRepository(t *testing.T) {
	h := newHarness(t, 1280, 800)
	a, _ := twoRepos(t, h.Env)
	h.open(a)
	h.u.prefs.Recent = append(h.u.prefs.Recent, t.TempDir())
	h.u.openRecent(h.u.prefs.Recent[1])
	h.tt.Frame()
	if h.u.repo == nil || h.u.repo.Root() != a {
		t.Fatal("failed switch closed the repository")
	}
	h.want("That folder is not inside a Git repository.")
}

func TestWelcomeScrollsLongRecentList(t *testing.T) {
	g := gittest.New(t)
	p := prefs.Default()
	for i := range prefs.MaxRecent {
		p.Recent = append(p.Recent, filepath.Join(t.TempDir(), "project-"+string(rune('a'+i))))
	}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"zh-Hans"}, Go: func(fn func()) { fn() }, Preferences: p})
	tt := ui.NewTester(window(u), 560, 480)
	tt.Frame()
	tt.Frame()
	if !tt.HasText("仓库") {
		t.Fatalf("texts %q", tt.Texts())
	}
	snapshot(t, tt, "welcome-recent-small-zh")
	tt.SetDark(true)
	snapshot(t, tt, "welcome-recent-small-dark")
}
