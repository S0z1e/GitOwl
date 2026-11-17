package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// inList reports whether the keyboard focus is in the changes list.
func (h *harness) inList() bool {
	for _, r := range h.u.wt.rows {
		if !r.key.header && h.tt.Focused(r.key.path) {
			return true
		}
	}
	return false
}

func (h *harness) press(k ui.Key) {
	h.T.Helper()
	h.tt.Key(0, k)
	h.tt.Frame()
}

func (h *harness) selectedFile() changeKey {
	h.T.Helper()
	if h.u.wt.selected == nil {
		h.T.Fatal("nothing selected")
	}
	return *h.u.wt.selected
}

// TestKeyboardStagingFollowsTheFile stages three files with Space alone:
// the arrows cross the areas without stopping on their headings, the
// selection follows each file to Staged, and the list stays one stop of
// Tab with the keyboard focus.
func TestKeyboardStagingFollowsTheFile(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "t\n")
	for _, f := range []string{"a.txt", "b.txt"} {
		gittest.Write(t, filepath.Join(dir, f), f+"\n")
	}
	h.open(dir)
	h.tt.SetFocused(true)
	h.tabTo("Stage seed.txt")

	if got := h.selectedFile(); got.area != workflow.Unstaged || got.path != "seed.txt" {
		t.Fatalf("selected %+v", got)
	}
	// Down crosses the Untracked heading.
	h.press(ui.KeyDown)
	if got := h.selectedFile(); got.area != workflow.Untracked || got.path != "a.txt" {
		t.Fatalf("selected %+v", got)
	}
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("selecting changed the index: %q", got)
	}

	// Space stages a.txt; the selection goes with it to Staged.
	h.press(ui.KeySpace)
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\n" {
		t.Fatalf("index %q", got)
	}
	if got := h.selectedFile(); got.area != workflow.Staged || got.path != "a.txt" {
		t.Fatalf("selection did not follow: %+v", got)
	}
	if !h.inList() {
		t.Fatal("staging took the focus from the list")
	}
	if r := h.u.wt.rows[h.u.wt.cursor]; r.key != h.selectedFile() {
		t.Fatalf("cursor on %+v", r.key)
	}

	// End goes to the last file, b.txt, and Space stages it; Up and Up
	// again reach seed.txt over the Unstaged heading.
	h.press(ui.KeyEnd)
	if got := h.selectedFile(); got.path != "b.txt" {
		t.Fatalf("selected %+v", got)
	}
	h.press(ui.KeySpace)
	h.press(ui.KeyHome)
	if got := h.selectedFile(); got.area != workflow.Staged || got.path != "a.txt" {
		t.Fatalf("Home selected %+v", got)
	}
	h.press(ui.KeyEnd)
	if got := h.selectedFile(); got.area != workflow.Unstaged || got.path != "seed.txt" {
		t.Fatalf("End selected %+v", got)
	}
	h.press(ui.KeySpace)
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\nb.txt\nseed.txt\n" {
		t.Fatalf("index %q", got)
	}
	if got := h.selectedFile(); got.area != workflow.Staged || got.path != "seed.txt" {
		t.Fatalf("selection %+v", got)
	}
	// The diff shown is the staged one of the selected file.
	if d := h.u.wt.diff; d == nil || d.key != h.selectedFile() {
		t.Fatalf("diff %+v", d)
	}
	snapshot(t, h.tt, "keyboard-staging")

	// The list is one stop: Tab leaves it at once, Shift-Tab comes back.
	h.press(ui.KeyTab)
	if h.inList() {
		t.Fatal("Tab stopped inside the list")
	}
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.tt.Frame()
	if !h.inList() {
		t.Fatal("Shift-Tab did not come back to the list")
	}

	// Space while another change runs does nothing.
	h.u.wt.bulk = "stage"
	h.u.br.busy = "x"
	h.press(ui.KeySpace)
	h.u.br.busy = ""
	h.u.wt.bulk = ""
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\nb.txt\nseed.txt\n" {
		t.Fatalf("Space ran while busy: %q", got)
	}
}

// TestClickingACheckBoxKeepsTheListFocus stages with the mouse and goes on
// from the keyboard, in the same list.
func TestClickingACheckBoxKeepsTheListFocus(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	for _, f := range []string{"a.txt", "b.txt"} {
		gittest.Write(t, filepath.Join(dir, f), f+"\n")
	}
	h.open(dir)
	h.tt.SetFocused(true)
	h.click("b.txt")
	h.click("Stage a.txt")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\n" {
		t.Fatalf("index %q", got)
	}
	h.tt.Frame()
	if !h.inList() {
		t.Fatal("the check box kept the focus")
	}
	// The selection stayed on b.txt, which Space now stages.
	h.press(ui.KeySpace)
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\nb.txt\n" {
		t.Fatalf("index %q", got)
	}
}

// TestPartlyStagedFileIsTwoEntries shows a file with staged and unstaged
// changes as two entries, each marked as a part, with its own diff and
// acting on its own area; the count names the file once.
func TestPartlyStagedFileIsTwoEntries(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\ntwo\n")
	gittest.Write(t, filepath.Join(dir, "g.txt"), "g\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "ONE\ntwo\n")
	h.Git(dir, "add", "f.txt")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "ONE\nTWO\n")
	gittest.Write(t, filepath.Join(dir, "g.txt"), "G\n")
	h.open(dir)

	// Each part says, in its accessible name, where the other part is.
	n := count(h.tt.Texts(), "Partial: Other changes of this file are staged") + count(h.tt.Texts(), "Partial: Other changes of this file are not staged yet")
	if n != 2 {
		t.Fatalf("%d partial marks in %q", n, h.tt.Texts())
	}
	if h.tt.HasText(h.u.t("changesCount", "count", 3)) {
		t.Fatal("a file in two areas counted twice")
	}
	// Each part has its own diff.
	h.selectFile(workflow.Staged, "f.txt")
	staged := h.u.wt.diff.info.Patch
	h.selectFile(workflow.Unstaged, "f.txt")
	unstaged := h.u.wt.diff.info.Patch
	if staged == unstaged || !strings.Contains(staged, "+ONE") || !strings.Contains(unstaged, "+TWO") || strings.Contains(unstaged, "+ONE") {
		t.Fatalf("staged %q\nunstaged %q", staged, unstaged)
	}

	// Space on the unstaged part stages only it; the file is whole again.
	h.tt.SetFocused(true)
	h.tabTo("Stage f.txt")
	h.press(ui.KeySpace)
	if got := h.Git(dir, "diff", "--name-only"); got != "g.txt\n" {
		t.Fatalf("unstaged %q", got)
	}
	if got := h.Git(dir, "show", ":f.txt"); got != "ONE\nTWO\n" {
		t.Fatalf("index %q", got)
	}
	if h.tt.HasText("Partial") {
		t.Fatal("a whole file still marked as a part")
	}
}
