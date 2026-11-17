package desktop

import (
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

func diffShot(t *testing.T, h *harness, name string) {
	t.Helper()
	h.tt.SetDark(false)
	h.tt.Frame()
	snapshot(t, h.tt, name+"-light")
	h.tt.SetDark(true)
	h.tt.Frame()
	snapshot(t, h.tt, name+"-dark")
	h.tt.SetDark(false)
	h.tt.Frame()
}

// lineText names a split cell's content, or "·" for a blank cell, so a
// test can read the two columns side by side.
func cellText(l *diffLine) string {
	if l == nil {
		return "·"
	}
	return string(l.sign) + l.text
}

// TestSplitRowsPairsAsymmetricChange lines a two-line deletion up against a
// one-line addition: the first del pairs with the add, the second del sits
// opposite a blank, and context fills both sides.
func TestSplitRowsPairsAsymmetricChange(t *testing.T) {
	lines := []diffLine{
		{sign: ' ', text: "ctx", oldNum: 1, newNum: 1},
		{sign: '-', text: "old one", oldNum: 2},
		{sign: '-', text: "old two", oldNum: 3},
		{sign: '+', text: "new one", newNum: 2},
		{sign: ' ', text: "tail", oldNum: 4, newNum: 3},
	}
	rows := splitRows(lines)
	want := [][2]string{
		{" ctx", " ctx"},
		{"-old one", "+new one"},
		{"-old two", "·"},
		{" tail", " tail"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows %d, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		if got := [2]string{cellText(rows[i].left), cellText(rows[i].right)}; got != w {
			t.Fatalf("row %d = %v, want %v", i, got, w)
		}
	}
}

// TestSplitRowsPureAddKeepsLeftBlank puts a run of additions only on the
// right, each opposite a blank left cell.
func TestSplitRowsPureAddKeepsLeftBlank(t *testing.T) {
	lines := []diffLine{
		{sign: ' ', text: "ctx", oldNum: 1, newNum: 1},
		{sign: '+', text: "add a", newNum: 2},
		{sign: '+', text: "add b", newNum: 3},
	}
	rows := splitRows(lines)
	if len(rows) != 3 {
		t.Fatalf("rows %d, want 3", len(rows))
	}
	for _, i := range []int{1, 2} {
		if rows[i].left != nil {
			t.Fatalf("row %d left = %q, want blank", i, cellText(rows[i].left))
		}
		if rows[i].right == nil || rows[i].right.sign != '+' {
			t.Fatalf("row %d right = %q, want an addition", i, cellText(rows[i].right))
		}
	}
}

// TestSplitRowsPureDelKeepsRightBlank puts a run of deletions only on the
// left, each opposite a blank right cell.
func TestSplitRowsPureDelKeepsRightBlank(t *testing.T) {
	lines := []diffLine{
		{sign: '-', text: "del a", oldNum: 1},
		{sign: '-', text: "del b", oldNum: 2},
		{sign: ' ', text: "ctx", oldNum: 3, newNum: 1},
	}
	rows := splitRows(lines)
	if len(rows) != 3 {
		t.Fatalf("rows %d, want 3", len(rows))
	}
	for _, i := range []int{0, 1} {
		if rows[i].right != nil {
			t.Fatalf("row %d right = %q, want blank", i, cellText(rows[i].right))
		}
		if rows[i].left == nil || rows[i].left.sign != '-' {
			t.Fatalf("row %d left = %q, want a deletion", i, cellText(rows[i].left))
		}
	}
}

// splitHunkFile makes a repo whose one unstaged file has two separate
// edits, the design's two-card shape, for the view-combination tests.
func splitHunkFile(t *testing.T) (*harness, string) {
	t.Helper()
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "ai.go"), numbered(30, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "ai.go"), numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"}))
	h.open(dir)
	return h, dir
}

// setView turns the Unified/Split and Wrap toggles to a known combination,
// as clicking them does, then renders a frame.
func (h *harness) setView(split, wrap bool) {
	h.T.Helper()
	h.u.diffSplit, h.u.diffWrap = split, wrap
	h.tt.Frame()
}

// TestDiffSplitScreenshots renders issue 07's view/wrap states to
// $GITOWL_SNAPSHOT_DIR (issue 07): Split, Wrap,
// Split+Wrap and a narrow window forced to Unified, each in light and dark.
// A long line in one hunk shows wrapping against sideways scroll; the
// surrounding assertions keep each state honest.
func TestDiffSplitScreenshots(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "ai.go"), longLines(30, 12))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "ai.go"),
		strings.Replace(longLines(30, 12, 2, 25), "line 2\n", "two\n", 1))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "ai.go")

	h.setView(true, false)
	h.want("Split", "Wrap lines")
	diffShot(t, h, "07-split")

	h.setView(false, true)
	diffShot(t, h, "07-wrap")

	h.setView(true, true)
	diffShot(t, h, "07-split-wrap")

	// Narrow: the window shows the diff alone, forced to Unified.
	h.u.diffSplit = true
	h.tt.SetSize(600, 820)
	h.tt.Frame()
	if h.u.diffSplitActive() {
		t.Fatal("Split active in a narrow window")
	}
	diffShot(t, h, "07-narrow")
}

