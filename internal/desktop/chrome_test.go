package desktop

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// chromeHarness is the ticket 02 seam: a desktop UI tester over the custom
// shell, not the Onevra window().
func chromeHarness(t *testing.T, w, h int) (*harness, *savedPrefs) {
	t.Helper()
	g := gittest.New(t)
	store := &savedPrefs{}
	u := New(Options{
		Workflow:        workflow.New(g.Runner),
		Preferred:       []string{"en-US"},
		Go:              func(fn func()) { fn() },
		SavePreferences: store.save,
	})
	return &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), w, h)}, store
}

func TestChromeWelcomeShowsEmptyState(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.tt.Frame()

	for _, want := range []string{
		"No repository open",
		"Pick a repository from the left, or open one from disk to review its changes, history and branches.",
		"Open Repository…",
		"or drop a folder anywhere in this window",
		"Pinned",
		"Repositories",
		"Search",
		"⌘F",
		"Settings",
	} {
		if !h.tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, h.tt.Texts())
		}
	}
	if _, ok := h.tt.Find("GitOwl brand"); !ok {
		t.Fatalf("owl missing in %q", h.tt.Texts())
	}
	for _, s := range []string{"Commit", "Current changes", "All commits", "Close Repository and Return to Welcome"} {
		if h.tt.HasText(s) {
			t.Fatalf("welcome shows %q in %q", s, h.tt.Texts())
		}
	}
	if h.u.repo != nil {
		t.Fatal("welcome selected a repository")
	}
}

func TestChromeRepositoryRowsShowNameOnly(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.u.prefs.Recent = []string{"/tmp/alpha", "/tmp/beta"}
	h.tt.Frame()
	h.want("alpha", "beta", "Pin alpha", "Pin beta")
	for _, s := range []string{"/tmp/alpha", "/tmp/beta", "main", "Clean", "Remove alpha from Recent"} {
		if h.tt.HasText(s) {
			t.Fatalf("row shows extra %q in %q", s, h.tt.Texts())
		}
	}
	owl, ok := h.tt.Find("GitOwl brand")
	if !ok {
		t.Fatal("welcome left the middle")
	}
	row, ok := h.tt.Find("alpha")
	if !ok {
		t.Fatal("alpha row missing")
	}
	if row.X >= owl.X {
		t.Fatalf("repository column is not on the left: row x=%v owl x=%v", row.X, owl.X)
	}
}

func TestChromeClickRecentOpensRepository(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	a, _ := twoRepos(t, h.Env)
	h.u.prefs.Recent = []string{a}
	h.tt.Frame()
	h.click(filepath.Base(a))
	if h.u.repo == nil || h.u.repo.Root() != a {
		t.Fatalf("click did not open: %+v (%+v)", h.u.repo, h.u.notice)
	}
	if len(h.u.prefs.Recent) == 0 || h.u.prefs.Recent[0] != a {
		t.Fatalf("recent after click: %q", h.u.prefs.Recent)
	}
}

func TestChromeDropFolderOpensRepository(t *testing.T) {
	h, store := chromeHarness(t, 1280, 800)
	h.tt.Frame()
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "Initial")
	if !h.tt.DropFiles(640, 400, dir) {
		t.Fatal("chrome did not take the dropped folder")
	}
	h.tt.Frame()
	if h.u.repo == nil || h.u.repo.Root() != dir {
		t.Fatalf("drop did not open: %+v (%+v)", h.u.repo, h.u.notice)
	}
	if len(store.p.Recent) == 0 || store.p.Recent[0] != dir {
		t.Fatalf("recent after drop: %q", store.p.Recent)
	}
}

func TestChromeOpenRepositoryFromDisk(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	g.Commit(dir, "Initial")
	u := New(Options{
		Workflow:        workflow.New(g.Runner),
		Preferred:       []string{"en-US"},
		Go:              func(fn func()) { fn() },
		ChooseDirectory: func(string) (string, error) { return dir, nil },
	})
	tt := ui.NewTester(chromeWindow(u), 1280, 800)
	tt.Frame()
	if err := tt.Click("Open Repository…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.repo == nil || u.repo.Root() != dir {
		t.Fatalf("open from disk: %+v (%+v)", u.repo, u.notice)
	}
}

func TestChromePinSurvivesPreferenceReload(t *testing.T) {
	h, store := chromeHarness(t, 1280, 800)
	a, b := twoRepos(t, h.Env)
	h.u.prefs.Recent = []string{a, b}
	h.tt.Frame()
	h.click(filepath.Base(a))
	if h.u.repo == nil || h.u.repo.Root() != a {
		t.Fatal("setup open A")
	}
	h.click("Pin " + filepath.Base(b))
	if h.u.repo.Root() != a {
		t.Fatal("pin switched the current repository")
	}
	if len(store.p.Pinned) != 1 || store.p.Pinned[0] != b {
		t.Fatalf("pinned after pin: %q", store.p.Pinned)
	}

	u2 := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, Preferences: store.p})
	tt2 := ui.NewTester(chromeWindow(u2), 1280, 800)
	tt2.Frame()
	if !tt2.HasText("Unpin "+filepath.Base(b)) || u2.repo != nil {
		t.Fatalf("reload lost pin: repo=%v texts=%q pinned=%q", u2.repo, tt2.Texts(), u2.prefs.Pinned)
	}
	if err := tt2.Click("Unpin " + filepath.Base(b)); err != nil {
		t.Fatal(err)
	}
	tt2.Frame()
	if len(u2.prefs.Pinned) != 0 {
		t.Fatalf("unpin left %q", u2.prefs.Pinned)
	}
}

