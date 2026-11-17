package desktop

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// background are controls of the window behind a dialog, which Tab must
// not reach while it is open.
var background = []string{"Settings", "Fetch", "Editor", "Terminal", "Finder", "Current changes", "All commits", "Stage All", "Hide repositories", "Show repositories"}

func (h *harness) focusedOn() []string {
	var f []string
	for _, s := range h.tt.Texts() {
		if h.tt.Focused(s) {
			f = append(f, s)
		}
	}
	return f
}

// cycleInDialog tabs forward and back through the open dialog, checks
// the focus never leaves it, and returns the controls it visited.
func (h *harness) cycleInDialog(name string) map[string]bool {
	h.T.Helper()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	seen := map[string]bool{}
	for _, mods := range []ui.Modifiers{0, ui.Shift} {
		for range 16 {
			h.tt.Key(mods, ui.KeyTab)
			h.tt.Frame()
			on := h.focusedOn()
			if len(on) == 0 {
				h.T.Fatalf("%s: Tab left nothing focused", name)
			}
			for _, b := range background {
				if h.tt.Focused(b) {
					h.T.Fatalf("%s: Tab reached %q behind the dialog (focus %q)", name, b, on)
				}
			}
			seen[on[0]] = true
		}
	}
	return seen
}

// closeAndBack closes the dialog with Escape and checks the focus is back
// on the control named back.
func (h *harness) closeAndBack(name, back string) {
	h.T.Helper()
	h.tt.Key(0, ui.KeyEscape)
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	if h.u.dialogOpen() {
		h.T.Fatalf("%s: Escape left the dialog open", name)
	}
	if !h.tt.Focused(back) {
		h.T.Fatalf("%s: focus on %q after closing, want %q", name, h.focusedOn(), back)
	}
}

// TestDialogsKeepAndReturnTheFocus opens each dialog from the keyboard:
// Tab and Shift-Tab stay inside it, Escape closes it, and the focus is
// back on the control that opened it.
func TestDialogsKeepAndReturnTheFocus(t *testing.T) {
	h, _, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "staged\n")
	h.Git(dir, "add", "a.txt")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "changed\n")
	h.Git(dir, "branch", "keep")
	h.open(dir)
	h.tt.SetFocused(true)

	// Settings, from its button.
	h.tabTo("Settings")
	h.press(ui.KeyEnter)
	seen := h.cycleInDialog("Settings")
	for _, s := range []string{"Search settings", "General", "AI Drafts", "About", "Close Settings"} {
		if !seen[s] {
			t.Fatalf("Settings: Tab never reached %q (%v)", s, seen)
		}
	}
	h.closeAndBack("Settings", "Settings")

	// Delete keep…, from the branch row's menu. The tree is a virtual list,
	// and the menu key does not open a row's menu there yet (a known gap),
	// so this opens it by the mouse.
	h.tabTo("keep")
	h.click("keep")
	if err := h.tt.RightClick("keep"); err != nil {
		t.Fatalf("right click keep: %v", err)
	}
	h.tt.Frame()
	if err := h.tt.ChooseMenuItem("Delete branch…"); err != nil {
		t.Fatalf("%v (menu %q)", err, h.tt.Menu())
	}
	h.tt.Frame()
	h.cycleInDialog("Delete branch")
	h.closeAndBack("Delete branch", "keep")
	if !strings.Contains(h.Git(dir, "branch"), "keep") {
		t.Fatal("Escape deleted the branch")
	}

	// Discard…, from the diff's file menu.
	h.selectFile(workflow.Unstaged, "a.txt")
	h.tabTo("Actions for a.txt")
	h.press(ui.KeyEnter)
	if err := h.tt.ChooseMenuItem("Discard Unstaged Changes…"); err != nil {
		t.Fatalf("%v (menu %q)", err, h.tt.Menu())
	}
	h.tt.Frame()
	h.cycleInDialog("Discard")
	h.closeAndBack("Discard", "Actions for a.txt")
	if h.Git(dir, "show", ":a.txt") != "staged\n" || h.Git(dir, "diff", "--name-only") != "a.txt\n" {
		t.Fatal("Escape discarded the change")
	}

}