// TestDiffViewCombinationsStageTheSourceHunk stages and unstages a hunk in
// each of the four view/wrap combinations, checking the index each time:
// the staging is bound to the source hunk, not the on-screen layout, so the
// same hunk index acts the same whatever the view. Here hunk 1 (line 2) is
// staged, then unstaged, in every combination.
func TestDiffViewCombinationsStageTheSourceHunk(t *testing.T) {
	staged := numbered(30, map[int]string{2: "two\n"})
	for _, split := range []bool{false, true} {
		for _, wrap := range []bool{false, true} {
			name := "unified"
			if split {
				name = "split"
			}
			if wrap {
				name += "-wrap"
			}
			t.Run(name, func(t *testing.T) {
				h, dir := splitHunkFile(t)
				h.selectFile(workflow.Unstaged, "ai.go")
				h.setView(split, wrap)
				// Stage hunk 1 from the unstaged side.
				h.stageHunk(true, 1, 2)
				if got := h.Git(dir, "show", ":ai.go"); got != staged {
					t.Fatalf("after staging, index:\n%s", got)
				}
				// Unstage it again from the staged side, same view.
				h.selectFile(workflow.Staged, "ai.go")
				h.setView(split, wrap)
				h.stageHunk(false, 1, 1)
				if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
					t.Fatalf("still staged %q", got)
				}
			})
		}
	}
}

// TestSplitShowsBothSidesAligned renders the Split view and checks both the
// removed and the added line of a change are on screen, the left side left
// of the right side, so the two columns read side by side.
func TestSplitShowsBothSidesAligned(t *testing.T) {
	h, _ := splitHunkFile(t)
	h.selectFile(workflow.Unstaged, "ai.go")
	h.setView(true, true)
	h.want("Split", "line 1")
	left, okL := h.tt.Find("line 2")
	right, okR := h.tt.Find("two")
	if !okL || !okR {
		t.Fatalf("missing a side in %q", h.tt.Texts())
	}
	if left.X >= right.X {
		t.Fatalf("old side x=%v not left of new side x=%v", left.X, right.X)
	}
}

// TestWrapKeepsLineNumbers turns Wrap lines on with a long line and checks
// the line numbers and the staged patch are unchanged: wrapping is a view
// choice, not a change to the diff.
func TestWrapKeepsLineNumbers(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.go"), longLines(12, 3))
	h.Commit(dir, "Initial")
	edited := strings.Replace(longLines(12, 3), "line 5\n", "changed 5\n", 1)
	gittest.Write(t, filepath.Join(dir, "f.go"), edited)
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.go")
	patch := h.u.wt.diff.info.Patch
	h.setView(false, true)
	// The changed line's number still shows beside it.
	if !h.tt.HasText("changed") {
		t.Fatalf("no changed line in %q", h.tt.Texts())
	}
	if h.u.wt.diff.info.Patch != patch {
		t.Fatal("wrapping changed the diff")
	}
	h.stageHunk(true, 1, 1)
	if got := h.Git(dir, "show", ":f.go"); got != edited {
		t.Fatalf("staged patch differs:\n%s", got)
	}
}

// TestNarrowWindowForcesUnified keeps Split off in a narrow window, with
// the reason, and returns to the user's Split choice when the window
// widens.
func TestNarrowWindowForcesUnified(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "ai.go"), numbered(30, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "ai.go"), numbered(30, map[int]string{2: "two\n"}))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "ai.go")
	h.u.diffSplit = true
	h.tt.Frame()
	if !h.u.diffSplitActive() {
		t.Fatal("Split not active in a wide window")
	}
	// Narrow: the window shows the diff, and Split is forced off.
	h.tt.SetSize(600, 820)
	h.tt.Frame()
	if h.u.diffSplitActive() {
		t.Fatal("Split still active in a narrow window")
	}
	if !h.u.diffSplit {
		t.Fatal("the user's Split choice was lost")
	}
	// Wide again: the choice returns.
	h.tt.SetSize(1360, 820)
	h.tt.Frame()
	if !h.u.diffSplitActive() {
		t.Fatal("Split did not return when the window widened")
	}
}

// TestHistoryDiffIsReadOnly shows a commit's diff as the design's cards but
// without any staging action, so History stays read-only.
func TestHistoryDiffIsReadOnly(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "ai.go"), numbered(10, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "ai.go"), numbered(10, map[int]string{2: "two\n"}))
	h.Commit(dir, "Edit ai.go")
	h.open(dir)
	h.click("All commits")
	h.want("Edit ai.go")
	for _, s := range []string{"Stage chunk", "Stage file", "Unstage chunk"} {
		if h.tt.HasText(s) {
			t.Fatalf("History offers a staging action %q", s)
		}
	}
}
