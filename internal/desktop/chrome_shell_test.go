package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/launch"
	"gitowl/internal/prefs"
	"gitowl/internal/workflow"
)

func chromeRepo(t *testing.T, w, h int) (*harness, string) {
	t.Helper()
	hr, _ := chromeHarness(t, w, h)
	dir := hr.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	hr.Commit(dir, "Initial")
	hr.open(dir)
	return hr, dir
}

func chromeLaunchRepo(t *testing.T, w, h int) (*harness, *launcher, string) {
	t.Helper()
	g := gittest.New(t)
	l := &launcher{}
	u := New(Options{
		Workflow:  workflow.New(g.Runner),
		Preferred: []string{"en-US"},
		Go:        func(fn func()) { fn() },
		Launch:    l.launch,
	})
	hr := &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), w, h)}
	dir := hr.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	hr.Commit(dir, "Initial")
	hr.open(dir)
	return hr, l, dir
}

func TestChromeToolbarOrderAfterOpen(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	name := filepath.Base(dir)
	path := homeShort(dir)
	h.want(name, "main", path, "Fetch", "Pull", "Push", "Stash", "Editor", "Terminal", "Finder",
		"Current changes", "All commits")
	for _, s := range []string{"Switch Repository…", "Open In"} {
		if h.tt.HasText(s) {
			t.Fatalf("toolbar has extra control %q in %q", s, h.tt.Texts())
		}
	}
	settings, ok := h.tt.Find("Settings")
	if !ok {
		t.Fatal("settings gear missing")
	}
	if settings.X > float32(chromeRailWidth) {
		t.Fatalf("settings left the repository column: x=%v", settings.X)
	}
	fetch, ok := h.tt.Find("Fetch")
	if !ok || fetch.X < float32(chromeRailWidth) {
		t.Fatalf("Fetch x=%v, want after the %d rail", fetch.X, chromeRailWidth)
	}
	if fetch.Y > float32(chromeToolbarHeight) {
		t.Fatalf("Fetch y=%v, want inside the %d toolbar", fetch.Y, chromeToolbarHeight)
	}
	orderLeftToRight(t, h.tt, "main", path, "Fetch", "Pull", "Push", "Stash", "Editor", "Terminal", "Finder")
}

