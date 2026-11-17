package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// longLines is numbered lines with the lines in long replaced by a line of
// 300 characters.
func longLines(n int, long ...int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		skip := false
		for _, l := range long {
			if l == i {
				fmt.Fprintf(&b, "const line%d = %q\n", i, strings.Repeat("x", 300))
				skip = true
			}
		}
		if !skip {
			fmt.Fprintf(&b, "line %d\n", i)
		}
	}
	return b.String()
}

// TestHunkCardStagesTheSecondHunkOfALongLineDiff opens a diff whose lines
// are longer than the window: each hunk is its own card whose Stage chunk
// button and Change navigation stay on screen, without pointing at
// anything, and the second card stages only its hunk.
func TestHunkCardStagesTheSecondHunkOfALongLineDiff(t *testing.T) {
	for _, size := range [][2]int{{1280, 800}, {600, 600}} {
		t.Run(itoa(size[0]), func(t *testing.T) {
			h := newHarness(t, size[0], size[1])
			dir := h.Repo()
			gittest.Write(t, filepath.Join(dir, "f.go"), longLines(30))
			h.Commit(dir, "Initial")
			edited := longLines(30, 2, 25)
			gittest.Write(t, filepath.Join(dir, "f.go"), edited)
			h.open(dir)
			h.selectFile(workflow.Unstaged, "f.go")

			// No hover: the first card's action is in the window, undisturbed by
			// the 300-character code lines that wrap in the code column beside
			// them.
			hint1 := h.u.t("hunkActionHint", "index", 1, "count", 2)
			for _, s := range []string{"Stage chunk · " + hint1} {
				r, ok := h.tt.Find(s)
				if !ok || r.X < 0 || r.X+r.W > float32(size[0]) || r.Y+r.H > float32(size[1]) {
					t.Fatalf("%q at %v (found %v) in a %v window", s, r, ok, size)
				}
			}
			if h.tt.HasText("Stage This Hunk") {
				t.Fatal("an old hover action shows")
			}
			snapshot(t, h.tt, "hunk-card-"+itoa(size[0]))
			// Stage the second hunk (line 25) from its own card.
			h.stageHunk(true, 2, 2)
			if got := h.Git(dir, "show", ":f.go"); got != longLines(30, 25) {
				t.Fatalf("index:\n%s", got)
			}
			if data, _ := os.ReadFile(filepath.Join(dir, "f.go")); string(data) != edited {
				t.Fatal("the file changed")
			}
			// One hunk is left in the diff.
			if n := len(h.u.diffChanges(h.u.wt.diff)); n != 1 {
				t.Fatalf("%d hunks left, want 1", n)
			}
			if s := h.u.wt.selected; s == nil || s.area != workflow.Unstaged {
				t.Fatalf("selection %+v", s)
			}
		})
	}
}

// TestHunkCardFromTheKeyboard reaches a card's action with Tab and stages
// with it.
func TestHunkBarFromTheKeyboard(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"}))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	h.tt.SetFocused(true)
	// Two hunks; Tab reaches the first card's action and Space stages it.
	h.tabTo("Stage chunk · " + h.u.t("hunkActionHint", "index", 1, "count", 2))
	h.press(ui.KeySpace)
	if got := h.Git(dir, "show", ":f.txt"); got != numbered(30, map[int]string{2: "two\n"}) {
		t.Fatalf("index:\n%s", got)
	}
	// The staged side names its hunk too, and unstages it.
	h.selectFile(workflow.Staged, "f.txt")
	h.want("Unstage chunk")
	h.stageHunk(false, 1, 1)
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("still staged %q", got)
	}
	// Nothing of the file is staged: the selection follows it back.
	if s := h.u.wt.selected; s == nil || s.area != workflow.Unstaged || s.path != "f.txt" {
		t.Fatalf("selection %+v", s)
	}
}

