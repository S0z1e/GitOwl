package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func numbered(n int, edits map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if s, ok := edits[i]; ok {
			b.WriteString(s)
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// stageHunk clicks the Stage/Unstage button of hunk card index (from 1),
// found by its unique accessible label. The design draws every hunk as a
// card with its own always-visible action, so no hover is needed.
func (h *harness) stageHunk(stage bool, index, count int) {
	h.T.Helper()
	action := "Stage chunk"
	if !stage {
		action = "Unstage chunk"
	}
	hint := h.u.t("hunkActionHint", "index", index, "count", count)
	label := action + " · " + hint
	if err := h.tt.Click(label); err != nil {
		h.T.Fatalf("click hunk %d/%d: %v (texts %q)", index, count, err, h.tt.Texts())
	}
	h.tt.Frame()
}

func TestStageHunkFromDiff(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, nil))
	h.Commit(dir, "Initial")
	edited := numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"})
	gittest.Write(t, filepath.Join(dir, "f.txt"), edited)
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	snapshot(t, h.tt, "hunk-actions")
	// Two edits make two hunks; stage the second (line 25) from its card.
	h.stageHunk(true, 2, 2)
	if got := h.Git(dir, "show", ":f.txt"); got != numbered(30, map[int]string{25: "twenty-five\n"}) {
		t.Fatalf("index:\n%s", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "f.txt")); string(data) != edited {
		t.Fatal("file changed")
	}
	// The file is in both areas now; the unstaged diff shows what is left.
	h.want("Staged changes", "Unstaged changes")
	if s := h.u.wt.selected; s == nil || s.area != workflow.Unstaged {
		t.Fatalf("selection %+v", s)
	}
	if p := h.u.wt.diff; p == nil || strings.Contains(p.info.Patch, "twenty-five") || !strings.Contains(p.info.Patch, "two") {
		t.Fatalf("diff after staging %+v", p)
	}

	// Unstage it from the staged side; the selection follows the file back
	// when nothing of it is staged.
	h.selectFile(workflow.Staged, "f.txt")
	h.stageHunk(false, 1, 1)
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("still staged %q", got)
	}
	if s := h.u.wt.selected; s == nil || s.area != workflow.Unstaged || s.path != "f.txt" {
		t.Fatalf("selection %+v", s)
	}
}

func TestHunkStaleExplains(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n"}))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n", 3: "three\n"}))
	h.u.applyHunk(0)
	h.tt.Frame()
	h.want("The file changed since its diff was shown, so nothing was staged. GitOwl read it again; choose the hunk again.")
	if p := h.u.wt.diff; p == nil || !strings.Contains(p.info.Patch, "three") {
		t.Fatal("diff not read again")
	}
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("staged %q", got)
	}
}

func TestNewFileExplainsWholeFile(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "n\n")
	h.Git(dir, "add", "new.txt")
	h.open(dir)
	h.selectFile(workflow.Staged, "new.txt")
	h.want("New files are staged and unstaged as a whole file.", "Unstage")
	if h.u.wt.diff.state.HunkActions {
		t.Fatal("hunk actions on a new file")
	}
}

func chromeHunkRepo(t *testing.T) (*harness, string) {
	t.Helper()
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, nil))
	h.Commit(dir, "Initial")
	return h, dir
}

// stepHunk moves the diff of path to the next or previous hunk through the
// file's … menu, since the design draws no step bar.
func (h *harness) stepHunk(path, item string) {
	h.T.Helper()
	h.click(h.u.t("fileActions", "path", path))
	if err := h.tt.ChooseMenuItem(item); err != nil {
		h.T.Fatalf("%v (menu %q)", err, h.tt.Menu())
	}
	h.tt.Frame()
}

func TestChromeHunkBarNamesTheCurrentHunkAndMoves(t *testing.T) {
	h, dir := chromeHunkRepo(t)
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"}))
	h.u.Refresh()
	h.tt.Frame()
	h.selectFile(workflow.Unstaged, "f.txt")
	h.want("Stage chunk")
	h.stepHunk("f.txt", "Next Hunk")
	if p := h.u.wt.diff; p == nil || p.hunk != 1 {
		t.Fatalf("next hunk %+v", p)
	}
	h.stepHunk("f.txt", "Previous Hunk")
	if p := h.u.wt.diff; p == nil || p.hunk != 0 {
		t.Fatalf("previous hunk %+v", p)
	}
}

func TestChromeHunkBarStagesTheNamedHunk(t *testing.T) {
	h, dir := chromeHunkRepo(t)
	edited := numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"})
	gittest.Write(t, filepath.Join(dir, "f.txt"), edited)
	h.u.Refresh()
	h.tt.Frame()
	h.selectFile(workflow.Unstaged, "f.txt")
	h.stepHunk("f.txt", "Next Hunk")
	// Each card's Stage hunk button stages its own hunk. The first card is
	// hunk 1, so it stages line 2 even though hunk 2 is the current one.
	h.click("Stage chunk")
	if got := h.Git(dir, "show", ":f.txt"); got != numbered(30, map[int]string{2: "two\n"}) {
		t.Fatalf("index:\n%s", got)
	}
	h.selectFile(workflow.Staged, "f.txt")
	h.want("Unstage chunk")
	h.click("Unstage chunk")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("still staged %q", got)
	}
}

func TestChromeWholeFileDiffExplainsWhy(t *testing.T) {
	h, dir := chromeRepo(t, 1280, 800)
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "new.txt"), "n\n")
	h.Git(dir, "add", "new.txt")
	h.u.Refresh()
	h.tt.Frame()
	h.selectFile(workflow.Staged, "new.txt")
	h.want("New files are staged and unstaged as a whole file.")
	if h.tt.HasText("Stage chunk") || h.tt.HasText("Unstage chunk") || h.tt.HasText("Hunk 1 of") {
		t.Fatalf("whole-file diff offered hunk actions: %q", h.tt.Texts())
	}
}
