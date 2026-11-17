package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// TestChromeHistorySelectsNewestOnEntry is the first All commits tracer:
// opening the surface selects the newest commit and shows its detail under
// the list, without GitOwl switching there on its own.
func TestChromeHistorySelectsNewestOnEntry(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Older")
	gittest.Write(t, filepath.Join(dir, "x", "first.txt"), "1\n")
	gittest.Write(t, filepath.Join(dir, "y", "second.txt"), "2\n")
	h.Commit(dir, "Newest")
	h.u.Refresh()
	h.tt.Frame()
	if h.u.middleTab != 0 || h.u.hist.selected != "" {
		t.Fatalf("tab %d selected %q before All commits", h.u.middleTab, h.u.hist.selected)
	}

	h.click("All commits")
	if h.u.middleTab != 1 {
		t.Fatalf("All commits left tab %d", h.u.middleTab)
	}
	if len(h.u.hist.commits) == 0 {
		t.Fatal("All commits has no list")
	}
	c := h.u.hist.commits[h.u.hist.cursor]
	if c.Subject != "Newest" || h.u.hist.selected != c.Hash {
		t.Fatalf("selected %+v hash %q", c, h.u.hist.selected)
	}
	if h.u.hist.detail == nil || h.u.hist.detail.Subject != "Newest" {
		t.Fatalf("detail %+v", h.u.hist.detail)
	}
	if h.u.hist.fileCursor != -1 || h.u.hist.diff != nil {
		t.Fatalf("opened a file: cursor %d diff %+v", h.u.hist.fileCursor, h.u.hist.diff)
	}
	h.want("Newest", "Older", "Initial", "GitOwl Test", c.Short(), c.Hash)
	subj, ok := h.tt.Find("Newest")
	if !ok {
		t.Fatal("newest subject missing")
	}
	hash, ok := h.tt.Find(c.Hash)
	if !ok {
		t.Fatal("full hash missing from the detail")
	}
	if hash.Y <= subj.Y {
		t.Fatalf("detail hash y=%v not below the list subject y=%v", hash.Y, subj.Y)
	}
}

func TestChromeHistoryCloseDetailKeepsSelection(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	h.click("All commits")
	full := h.u.hist.selected
	h.want("Initial", full, "Commit", "Close")
	h.click("Close")
	// The detail slides shut over detailSlide before it is gone.
	h.tt.Advance(time.Second)
	if !h.u.hist.detailOpen {
		// closed
	} else {
		t.Fatal("Close left the detail open")
	}
	if h.u.hist.selected != full {
		t.Fatalf("Close cleared the selection %q", h.u.hist.selected)
	}
	if h.tt.HasText(full) {
		t.Fatalf("closed detail still shows the full hash in %q", h.tt.Texts())
	}
	h.want("Initial")
	for _, s := range []string{"Checkout", "Create branch…", "Revert…", "Load more commits"} {
		if h.tt.HasText(s) {
			t.Fatalf("history shows %q in %q", s, h.tt.Texts())
		}
	}
}

func TestChromeHistoryClickFileOpensChanges(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "x", "first.txt"), "1\nmore\n")
	gittest.Write(t, filepath.Join(dir, "y", "second.txt"), "2\n")
	h.Commit(dir, "Two files")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	if h.u.hist.detailTab != 0 || h.u.hist.fileCursor != -1 {
		t.Fatalf("tab %d file %d, want Commit with no file", h.u.hist.detailTab, h.u.hist.fileCursor)
	}
	h.click("first.txt")
	if h.u.hist.detailTab != 1 || h.u.hist.fileCursor != 0 {
		t.Fatalf("tab %d file %d, want Changes on first.txt", h.u.hist.detailTab, h.u.hist.fileCursor)
	}
	if p := h.u.hist.diff; p == nil || p.key.path != "x/first.txt" {
		t.Fatalf("diff %+v", p)
	}
	h.want("Changes")
	h.click("Files")
	if h.u.hist.detailTab != 2 {
		t.Fatalf("Files left tab %d", h.u.hist.detailTab)
	}
	h.want("first.txt", "second.txt")
	h.click("second.txt")
	if h.u.hist.detailTab != 1 || h.u.hist.diff == nil || h.u.hist.diff.key.path != "y/second.txt" {
		t.Fatalf("Files click: tab %d diff %+v", h.u.hist.detailTab, h.u.hist.diff)
	}
}