// TestHunkCardRefusesAStaleHunk stages nothing when the file changed after
// its diff was read, and counts the changes of the new diff.
func TestHunkCardRefusesAStaleHunk(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, nil))
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n"}))
	h.open(dir)
	h.selectFile(workflow.Unstaged, "f.txt")
	gittest.Write(t, filepath.Join(dir, "f.txt"), numbered(30, map[int]string{2: "two\n", 25: "twenty-five\n"}))
	h.stageHunk(true, 1, 1)
	h.want("The file changed since its diff was shown, so nothing was staged. GitOwl read it again; choose the hunk again.")
	if got := h.Git(dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("staged %q", got)
	}
}

// TestDiffGridKeepsNumbersBesideWrappedCode draws the design's four-column
// grid (44|44|24|rest): long lines wrap inside the code column instead of
// scrolling sideways, so the line numbers keep their own columns to the
// left of the code and never sit under it, in both appearances and at a
// narrow width. The reference row height is 22 and the number column 44.
func TestDiffGridKeepsNumbersBesideWrappedCode(t *testing.T) {
	for _, size := range [][2]int{{1360, 820}, {600, 600}} {
		for _, dark := range []bool{false, true} {
			name := itoa(size[0])
			if dark {
				name += "-dark"
			}
			t.Run(name, func(t *testing.T) {
				h := newHarness(t, size[0], size[1])
				h.tt.SetDark(dark)
				dir := h.Repo()
				gittest.Write(t, filepath.Join(dir, "f.go"), longLines(12, 3, 4))
				h.Commit(dir, "Initial")
				edited := strings.Replace(longLines(12, 3, 4), "line 5\n", "changed 5\n", 1)
				edited = strings.Replace(edited, "const line3 = \"x", "const line3 = \"y", 1)
				gittest.Write(t, filepath.Join(dir, "f.go"), edited)
				h.open(dir)
				h.selectFile(workflow.Unstaged, "f.go")
				patch := h.u.wt.diff.info.Patch

				// Two context lines one row apart: their row pitch is the
				// design's 28 DIPs.
				r1, ok1 := h.tt.Find("line 1")
				r2, ok2 := h.tt.Find("line 2")
				if !ok1 || !ok2 {
					t.Fatalf("no context rows in %q", h.tt.Texts())
				}
				if pitch := r2.Y - r1.Y; pitch < 27 || pitch > 29 {
					t.Fatalf("row pitch %v, want ~%d", pitch, diffLineH)
				}
				// A line number sits in its own column to the left of the
				// code and does not overlap it; the code wraps within its
				// own column, so there is no sideways scroll under the
				// numbers.
				numR, okN := h.tt.Find("1")
				if !okN {
					t.Fatalf("no line number in %q", h.tt.Texts())
				}
				if numR.X+numR.W > r1.X {
					t.Fatalf("number %v overlaps code %v", numR, r1)
				}
				snapshot(t, h.tt, "diff-grid-"+name)
				// The hunk card's Stage chunk action is present, and the
				// diff is unchanged by rendering.
				if _, ok := h.tt.Find("Stage chunk · " + h.u.t("hunkActionHint", "index", 1, "count", 1)); !ok {
					t.Fatalf("no hunk card action in %q", h.tt.Texts())
				}
				if h.u.wt.diff.info.Patch != patch {
					t.Fatal("the diff changed")
				}
			})
		}
	}
}

// TestDiffExplainsFilesWithoutText keeps the explanations of binary
// files, nested repositories and diffs over the size limit, and of no
// selection.
func TestDiffExplainsFilesWithoutText(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "logo.png"), "\x89PNG\x00\x00binary")
	gittest.Write(t, filepath.Join(dir, "big.txt"), strings.Repeat("0123456789abcdef\n", workflow.MaxPatchBytes/16+1))
	nested := filepath.Join(dir, "nested")
	gittest.Write(t, filepath.Join(nested, "n.txt"), "n\n")
	h.Git(nested, "init", "-q")
	h.open(dir)
	for path, want := range map[string]string{"logo.png": "Binary file", "big.txt": "Diff too large", "nested/": "Nested repository"} {
		h.selectFile(workflow.Untracked, path)
		h.want(want)
		if h.tt.HasText("Stage chunk") {
			t.Fatalf("%s offers a hunk", path)
		}
	}
}
