package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func TestDiscardFromDiffMenu(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "staged\n")
	h.Git(dir, "add", "f.txt")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "staged\nunwanted\n")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "keep\n")
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	h.menu("Actions for f.txt", "Discard Unstaged Changes…")
	h.want("Discard unstaged changes to f.txt?", filepath.Join(dir, "f.txt"),
		"The file goes back to its staged version. What is staged stays staged; only the edits made since are lost.")
	snapshot(t, h.tt, "discard-confirm")
	// Cancel changes nothing.
	h.click("Cancel")
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != "staged\nunwanted\n" {
		t.Fatal("cancel changed the file")
	}
	h.menu("Actions for f.txt", "Discard Unstaged Changes…")
	h.click("Discard Changes")
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != "staged\n" {
		t.Fatalf("file %q", data)
	}
	if h.Git(dir, "diff", "--cached", "--name-only") != "f.txt\n" {
		t.Fatal("staged change lost")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "new.txt")); string(data) != "keep\n" {
		t.Fatal("untracked file touched")
	}
	if !announced(h.tt, "Discarded unstaged changes to f.txt") || h.u.discard.open {
		t.Fatalf("after: %q", h.tt.Announcements())
	}
	h.want("Staged changes")
	// The selection followed the file to the staged side.
	if s := h.u.wt.selected; s == nil || s.area != workflow.Staged {
		t.Fatalf("selection %+v", s)
	}
}

func TestDiscardAsksAgainAfterChange(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "two\n")
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	h.menu("Actions for f.txt", "Discard Unstaged Changes…")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "three\n")
	h.click("Discard Changes")
	h.want("The file or the staging area changed since you confirmed, so nothing was discarded. Review it below and confirm again.")
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != "three\n" || !h.u.discard.open {
		t.Fatal("discarded a newer edit")
	}
	// Confirming again discards what is there now.
	h.click("Discard Changes")
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != "one\n" {
		t.Fatalf("file %q", data)
	}
}

func TestDiscardContextMenuAndUnsupported(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "gone.txt"), "g\n")
	gittest.Write(t, filepath.Join(dir, "target"), "t\n")
	os.Symlink("target", filepath.Join(dir, "link"))
	h.Commit(dir, "Initial")
	os.Remove(filepath.Join(dir, "gone.txt"))
	os.Remove(filepath.Join(dir, "link"))
	os.Symlink("gone.txt", filepath.Join(dir, "link"))
	h.open(dir)
	if err := h.tt.RightClick("gone.txt"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Discard Unstaged Changes…"); err != nil {
		t.Fatalf("%v %q", err, h.tt.Menu())
	}
	h.tt.Frame()
	h.want("The deleted file is restored from its staged version.")
	h.click("Discard Changes")
	if data, _ := os.ReadFile(filepath.Join(dir, "gone.txt")); string(data) != "g\n" {
		t.Fatal("not restored")
	}
	h.u.planDiscard(workflow.Change{Path: "link", Area: workflow.Unstaged, Kind: h.u.snap.Unstaged[0].Kind})
	h.tt.Frame()
	h.want("Symbolic links and type changes cannot be discarded here.")
	if h.u.discard.open {
		t.Fatal("confirmation for a link")
	}
}

func TestDiscardBlockedWhileWriting(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	g.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "two\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	tt.Frame()
	u.planDiscard(u.snap.Unstaged[0])
	q.drain()
	u.confirmDiscard()
	// While it runs nothing else changes the repository or leaves it.
	if !u.writing() || u.Can(ActionClose) || u.stageAllBlock() != "busy" {
		t.Fatal("discard not exclusive")
	}
	u.bulkStage(false)
	if len(q.jobs) != 1 {
		t.Fatalf("%d jobs", len(q.jobs))
	}
	q.drain()
	if u.writing() {
		t.Fatal("still busy")
	}
}
