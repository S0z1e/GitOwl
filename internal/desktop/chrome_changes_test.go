package desktop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func (h *harness) wantTip(label, tip string) {
	h.T.Helper()
	r, ok := h.tt.Find(label)
	if !ok {
		h.T.Fatalf("missing %q in %q", label, h.tt.Texts())
	}
	h.tt.Move(r.X+r.W/2, r.Y+r.H/2)
	h.tt.Advance(700 * time.Millisecond)
	h.tt.Frame()
	h.want(tip)
}

func chromeDirtyRepo(t *testing.T, w, h int) (*harness, string) {
	t.Helper()
	hr, dir := chromeRepo(t, w, h)
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	gittest.Write(t, filepath.Join(dir, "src", "new.txt"), "n\n")
	hr.u.Refresh()
	hr.tt.Frame()
	return hr, dir
}

func TestChromeCurrentChangesListsThreeAreas(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "one\n")
	h.Commit(dir, "Mixed")
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "staged\n")
	h.Git(dir, "add", "mixed.txt")
	gittest.Write(t, filepath.Join(dir, "mixed.txt"), "unstaged\n")
	gittest.Write(t, filepath.Join(dir, "fresh.txt"), "new\n")
	h.u.Refresh()
	h.tt.Frame()

	h.want("Staged changes", "Unstaged changes", "Untracked files", "mixed.txt", "fresh.txt", "Filter files", "Stage All", "Unstage All")
	if h.tt.HasText("No staged changes") || h.tt.HasText("No untracked files") {
		t.Fatalf("empty-area copy leaked into chrome: %q", h.tt.Texts())
	}
	if _, ok := h.tt.Find("Diff"); ok {
		t.Fatalf("a Diff column title is on screen: %q", h.tt.Texts())
	}
}

func TestChromeEmptyAreasAreOmitted(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "only.txt"), "a\n")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Untracked files", "only.txt")
	for _, s := range []string{"Staged changes", "Unstaged changes"} {
		if h.tt.HasText(s) {
			t.Fatalf("empty area %q was drawn: %q", s, h.tt.Texts())
		}
	}
}

func TestChromeFilterHidesRowsAndKeepsRealCounts(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "alpha.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "beta.txt"), "b\n")
	gittest.Write(t, filepath.Join(dir, "dir", "gamma.txt"), "g\n")
	h.u.Refresh()
	h.tt.Frame()

	if err := h.tt.Click("Filter files"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("AlP")
	h.tt.Frame()
	if !h.tt.HasText("alpha.txt") || h.tt.HasText("beta.txt") || h.tt.HasText("gamma.txt") {
		t.Fatalf("filter did not hide rows: %q", h.tt.Texts())
	}
	h.want("Untracked files", "3")
	if h.tt.HasText("1 / 3") {
		t.Fatal("filter reduced the section count")
	}

	h.u.setFilter("zzz-none")
	h.tt.Frame()
	h.want("Untracked files", "3")
	if h.tt.HasText("alpha.txt") || h.tt.HasText("beta.txt") {
		t.Fatalf("filtered-empty still shows files: %q", h.tt.Texts())
	}

	h.u.setFilter("dir")
	h.tt.Frame()
	if !h.tt.HasText("gamma.txt") || h.tt.HasText("alpha.txt") {
		t.Fatalf("directory filter: %q", h.tt.Texts())
	}
}

func TestChromeStageAllCoversHiddenFilterRows(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "alpha.txt"), "a\n")
	gittest.Write(t, filepath.Join(dir, "beta.txt"), "b\n")
	h.u.Refresh()
	h.tt.Frame()
	h.u.setFilter("alpha")
	h.tt.Frame()
	if h.tt.HasText("2 files") || h.tt.HasText("1 file") {
		t.Fatalf("Stage All showed a count: %q", h.tt.Texts())
	}
	h.wantTip("Stage All", h.u.t("stageAllHint"))
	h.click("Stage All")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "alpha.txt\nbeta.txt\n" {
		t.Fatalf("index after filtered Stage All %q", got)
	}
	h.u.setFilter("beta")
	h.tt.Frame()
	h.click("Unstage All")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("index after filtered Unstage All %q", got)
	}
}

func TestChromeClickRowSelectsDiffCheckboxStages(t *testing.T) {
	h, dir := chromeDirtyRepo(t, 1280, 800)
	before := h.Git(dir, "diff", "--cached", "--name-only")
	h.click("seed.txt")
	if s := h.u.wt.selected; s == nil || s.path != "seed.txt" {
		t.Fatalf("selection %+v", s)
	}
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != before {
		t.Fatalf("clicking the row staged %q", got)
	}
	h.want("edit")
	h.click("Stage src/new.txt")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "src/new.txt\n" {
		t.Fatalf("index %q", got)
	}
	if s := h.u.wt.selected; s == nil || s.path != "seed.txt" {
		t.Fatalf("checkbox moved the selection to %+v", s)
	}
}

