package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/launch"
	"gitowl/internal/workflow"
)

func TestPaletteOpensEmptyWithFullList(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	h.key(ui.Cmd, ui.KeyK)
	if h.u.chrome.pal.query != "" {
		t.Fatalf("field %q, want empty", h.u.chrome.pal.query)
	}
	h.want(
		"Suggestions",
		"Commit 0 staged files",
		"Push commits", "Pull commits",
		"Open in editor", "Open in Terminal", "Reveal in Finder", "Settings…",
		"Stash all changes", "Discard unstaged changes",
	)
	for _, s := range []string{"Open Recent…", "Switch Worktree…"} {
		if h.tt.HasText(s) {
			t.Fatalf("palette offered a picker %q in %q", s, h.tt.Texts())
		}
	}
}

func TestPaletteCommitQueryHidesOtherGroups(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	h.key(ui.Cmd, ui.KeyK)
	h.tt.Type("Commit")
	h.tt.Frame()
	if !strings.EqualFold(h.u.chrome.pal.query, "Commit") {
		t.Fatalf("query %q", h.u.chrome.pal.query)
	}
	h.want("Suggestions", "Commit 0 staged files", "Push commits", "Pull commits")
	for _, s := range []string{"Open in editor", "Open in Terminal", "Reveal in Finder", "Settings…", "Stash all changes", "Discard unstaged changes"} {
		if h.tt.HasText(s) {
			t.Fatalf("query Commit still shows %q in %q", s, h.tt.Texts())
		}
	}
}

func TestPaletteEnterRunsHighlightedAndEscapeCloses(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	h.key(ui.Cmd, ui.KeyK)
	h.press(ui.KeyEscape)
	if h.u.chrome.pal.open || h.tt.HasText("Push commits") {
		t.Fatalf("Esc did not close: open %v texts %q", h.u.chrome.pal.open, h.tt.Texts())
	}

	chromeStage(h, dir, "a.txt", "a\n")
	h.typeMessage("From palette")
	h.key(ui.Cmd, ui.KeyK)
	h.want("Commit 1 staged file")
	h.press(ui.KeyEnter)
	if h.u.chrome.pal.open {
		t.Fatal("Enter left the palette open")
	}
	if got := h.Git(dir, "log", "-1", "--format=%s"); got != "From palette\n" {
		t.Fatalf("Enter did not commit: %q", got)
	}
}

func TestShortcutCommitDoesNothingWhenDisabled(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	before := strings.TrimSpace(h.Git(dir, "rev-list", "--count", "HEAD"))
	h.key(ui.Cmd, ui.KeyEnter)
	if h.u.chrome.pal.open {
		t.Fatal("⌘↵ opened the palette")
	}
	if got := strings.TrimSpace(h.Git(dir, "rev-list", "--count", "HEAD")); got != before {
		t.Fatalf("⌘↵ committed while disabled: count %s", got)
	}
	if h.u.cm.hint != nil || h.u.cm.committing {
		t.Fatalf("disabled commit ran: %+v", h.u.cm)
	}
}

func TestPaletteDiscardRestoresUnstagedKeepsStaged(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Git(dir, "add", "f.txt")
	h.Commit(dir, "Seed")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "staged\n")
	h.Git(dir, "add", "f.txt")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "staged\nunwanted\n")
	gittest.Write(t, filepath.Join(dir, "other.txt"), "other\n")
	h.Git(dir, "add", "other.txt")
	gittest.Write(t, filepath.Join(dir, "other.txt"), "other\nedit\n")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "keep\n")
	h.u.Refresh()
	h.tt.Frame()
	h.key(ui.Cmd, ui.KeyK)
	h.want("Discard unstaged changes", "2 files")
	if h.tt.HasText("⌘S") {
		t.Fatalf("Discard showed a shortcut in %q", h.tt.Texts())
	}
	h.click("Discard unstaged changes")
	if h.u.chrome.pal.open || h.u.discard.open {
		t.Fatal("Discard asked for confirmation")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != "staged\n" {
		t.Fatalf("f.txt %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "other.txt")); string(data) != "other\n" {
		t.Fatalf("other.txt %q", data)
	}
	if h.Git(dir, "diff", "--cached", "--name-only") != "f.txt\nother.txt\n" &&
		h.Git(dir, "diff", "--cached", "--name-only") != "other.txt\nf.txt\n" {
		t.Fatalf("staged lost: %q", h.Git(dir, "diff", "--cached", "--name-only"))
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "new.txt")); string(data) != "keep\n" {
		t.Fatal("untracked file touched")
	}
}