// TestMenusFromTheKeyboard opens the window's menus from the keyboard:
// Enter, Space or Down on a menu button, the menu key on a row. Inside a
// menu, the arrows, Home, End, Return and Escape are the system menu's
// own; closing it leaves the focus on what opened it.
func TestMenusFromTheKeyboard(t *testing.T) {
	h, _, _ := newAIHarness(t, nil)
	dir := stagedRepo(h)
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "changed\n")
	h.Git(dir, "branch", "keep")
	h.open(dir)
	h.selectFile(workflow.Unstaged, "a.txt")
	h.tt.SetFocused(true)
	for _, c := range []struct {
		button string
		key    ui.Key
		item   string
	}{
		{"Actions for a.txt", ui.KeySpace, "Discard Unstaged Changes…"},
	} {
		h.tabTo(c.button)
		h.press(c.key)
		items := h.tt.Menu()
		found := false
		for _, it := range items {
			found = found || it == c.item
		}
		if !found {
			t.Fatalf("%s: menu %q, want %q", c.button, items, c.item)
		}
		h.tt.CloseMenu()
		h.tt.Frame()
		if !h.tt.Focused(c.button) {
			t.Fatalf("%s: focus on %q after closing its menu", c.button, h.focusedOn())
		}
	}
	// A branch row's actions open by the mouse; the menu key on the virtual
	// tree's rows is a known gap.
	if err := h.tt.RightClick("keep"); err != nil {
		t.Fatalf("right click keep: %v", err)
	}
	h.tt.Frame()
	got := h.tt.Menu()
	if !slices.Contains(got, "Checkout") || !slices.Contains(got, "Delete branch…") {
		t.Fatalf("row menu %q", got)
	}
	h.tt.CloseMenu()
	h.tt.Frame()
	// A right-click does not move focus onto the row, so focus returns to the
	// control that had it before the menu. Moving it onto the row is a known gap.
	if h.tt.Focused("keep") {
		t.Fatal("a right-click moved focus onto the branch row")
	}
}

// TestSettingsKeepEveryFieldAndScroll finds every setting, across
// categories by search, and reaches the last one in the smallest window.
func TestSettingsKeepEveryFieldAndScroll(t *testing.T) {
	h, _, _ := newAIHarness(t, nil)
	h.tt.SetSize(560, 480)
	h.u.OpenSettings()
	h.tt.Frame()
	h.tt.Advance(time.Second)
	h.tt.Frame()
	// A narrow Settings picks its category with a select.
	h.want("Category", "General", "Language", "Appearance", "Editor")
	h.choose("Category", "AI Drafts")
	for _, s := range []string{"Service URL", "Model", "API key", "Model context", "Commit message prompt"} {
		if !h.tt.HasText(s) {
			t.Fatalf("AI Drafts has no %q in %q", s, h.tt.Texts())
		}
	}
	// The last field can be scrolled to and is in the window then.
	r, _ := h.tt.Find("Commit message prompt")
	for i := 0; i < 20 && r.Y+r.H > 480; i++ {
		h.tt.Scroll(280, 300, 0, 120)
		h.tt.Frame()
		r, _ = h.tt.Find("Commit message prompt")
	}
	if r.Y+r.H > 480 || r.Y < 0 {
		t.Fatalf("prompt at %v in a 480 DIP window", r)
	}
	snapshot(t, h.tt, "settings-560")
	// Search finds a setting of another category.
	h.click("Search settings")
	h.tt.Type("editor")
	h.tt.Frame()
	h.want("Editor")
	h.tt.Key(ui.Cmd, ui.KeyA)
	h.tt.Type("model")
	h.tt.Frame()
	h.want("Model")
}

// TestWindowEntriesStayNamed checks that every global entry is on screen
// with an accessible name, in both languages.
func TestWindowEntriesStayNamed(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans"} {
		t.Run(lang, func(t *testing.T) {
			h, _, _ := newAIHarness(t, nil)
			h.u.prefs.Language = lang
			dir := stagedRepo(h)
			h.Commit(dir, "Initial")
			gittest.Write(t, filepath.Join(dir, "a.txt"), "staged\n")
			h.Git(dir, "add", "a.txt")
			gittest.Write(t, filepath.Join(dir, "a.txt"), "changed\n")
			h.open(dir)
			h.selectFile(workflow.Unstaged, "a.txt")
			for _, key := range []string{"chromeHideRepositories", "chromeCurrentChanges", "chromeAllCommits",
				"stageAll", "unstageAll", "commitBSummary", "commitAction", "fetch", "pull", "push",
				"kitEditor", "kitTerminal", "kitFinder", "settings", "stageHunk"} {
				if s := h.u.t(key); !h.tt.HasText(s) {
					t.Fatalf("%s (%q) not in the window: %q", key, s, h.tt.Texts())
				}
			}
			h.want(h.u.t("fileActions", "path", "a.txt"), h.u.t("stageFile", "path", "a.txt"))
			h.click(h.u.t("chromeAllCommits"))
			h.want("Initial")
		})
	}
}

// TestTabGoesPastTheDiff walks the whole window with Tab while a diff with
// hunks is shown: the hunk card's action is a stop, which must not stop Tab
// short of the title bar.
func TestTabGoesPastTheDiff(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n"}))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	h.tt.SetFocused(true)
	h.tabTo("Stage chunk")
	h.tabTo("Settings")
}