func TestChromeBranchCapsuleDoesNotSwitch(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.Git(dir, "branch", "feature")
	h.u.Refresh()
	h.tt.Frame()
	if err := h.tt.Click("main"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if h.u.snap.Branch != "main" {
		t.Fatalf("capsule switched branch to %q", h.u.snap.Branch)
	}
	if items := h.tt.Menu(); len(items) > 0 {
		t.Fatalf("capsule opened a menu %q", items)
	}
}

func TestChromeUnbornSaysNoCommitsYet(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.open(h.Repo())
	h.want("No commits yet", "main")
}

func TestChromeToolbarButtonsShareHeight(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	var height float32
	for _, label := range []string{"Fetch", "Pull", "Push", "Stash", "Editor", "Terminal", "Finder"} {
		r, ok := h.tt.Find(label)
		if !ok {
			t.Fatalf("missing %q in %q", label, h.tt.Texts())
		}
		if height == 0 {
			height = r.H
		}
		if r.H != height {
			t.Fatalf("%s height %v, want shared %v", label, r.H, height)
		}
	}
}

func TestChromeNarrowFetchPullPushAreIconsWithLabels(t *testing.T) {
	wide, _ := chromeRepo(t, 1280, 800)
	wf, ok := wide.tt.Find("Fetch")
	if !ok {
		t.Fatal("wide Fetch missing")
	}
	narrow, _ := chromeRepo(t, 720, 800)
	nf, ok := narrow.tt.Find("Fetch")
	if !ok {
		t.Fatal("narrow Fetch missing")
	}
	if nf.W >= wf.W {
		t.Fatalf("narrow Fetch width %v, want smaller than wide %v", nf.W, wf.W)
	}
	if !near(nf.W, kitToolbarH, 2) || !near(nf.H, kitToolbarH, 2) {
		t.Fatalf("narrow Fetch %v×%v, want icon %d×%d", nf.W, nf.H, kitToolbarH, kitToolbarH)
	}
	for _, label := range []string{"Pull", "Push"} {
		r, ok := narrow.tt.Find(label)
		if !ok {
			t.Fatalf("narrow %s missing in %q", label, narrow.tt.Texts())
		}
		if !near(r.W, kitToolbarH, 2) {
			t.Fatalf("narrow %s width %v, want icon %d", label, r.W, kitToolbarH)
		}
	}
}

func TestChromeNavigationSwitchesSurfaces(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	if h.u.middleTab != 0 {
		t.Fatalf("opened on tab %d, want Current changes", h.u.middleTab)
	}
	h.click("All commits")
	if h.u.middleTab != 1 {
		t.Fatalf("All commits left tab %d", h.u.middleTab)
	}
	h.want("Initial")
	h.click("Current changes")
	if h.u.middleTab != 0 {
		t.Fatalf("Current changes left tab %d", h.u.middleTab)
	}
	gittest.Write(t, filepath.Join(h.u.repo.Root(), "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	if h.u.middleTab != 0 {
		t.Fatalf("refresh switched to tab %d", h.u.middleTab)
	}
}

func TestChromeSidebarToggleHidesRepositories(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	pin := "Pin " + filepath.Base(dir)
	if !h.tt.HasText(pin) {
		t.Fatalf("repository row missing before the toggle: %q", h.tt.Texts())
	}
	nav, ok := h.tt.Find("Current changes")
	if !ok {
		t.Fatal("navigation missing")
	}
	if nav.X < float32(chromeRailWidth) {
		t.Fatalf("navigation x=%v, want beside the %d rail", nav.X, chromeRailWidth)
	}
	toggle, ok := h.tt.Find(sidebarHideLabel(h.u))
	if !ok {
		t.Fatal("sidebar toggle missing")
	}
	if toggle.X > float32(chromeRailWidth) || toggle.Y > float32(chromeToolbarHeight) {
		t.Fatalf("toggle at %v, want repository column top", toggle)
	}
	h.click(sidebarHideLabel(h.u))
	h.tt.Advance(time.Second) // the rail slides out first
	if h.tt.HasText(pin) {
		t.Fatalf("repository rail stayed after hide: %q", h.tt.Texts())
	}
	if !h.tt.HasText("Current changes") || !h.tt.HasText("All commits") {
		t.Fatalf("navigation hid with the repositories: %q", h.tt.Texts())
	}
	// The toggle moves to the toolbar, clear of the traffic lights.
	show, ok := h.tt.Find(sidebarShowLabel(h.u))
	if !ok {
		t.Fatalf("toolbar toggle did not offer to show repositories: %q", h.tt.Texts())
	}
	if show.X < float32(chromeTrafficInset) || show.Y > float32(chromeToolbarHeight) {
		t.Fatalf("toolbar toggle at %v, want beside the traffic lights in the toolbar", show)
	}
	h.click(sidebarShowLabel(h.u))
	h.tt.Advance(time.Second)
	if !h.tt.HasText(pin) || !h.tt.HasText("Current changes") {
		t.Fatalf("repository rail did not return: %q", h.tt.Texts())
	}
}

func TestChromeCompactNavigationIsADrawer(t *testing.T) {
	h, dir := chromeRepo(t, 1024, 800)
	if h.tt.HasText("Current changes") {
		t.Fatalf("compact window showed navigation inline: %q", h.tt.Texts())
	}
	h.click(sidebarShowLabel(h.u))
	h.tt.Advance(time.Second) // the drawer slides in first
	h.want("Current changes", "All commits")
	h.click("All commits")
	if h.u.middleTab != 1 {
		t.Fatal("drawer All commits did not switch")
	}

	h.u.notice = &notice{code: workflow.CodeGitFailed}
	h.click(sidebarHideLabel(h.u))
	msg := h.u.t("errorGitFailed")
	r, ok := h.tt.Find(msg)
	if !ok {
		t.Fatalf("hidden-nav notice missing in %q", h.tt.Texts())
	}
	if r.X < float32(chromeRailWidth) {
		t.Fatalf("notice x=%v, want above the middle column after the rail", r.X)
	}

	wide := ui.NewTester(chromeWindow(h.u), 1280, 800)
	wide.Frame()
	if !wide.HasText("Current changes") {
		t.Fatalf("wide window hid navigation: %q", wide.Texts())
	}
	_ = dir
}

func TestChromeEditorTerminalFinderActOnWorktree(t *testing.T) {
	h, l, dir := chromeLaunchRepo(t, 1280, 800)
	h.click("Terminal")
	h.click("Finder")
	if len(l.got) != 2 || l.got[0] != (launch.Request{Tool: launch.Terminal, Path: dir}) ||
		l.got[1] != (launch.Request{Tool: launch.Finder, Path: dir}) {
		t.Fatalf("requests %+v", l.got)
	}
	h.click("Editor")
	if len(l.got) != 2 || !h.u.set.open || h.u.set.state.Category != "general" {
		t.Fatalf("no editor: %+v open %v", l.got, h.u.set.open)
	}

	h.u.set.open = false
	h.u.prefs.Editor = prefs.Editor{Name: "Zed", Path: "/Applications/Zed.app"}
	h.u.lastActionAt = time.Time{}
	h.tt.Frame()
	h.click("Editor")
	if r := l.got[len(l.got)-1]; r != (launch.Request{Tool: launch.Editor, App: "/Applications/Zed.app", Path: dir}) {
		t.Fatalf("editor request %+v", r)
	}
}

func TestChromeRefreshOnWindowFocus(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.click("Current changes")
	gittest.Write(t, filepath.Join(dir, "outside.txt"), "out\n")
	h.tt.SetFocused(false)
	h.tt.Frame()
	if h.tt.HasText("outside.txt") {
		t.Fatal("read the repository while unfocused")
	}
	h.tt.SetFocused(true)
	h.tt.Frame()
	h.want("outside.txt")
}

func TestChromeHomePathUsesTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	got := homeShort(filepath.Join(home, "Documents", "demo"))
	want := filepath.Join("~", "Documents", "demo")
	if got != want {
		t.Fatalf("homeShort = %q, want %q", got, want)
	}
}

func orderLeftToRight(t *testing.T, tt *ui.Tester, labels ...string) {
	t.Helper()
	var prev ui.Rect
	for i, s := range labels {
		r, ok := tt.Find(s)
		if !ok {
			t.Fatalf("missing %q in %q", s, tt.Texts())
		}
		if i > 0 && r.X+1 < prev.X {
			t.Fatalf("%q x=%.0f before %q x=%.0f", s, r.X, labels[i-1], prev.X)
		}
		prev = r
	}
}
