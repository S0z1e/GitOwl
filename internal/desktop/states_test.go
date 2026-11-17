package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
)

func TestCleanRepositoryMergesContent(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	h.want("Current changes", "All commits", "main", "Summary")
	if h.tt.HasText("View History") {
		t.Fatalf("clean workspace shows View History in %q", h.tt.Texts())
	}
	if h.u.middleTab != 0 {
		t.Fatal("switched to history by itself")
	}
	snapshot(t, h.tt, "clean")
	h.tt.SetDark(true)
	snapshot(t, h.tt, "clean-dark")
	h.tt.SetDark(false)

	h.click("All commits")
	h.want("Initial")
	h.click("Current changes")
	h.want("Current changes", "Summary")

	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	h.u.Refresh()
	h.tt.Frame()
	h.want("a.txt", "Summary")
}

func TestCleanKeepsAndRestoresTheMessage(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	h.open(dir)
	h.typeMessage("Half written")

	h.Git(dir, "checkout", "--", "a.txt")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Summary")
	if h.tt.HasText("Continue Editing Message") || h.u.cm.message != "Half written" {
		t.Fatalf("message %q in %q", h.u.cm.message, h.tt.Texts())
	}

	gittest.Write(t, filepath.Join(dir, "a.txt"), "c\n")
	h.u.Refresh()
	h.tt.Frame()
	h.want("a.txt")
	if h.u.cm.message != "Half written" {
		t.Fatalf("message %q", h.u.cm.message)
	}
}

func TestCommitToCleanShowsTheResult(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.open(dir)
	h.want("No commits yet")
	h.click("Stage a.txt")
	h.typeMessage("Add a")
	h.click("Commit")
	h.want("Committed " + h.u.cm.last.Short() + ": Add a")
	if h.tt.HasText("Continue Editing Message") {
		t.Fatal("offers to restore a committed message")
	}
}

func TestUnbornCleanRepository(t *testing.T) {
	h := newHarness(t, 1280, 800)
	h.open(h.Repo())
	h.want("No commits yet", "Current changes")
	h.click("All commits")
	h.want("No commits yet")
}

func TestFailedRefreshIsNotClean(t *testing.T) {
	h := newHarness(t, 900, 700)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.open(dir)
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	h.u.Refresh()
	h.tt.Frame()
	if h.u.notice == nil {
		t.Fatal("no failure reported")
	}
	h.want("Dismiss", "a.txt")
	if h.tt.HasText("Working tree clean") {
		t.Fatal("a failed read shows as clean")
	}
}

func TestSwitchingRepositoryDropsCleanState(t *testing.T) {
	h := newHarness(t, 1280, 800)
	a, b := h.Repo(), h.Repo()
	gittest.Write(t, filepath.Join(a, "a.txt"), "a\n")
	gittest.Write(t, filepath.Join(b, "b.txt"), "b\n")
	h.Commit(b, "B")
	h.open(a)
	h.click("Stage a.txt")
	h.typeMessage("Add a")
	h.click("Commit")
	h.want("Committed")
	h.open(b)
	h.want("Current changes", "B")
	for _, got := range h.tt.Texts() {
		if got == "Continue Editing Message" || strings.HasPrefix(got, "Committed") {
			t.Fatalf("previous repository's %q shown", got)
		}
	}
}