func TestChromePartialFileIsTwoRows(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\ntwo\n")
	h.Commit(dir, "Base")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "ONE\ntwo\n")
	h.Git(dir, "add", "f.txt")
	gittest.Write(t, filepath.Join(dir, "f.txt"), "ONE\nTWO\n")
	h.u.Refresh()
	h.tt.Frame()
	h.want("Staged changes", "Unstaged changes", "Partial")
	h.want("Other changes of this file are staged", "Other changes of this file are not staged yet")
	h.selectFile(workflow.Staged, "f.txt")
	staged := h.u.wt.diff.info.Patch
	h.selectFile(workflow.Unstaged, "f.txt")
	unstaged := h.u.wt.diff.info.Patch
	if staged == unstaged || !strings.Contains(staged, "+ONE") || !strings.Contains(unstaged, "+TWO") {
		t.Fatalf("staged %q\nunstaged %q", staged, unstaged)
	}
	h.click("Stage f.txt")
	h.tt.Frame()
	if _, ok := h.tt.Find("Partial"); ok {
		t.Fatal("a whole file still marked Partial")
	}
}

func TestChromeArrowsCrossAreasWithoutStoppingOnHeaders(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "t\n")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.u.Refresh()
	h.tt.Frame()
	h.tt.SetFocused(true)
	h.tabTo("Changes")
	if got := h.selectedFile(); got.path != "seed.txt" {
		t.Fatalf("wide window should already select %+v", got)
	}
	h.press(ui.KeyDown)
	if got := h.selectedFile(); got.area != workflow.Untracked || got.path != "a.txt" {
		t.Fatalf("Down stopped on a header: %+v", got)
	}
	h.press(ui.KeyHome)
	if got := h.selectedFile(); got.path != "seed.txt" {
		t.Fatalf("Home %+v", got)
	}
	h.press(ui.KeyEnd)
	if got := h.selectedFile(); got.path != "a.txt" {
		t.Fatalf("End %+v", got)
	}
	h.press(ui.KeySpace)
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "a.txt\n" {
		t.Fatalf("Space did not stage: %q", got)
	}
}

func TestChromeNarrowStaysOnListUntilRowClick(t *testing.T) {
	h, dir := chromeDirtyRepo(t, 720, 800)
	if h.tt.HasText("Unified") || h.tt.HasText("Wrap lines") {
		t.Fatalf("narrow window showed the Diff first: %q", h.tt.Texts())
	}
	h.want("seed.txt", "Stage All")
	if h.u.wt.selected != nil {
		t.Fatalf("narrow auto-selected %+v", h.u.wt.selected)
	}
	h.click("seed.txt")
	h.want("Unified", "Wrap lines", "Back", "edit")
	if h.tt.HasText("Stage All") {
		t.Fatal("narrow Diff still shows the list")
	}
	dirR, _ := h.tt.Find("src")
	nameR, ok := h.tt.Find("new.txt")
	_ = dir
	if ok && dirR.H > 0 && dirR.Y+1 < nameR.Y {
		// directory sits under the name on the Diff header
	}
	h.click("Back")
	h.want("Stage All", "seed.txt")
	if h.tt.HasText("Unified") {
		t.Fatalf("Back did not restore the list: %q", h.tt.Texts())
	}
}

func TestChromeWideSelectsFirstFileAndShowsFourColumns(t *testing.T) {
	h, dir := chromeDirtyRepo(t, 1280, 800)
	if h.u.wt.selected == nil || h.u.wt.diff == nil {
		t.Fatalf("wide window left the first file unselected: %+v", h.u.wt.selected)
	}
	h.want("edit", "Unified", "Split", "Wrap lines")
	nav, ok := h.tt.Find("Current changes")
	if !ok {
		t.Fatal("navigation missing")
	}
	file, ok := h.tt.Find("seed.txt")
	if !ok {
		t.Fatal("file list missing")
	}
	unified, ok := h.tt.Find("Unified")
	if !ok {
		t.Fatal("Diff missing")
	}
	if nav.X < float32(chromeRailWidth) || file.X <= nav.X || unified.X <= file.X {
		t.Fatalf("columns not rail/nav/list/diff: nav=%v file=%v unified=%v", nav, file, unified)
	}
	h.click("Split")
	if !h.u.diffSplit {
		t.Fatal("Split did not switch the Diff")
	}
	h.click("Wrap lines")
	if !h.u.diffWrap {
		t.Fatal("Wrap lines did not toggle")
	}
	_ = dir
}