func TestChromeHistoryEmptyCountIsZero(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	dir := h.Repo()
	h.open(dir)
	h.click("All commits")
	h.want("No commits yet")
	if n := h.u.chromeHistoryCount(); n != 0 {
		t.Fatalf("count %d, want 0", n)
	}
	if h.u.middleTab != 1 {
		t.Fatalf("empty All commits left tab %d", h.u.middleTab)
	}
}

func TestChromeHistoryRefreshKeepsSelection(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Older")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	h.Commit(dir, "Newest")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	h.click("Older")
	older := h.u.hist.selected
	h.u.Refresh()
	h.tt.Frame()
	if h.u.hist.commits[h.u.hist.cursor].Subject != "Older" || h.u.hist.selected != older {
		t.Fatalf("refresh lost Older: %+v", h.u.hist.commits[h.u.hist.cursor])
	}
	gittest.Write(t, filepath.Join(dir, "c.txt"), "c\n")
	h.Commit(dir, "Made elsewhere")
	h.u.Refresh()
	h.tt.Frame()
	if h.u.hist.commits[h.u.hist.cursor].Subject != "Older" {
		t.Fatal("a new commit stole the selection")
	}
}

func TestChromeHistoryNavCountIsFullList(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	for i := range 8 {
		h.Git(dir, "commit", "-q", "--allow-empty", "-m", "c"+string(rune('A'+i)))
	}
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	if n := h.u.chromeHistoryCount(); n != 9 || n != len(h.u.hist.commits) {
		t.Fatalf("count %d list %d, want 9", n, len(h.u.hist.commits))
	}
	if h.u.hist.more {
		t.Fatal("All commits left a next page")
	}
	for _, s := range []string{"Load more commits"} {
		if h.tt.HasText(s) {
			t.Fatalf("history shows %q in %q", s, h.tt.Texts())
		}
	}
	h.want("Initial", "cA", "cH", "main")
}

func TestChromeHistoryParentInListIsSelectable(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Parent commit")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	h.Commit(dir, "Child commit")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	parent := h.u.hist.commits[1]
	h.click(parent.Short())
	if h.u.hist.commits[h.u.hist.cursor].Subject != "Parent commit" {
		t.Fatalf("clicked parent, got %+v", h.u.hist.commits[h.u.hist.cursor])
	}
	if h.u.hist.detail == nil || h.u.hist.detail.Subject != "Parent commit" {
		t.Fatalf("detail %+v", h.u.hist.detail)
	}
}

func TestChromeHistoryParentOutsideListIsNotClickable(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Child")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	missing := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	h.u.hist.detail.Parents = []string{missing}
	h.tt.Frame()
	if h.u.chromeHistoryIndex(missing) >= 0 {
		t.Fatal("missing parent was in the list")
	}
	if !h.tt.HasText("deadbee") {
		t.Fatalf("parent hash missing in %q", h.tt.Texts())
	}
	_ = h.tt.Click("deadbee")
	h.tt.Frame()
	if h.u.hist.commits[h.u.hist.cursor].Subject != "Child" {
		t.Fatalf("a parent outside the list was selected: %+v", h.u.hist.commits[h.u.hist.cursor])
	}
}

func TestChromeHistoryCopyHashWritesFullHash(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	h.click("All commits")
	full := h.u.hist.commits[0].Hash
	h.click("Copy hash")
	if got := h.tt.Clipboard(); got != full {
		t.Fatalf("clipboard %q, want %q", got, full)
	}
	h.want("Hash copied")
}