func TestPaletteShortcutsAndSharedCan(t *testing.T) {
	h, l, dir := chromeLaunchRepo(t, 1280, 800)
	h.key(ui.Cmd|ui.Shift, ui.KeyP)
	if !h.u.sync.pop.pushOpen || h.u.pick.open {
		t.Fatalf("⇧⌘P: push %v picker %v", h.u.sync.pop.pushOpen, h.u.pick.open)
	}
	h.want("Set as upstream for main")
	h.u.sync.pop = syncPopover{}
	h.tt.Frame()

	h.key(ui.Cmd|ui.Shift, ui.KeyL)
	if !h.u.sync.pop.pullOpen {
		t.Fatal("⇧⌘L did not open Pull")
	}
	h.want("Merge")
	h.u.sync.pop = syncPopover{}
	h.tt.Frame()

	h.key(ui.Cmd, ui.KeyComma)
	if !h.u.set.open {
		t.Fatal("⌘, did not open Settings")
	}
	h.u.set.open = false
	h.tt.Frame()

	h.key(ui.Cmd, ui.KeyE)
	if !h.u.set.open || h.u.set.state.Category != "general" {
		t.Fatal("⌘E without an editor did not open Settings")
	}
	h.u.set.open = false
	h.u.prefs.Editor.Name, h.u.prefs.Editor.Path = "Zed", "/Applications/Zed.app"
	h.tt.Frame()
	h.key(ui.Cmd, ui.KeyE)
	if r := l.got[len(l.got)-1]; r != (launch.Request{Tool: launch.Editor, App: "/Applications/Zed.app", Path: dir}) {
		t.Fatalf("editor %+v", l.got)
	}

	h.key(ui.Cmd|ui.Alt, ui.KeyT)
	h.key(ui.Cmd|ui.Shift, ui.KeyR)
	if len(l.got) < 3 || l.got[len(l.got)-2] != (launch.Request{Tool: launch.Terminal, Path: dir}) ||
		l.got[len(l.got)-1] != (launch.Request{Tool: launch.Finder, Path: dir}) {
		t.Fatalf("terminal/finder %+v", l.got)
	}

	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.u.Refresh()
	h.tt.Frame()
	h.key(ui.Cmd, ui.KeyK)
	h.want("⌘↵", "⇧⌘P", "⇧⌘L", "⌘E", "⌥⌘T", "⇧⌘R", "⌘,")
	h.click("Stash all changes")
	if !h.u.snap.Clean() {
		t.Fatal("Stash all changes is not the toolbar Stash")
	}
}

func TestChromePrintedKeysAndOldShortcutsDoNotRun(t *testing.T) {
	g := gittest.New(t)
	chose := 0
	u := New(Options{
		Workflow:        workflow.New(g.Runner),
		Preferred:       []string{"en-US"},
		Go:              func(fn func()) { fn() },
		ChooseDirectory: func(string) (string, error) { chose++; return "", nil },
	})
	h := &harness{Env: g, u: u, tt: ui.NewTester(chromeWindow(u), 1280, 800)}
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	h.Commit(dir, "Initial")
	h.open(dir)

	h.key(ui.Cmd, ui.KeyO)
	if chose != 0 {
		t.Fatal("⌘O ran Open Repository")
	}
	h.key(ui.Cmd, ui.KeyF)
	if h.u.chrome.query != "" || h.u.chrome.pal.open {
		t.Fatal("⌘F ran a shortcut")
	}
	h.key(ui.Cmd|ui.Shift, ui.KeyW)
	if h.u.repo == nil {
		t.Fatal("⇧⌘W closed the repository")
	}
	h.key(ui.Cmd|ui.Shift, ui.KeyO)
	if h.u.pick.open {
		t.Fatal("⇧⌘O opened the recent picker")
	}
	h.key(ui.Cmd, ui.KeyR)
	// Refresh is not a chrome shortcut; it must not be the only bound R.
	h.key(ui.Cmd|ui.Shift, ui.KeyP)
	if h.u.pick.open {
		t.Fatal("⇧⌘P opened the worktree picker")
	}
}

func TestPaletteOneKeyPressRunsOnce(t *testing.T) {
	h, l, _ := chromeLaunchRepo(t, 1280, 800)
	h.u.Do(ActionTerminal)
	h.u.Do(ActionTerminal)
	h.tt.Frame()
	if len(l.got) != 1 {
		t.Fatalf("Terminal ran %d times", len(l.got))
	}
	h.u.lastActionAt = time.Time{}
	h.u.Do(ActionTerminal)
	h.u.Do(ActionTerminal)
	h.tt.Frame()
	if len(l.got) != 2 {
		t.Fatalf("after the repeat window: %d", len(l.got))
	}
}

func TestPalettePushPullSettingsFromRows(t *testing.T) {
	h, _, mine, _ := chromeClones(t, 1280, 800)
	h.open(mine)
	h.tt.Frame()
	h.key(ui.Cmd, ui.KeyK)
	h.click("Push commits")
	if !h.u.sync.pop.pushOpen || h.u.chrome.pal.open {
		t.Fatal("Push commits did not open the toolbar popover")
	}
	h.u.sync.pop = syncPopover{}
	h.tt.Frame()
	h.key(ui.Cmd, ui.KeyK)
	h.click("Pull commits")
	if !h.u.sync.pop.pullOpen {
		t.Fatal("Pull commits did not open the toolbar popover")
	}
	h.u.sync.pop = syncPopover{}
	h.tt.Frame()
	h.key(ui.Cmd, ui.KeyK)
	h.click("Settings…")
	if !h.u.set.open || h.u.chrome.pal.open {
		t.Fatal("Settings… did not open Settings")
	}
}