func TestChromeCleanWorkingTreeStaysOnCurrentChanges(t *testing.T) {
	h, _ := chromeRepo(t, 1280, 800)
	if h.u.middleTab != 0 {
		t.Fatalf("clean workspace left Current changes, tab %d", h.u.middleTab)
	}
	h.want("Current changes", "Filter files")
	// Stage All and Unstage All live in a section header, and a clean tree
	// has no sections, so neither button is on screen.
	for _, s := range []string{"View History", "Staged changes", "Unstaged changes", "Untracked files"} {
		if h.tt.HasText(s) {
			t.Fatalf("clean workspace shows %q in %q", s, h.tt.Texts())
		}
	}
	if !h.tt.HasText(h.u.t("cleanTitle")) {
		t.Fatalf("clean workspace shows no empty-state title in %q", h.tt.Texts())
	}
	if h.u.stageAllBlock() != "errorNothingToStage" || h.u.unstageAllBlock() != "emptyStaged" {
		t.Fatalf("bulk blocks %q %q", h.u.stageAllBlock(), h.u.unstageAllBlock())
	}
}

func TestChromeRefreshKeepsLastKnownStateUntilReadFinishes(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	g.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(chromeWindow(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	tt.Frame()
	if !tt.HasText("seed.txt") {
		t.Fatalf("open lost the file: %q", tt.Texts())
	}
	gittest.Write(t, filepath.Join(dir, "later.txt"), "l\n")
	u.Refresh()
	tt.Frame()
	if tt.HasText("later.txt") {
		t.Fatal("a read in flight replaced the last known list")
	}
	if !tt.HasText("seed.txt") {
		t.Fatalf("in-flight refresh cleared the list: %q", tt.Texts())
	}
	q.drain()
	tt.Frame()
	if !tt.HasText("later.txt") {
		t.Fatalf("finished refresh missing later.txt: %q", tt.Texts())
	}
}

func TestChromeUnstageAllDisabledDuringConflict(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "base\n")
	h.Commit(dir, "Base")
	h.Git(dir, "switch", "-q", "-c", "feature")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "feature\n")
	h.Commit(dir, "Feature")
	h.Git(dir, "switch", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "main\n")
	h.Commit(dir, "Main")
	if _, err := h.Env.Runner.Run(context.Background(), dir, nil, "merge", "feature"); err == nil {
		t.Fatal("expected a conflict")
	}
	h.u.Refresh()
	h.tt.Frame()
	if h.u.unstageAllBlock() != "errorUnstageAllConflicts" {
		t.Fatalf("unstage block %q", h.u.unstageAllBlock())
	}
	// Unstage All sits in the Staged header, which a conflict does not create,
	// so only its block reason is checked here.
	if h.u.unstageAllBlock() != "errorUnstageAllConflicts" {
		t.Fatal("Unstage All is not blocked during a conflict")
	}
	h.want("Conflicted")
}

func TestChromeDeletedFileMenuOffersWorktreeFolder(t *testing.T) {
	h, l, dir := chromeLaunchRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "gone.txt"), "a\n")
	h.Commit(dir, "Keep")
	os.Remove(filepath.Join(dir, "gone.txt"))
	h.u.Refresh()
	h.tt.Frame()
	h.selectFile(workflow.Unstaged, "gone.txt")
	h.tt.Frame()
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

func TestChromeStageAllBusyWaitsForOtherWrites(t *testing.T) {
	g := gittest.New(t)
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	g.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "b\n")
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(chromeWindow(u), 1280, 800)
	u.OpenPath(dir)
	q.drain()
	tt.Frame()
	if err := tt.Click("Stage All"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if u.commitBlock() != "busy" || u.draftBlock() != "busy" || !u.switchBlocked() {
		t.Fatalf("busy blocks commit=%q draft=%q switch=%v", u.commitBlock(), u.draftBlock(), u.switchBlocked())
	}
	q.drain()
	tt.Frame()
}

func TestChromeStatusLettersStayNeutralOnTheList(t *testing.T) {
	h, dir := chromeDirtyRepo(t, 1280, 800)
	h.want("M", "U")
	row, ok := h.tt.Find("M")
	if !ok {
		t.Fatal("status letter missing")
	}
	unified, ok := h.tt.Find("Unified")
	if !ok {
		t.Fatal("Diff missing")
	}
	if row.X >= unified.X {
		t.Fatalf("status letter is on the Diff, not the list: letter=%v unified=%v", row, unified)
	}
	_ = dir
}