func TestChromeHistoryScopeFollowsCheckedOutBranch(t *testing.T) {
	h, _ := chromeHarness(t, 1280, 800)
	_, mine, theirs := clones(t, h.Env)
	theyPush(t, h.Env, theirs, "up.txt")
	h.open(mine)
	h.Git(mine, "fetch", "-q", "origin")
	h.u.Refresh()
	h.tt.Frame()
	h.Git(mine, "switch", "-q", "-c", "other")
	gittest.Write(t, filepath.Join(mine, "only-other.txt"), "o\n")
	h.Commit(mine, "Only on other")
	h.Git(mine, "switch", "-q", "main")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	subjects := map[string]bool{}
	for _, c := range h.u.hist.commits {
		subjects[c.Subject] = true
	}
	if !subjects["Initial"] || !subjects["Only on other"] {
		t.Fatalf("All branches should be shown by default: %v", subjects)
	}
	if !subjects["Their up.txt"] && !hasIncoming(h) {
		t.Fatalf("missing upstream commit in %v", subjects)
	}
	// Browsing a branch draws only that branch, hiding the others.
	h.u.browseScope("local:other", workflow.HistoryScope{Ref: "refs/heads/other", Name: "other"})
	h.tt.Frame()
	subjects = map[string]bool{}
	for _, c := range h.u.hist.commits {
		subjects[c.Subject] = true
	}
	if !subjects["Only on other"] || subjects["Their up.txt"] {
		t.Fatalf("browsing other drew %v", subjects)
	}
}

func hasIncoming(h *harness) bool {
	for _, cl := range h.u.hist.classes {
		if cl == workflow.Incoming {
			return true
		}
	}
	for _, c := range h.u.hist.commits {
		if c.Subject == "Their up.txt" {
			return true
		}
	}
	return false
}

func TestChromeHistoryDeletedFileOffersWorktreeFolder(t *testing.T) {
	h, l, dir := chromeLaunchRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "gone.txt"), "a\n")
	h.Commit(dir, "Add gone")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	h.Git(dir, "add", "-A")
	h.Commit(dir, "Remove gone")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	h.click("Add gone")
	if err := h.tt.RightClick("gone.txt"); err != nil {
		t.Fatal(err)
	}
	if err := h.tt.ChooseMenuItem("Show Worktree Folder in Finder"); err != nil {
		t.Fatalf("%v (%q)", err, h.tt.Menu())
	}
	h.tt.Frame()
	if len(l.got) != 1 || l.got[0].Path != dir {
		t.Fatalf("requests %+v", l.got)
	}
}

func TestChromeHistoryReadsTheFullListWithoutPaging(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	for i := range historyPage + 3 {
		h.Git(dir, "commit", "-q", "--allow-empty", "-m", "n"+string(rune('0'+i%10)))
	}
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	want := historyPage + 4
	if n := len(h.u.hist.commits); n != want || h.u.hist.more || h.u.chromeHistoryCount() != n {
		t.Fatalf("list %d more=%v count %d, want %d and no next page", n, h.u.hist.more, h.u.chromeHistoryCount(), want)
	}
	if h.tt.HasText("Load more commits") {
		t.Fatal("All commits shows Load more")
	}
}

func TestChromeHistoryRowsShowGraphRefsAndTime(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Tip commit")
	h.u.Refresh()
	h.tt.Frame()
	h.click("All commits")
	c := h.u.hist.commits[0]
	h.want("Tip commit", "GitOwl Test", c.Short(), "main", h.u.t("commitWhen", "when", c.When))
	if !h.u.hist.graphing || len(h.u.hist.rows) != len(h.u.hist.commits) {
		t.Fatalf("graphing=%v rows=%d commits=%d", h.u.hist.graphing, len(h.u.hist.rows), len(h.u.hist.commits))
	}
	if h.u.hist.more {
		t.Fatal("paged history")
	}
}
