package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func (h *harness) click(label string) {
	h.T.Helper()
	if err := h.tt.Click(label); err != nil {
		h.T.Fatalf("click %q: %v (texts %q)", label, err, h.tt.Texts())
	}
	h.tt.Frame()
}

func TestStageAndUnstageFromTheWindow(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "two\n")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "new\n")
	h.open(dir)

	h.selectFile(workflow.Unstaged, "a.txt")
	h.click("Stage a.txt")
	if got := h.Git(dir, "show", ":a.txt"); got != "two\n" {
		t.Fatalf("index %q", got)
	}
	// The list, the selection and the diff follow the file to Staged.
	if h.u.wt.selected == nil || h.u.wt.selected.area != workflow.Staged {
		t.Fatalf("selection %+v", h.u.wt.selected)
	}
	if p := h.u.wt.diff; p == nil || p.key.area != workflow.Staged || !strings.Contains(p.state.New.String(), "two") {
		t.Fatalf("diff after staging %+v", p)
	}
	h.want("Staged changes", "Unstage a.txt")

	h.click("Stage new.txt")
	h.want("Staged changes", "new.txt")

	h.click("Unstage a.txt")
	if got := h.Git(dir, "show", ":a.txt"); got != "one\n" {
		t.Fatalf("index after unstage %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "two\n" {
		t.Fatalf("working tree %q", data)
	}
	if h.u.wt.selected == nil || h.u.wt.selected.area != workflow.Unstaged {
		t.Fatalf("selection after unstage %+v", h.u.wt.selected)
	}
	snapshot(t, h.tt, "after-unstage")
}

func TestUnstageInRepositoryWithoutCommitsFromTheWindow(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "first.txt"), "first\n")
	h.Git(dir, "add", "first.txt")
	h.open(dir)
	h.click("Unstage first.txt")
	h.want("Untracked files", "first.txt")
	if data, _ := os.ReadFile(filepath.Join(dir, "first.txt")); string(data) != "first\n" {
		t.Fatalf("file %q", data)
	}
}

func TestStaleStageIsRefusedAndRefreshed(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "1\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "2\n")
	h.open(dir)
	h.Git(dir, "add", "a.txt") // outside GitOwl
	gittest.Write(t, filepath.Join(dir, "a.txt"), "3\n")
	h.click("Stage a.txt")
	h.want("The repository changed since it was last read. GitOwl refreshed it; review and try again.", "Staged changes")
	if got := h.Git(dir, "show", ":a.txt"); got != "2\n" {
		t.Fatalf("stale stage ran: %q", got)
	}
}

func TestStageResultAfterSwitchIsDropped(t *testing.T) {
	g := gittest.New(t)
	a, b := g.Repo(), g.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	u.OpenPath(a)
	q.drain()
	u.toggleStaged(u.snap.Untracked[0])
	u.OpenPath(b)
	q.run(1)
	q.run(0) // The stage finishes after the switch: b is current, so a is closed.
	if u.repo.Root() != b || len(u.snap.Staged) != 0 || u.wt.busy != nil {
		t.Fatalf("stale stage result applied: %+v", u.snap)
	}
	if out := g.Git(a, "status", "--porcelain"); !strings.HasPrefix(out, "??") {
		t.Fatalf("closed repository was changed: %q", out)
	}
}

func TestCheckboxStagesWithoutSelecting(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "changed\n")
	gittest.Write(t, filepath.Join(dir, "src", "b.txt"), "b\n")
	h.open(dir)
	h.want("b.txt", "src")

	// Selecting a row shows its diff and leaves the index alone.
	h.click("a.txt")
	if s := h.u.wt.selected; s == nil || s.path != "a.txt" || s.area != workflow.Unstaged {
		t.Fatalf("selection %+v", s)
	}
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("selecting staged %q", got)
	}
	// The check box stages its file and leaves the selection alone.
	h.click("Stage src/b.txt")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "src/b.txt\n" {
		t.Fatalf("index %q", got)
	}
	if s := h.u.wt.selected; s == nil || s.path != "a.txt" {
		t.Fatalf("the check box changed the selection to %+v", s)
	}
	h.want("Staged changes", "Unstage src/b.txt")
	snapshot(t, h.tt, "checkbox-staged")
	h.click("Unstage src/b.txt")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index after unstage %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "src", "b.txt")); string(data) != "b\n" {
		t.Fatalf("working tree %q", data)
	}
}

func TestCheckboxesOfOnePathInTwoAreas(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "m.txt"), "1\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "m.txt"), "1\n2\n")
	h.Git(dir, "add", "m.txt")
	gittest.Write(t, filepath.Join(dir, "m.txt"), "1\n2\n3\n")
	h.open(dir)
	h.want("Unstage m.txt", "Stage m.txt")
	h.selectFile(workflow.Staged, "m.txt")
	if s := h.u.wt.diff.state.New.String(); strings.Contains(s, "3") {
		t.Fatalf("staged diff shows the unstaged part: %q", s)
	}
	h.selectFile(workflow.Unstaged, "m.txt")
	if s := h.u.wt.diff.state.New.String(); !strings.Contains(s, "3") {
		t.Fatalf("unstaged diff %q", s)
	}
	// The unstaged entry stages only its part, which is the rest.
	h.click("Stage m.txt")
	if got := h.Git(dir, "show", ":m.txt"); got != "1\n2\n3\n" {
		t.Fatalf("index %q", got)
	}
	if h.tt.HasText("Stage m.txt") {
		t.Fatal("unstaged entry kept")
	}
}

func TestCheckboxFailureShowsTheRealState(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.open(dir)
	gittest.Write(t, filepath.Join(dir, ".git", "index.lock"), "")
	h.click("Stage a.txt")
	if h.u.notice == nil {
		t.Fatal("no failure shown")
	}
	h.want("Git could not complete the operation.", "Stage a.txt", "Untracked files")
	if len(h.u.snap.Staged) != 0 || h.Git(dir, "diff", "--cached", "--name-only") != "" {
		t.Fatal("shows a stage that did not happen")
	}
	os.Remove(filepath.Join(dir, ".git", "index.lock"))
	h.click("Stage a.txt")
	h.want("Unstage a.txt")
}

func TestOneStagingAtATime(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	tt.Frame()
	if err := tt.Click("Stage a.txt"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Click("Stage b.txt") // disabled while a.txt is staged
	tt.Frame()
	if u.wt.busy == nil {
		t.Fatal("first Stage did not start")
	}
	q.drain()
	tt.Frame()
	if got := g.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\n" {
		t.Fatalf("index %q", got)
	}
}

func TestStagingFromTheKeyboard(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.open(dir)
	h.tt.SetFocused(true)
	// Tab reaches the list; arrows select a file and Space stages it.
	h.tabTo("Stage a.txt")
	h.tt.Key(0, ui.KeyEnd)
	h.tt.Frame()
	if s := h.u.wt.selected; s == nil || s.path != "a.txt" {
		t.Fatalf("selection %+v", s)
	}
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("selecting staged %q", got)
	}
	h.tt.Key(0, ui.KeySpace)
	h.tt.Frame()
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\n" {
		t.Fatalf("index %q", got)
	}
	// The selection follows the file, so Space again unstages it.
	h.tt.Key(0, ui.KeySpace)
	h.tt.Frame()
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index after second space %q", got)
	}
}