func TestChromeSearchFiltersPinnedAndRepositories(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.u.prefs.Pinned = []string{"/tmp/alpha"}
	h.u.prefs.Recent = []string{"/tmp/beta", "/tmp/gamma"}
	h.tt.Frame()
	if err := h.tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("ZZ")
	h.tt.Frame()
	if h.tt.HasText("alpha") || h.tt.HasText("beta") || h.tt.HasText("Pinned") || h.tt.HasText("Repositories") {
		t.Fatalf("search left rows: %q", h.tt.Texts())
	}
	if !h.tt.HasText("Search") || !h.tt.HasText("⌘F") {
		t.Fatalf("search field gone: %q", h.tt.Texts())
	}

	h.u.chrome.query = ""
	h.tt.Frame()
	h.want("alpha", "beta", "gamma", "Pinned", "Repositories")

	if err := h.tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("AlP")
	h.tt.Frame()
	if !h.tt.HasText("alpha") || h.tt.HasText("beta") || h.tt.HasText("Repositories") {
		t.Fatalf("case-insensitive filter: %q", h.tt.Texts())
	}

	h.u.chrome.query = ""
	h.tt.Frame()
	h.tt.Key(ui.Cmd, ui.KeyF)
	h.tt.Frame()
	if h.u.chrome.query != "" {
		t.Fatalf("printed ⌘F ran a shortcut: %q", h.u.chrome.query)
	}
}

func TestChromeShowMoreExpandsHiddenRepositories(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	var list []string
	for i := range chromeRepoVisible + 3 {
		list = append(list, filepath.Join("/tmp", "repo"+strconv.Itoa(i)))
	}
	h.u.prefs.Recent = list
	h.tt.Frame()
	h.want("repo0", "repo6", "Show 3 more")
	if h.tt.HasText("repo7") {
		t.Fatalf("hidden row visible: %q", h.tt.Texts())
	}
	h.click("Show 3 more")
	h.tt.Frame()
	h.want("repo7")

	if err := h.tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("repo10")
	h.tt.Frame()
	if !h.tt.HasText("repo10") || h.tt.HasText("repo0") {
		t.Fatalf("filter after show more: %q", h.tt.Texts())
	}
}

func TestChromeArtboardSizesAndTrafficLightInset(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.u.prefs.Recent = []string{"/tmp/alpha"}
	h.tt.Frame()

	owl, ok := h.tt.Find("GitOwl brand")
	if !ok || !near(owl.W, chromeWelcomeOwl, 1) || !near(owl.H, chromeWelcomeOwl, 1) {
		t.Fatalf("owl %v×%v, want %d×%d", owl.W, owl.H, chromeWelcomeOwl, chromeWelcomeOwl)
	}
	open, ok := h.tt.Find("Open Repository…")
	if !ok || !near(open.H, chromeOpenHeight, 2) {
		t.Fatalf("open height %v, want %d", open.H, chromeOpenHeight)
	}
	row, ok := h.tt.Find("alpha")
	if !ok || !near(row.H, chromeRowHeight, 2) {
		t.Fatalf("row height %v, want %d", row.H, chromeRowHeight)
	}
	if row.X > chromeRailWidth {
		t.Fatalf("row x %v is outside the %d rail", row.X, chromeRailWidth)
	}
	title, ok := h.tt.Find("GitOwl")
	if !ok || title.X < chromeRailWidth {
		t.Fatalf("toolbar title x %v, want inset after the %d rail", title.X, chromeRailWidth)
	}
	if title.Y > chromeToolbarHeight {
		t.Fatalf("toolbar title y %v, want inside the %d toolbar", title.Y, chromeToolbarHeight)
	}
}

func TestChromeGearOpensSettingsWithoutCloseControl(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	h.tt.Frame()
	settings, ok := h.tt.Find("Settings")
	if !ok {
		t.Fatal("settings gear missing")
	}
	if settings.X > 200 {
		t.Fatalf("settings is not in the repository column: x=%v", settings.X)
	}
	h.click("Settings")
	if !h.u.set.open {
		t.Fatal("gear did not open Settings")
	}
	if h.tt.HasText("Close Repository and Return to Welcome") {
		t.Fatal("chrome has a close control")
	}
}

func TestChromeOpenedRepositoryStillShowsDiff(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "two\n")
	h.u.prefs.Recent = []string{dir}
	h.tt.Frame()
	h.click(filepath.Base(dir))
	h.want("a.txt", "Unstaged changes")
	if h.u.wt.diff == nil {
		t.Fatalf("opened review has no diff: %q", h.tt.Texts())
	}
	h.want("two")
	if h.tt.HasText("Close Repository and Return to Welcome") {
		t.Fatal("opened chrome has a close control")
	}
	if h.tt.HasText("No repository open") {
		t.Fatal("welcome stayed after open")
	}
}
