package desktop

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
)

// TestWideShellFillsWindow checks that, with a repository open in a wide
// window, the review columns start under the toolbar and reach the bottom
// edge. The shell used to centre its content vertically, which left a band
// above the file list and under the commit box.
func TestWideShellFillsWindow(t *testing.T) {
	for _, size := range [][2]int{{1508, 944}, {1280, 800}, {1100, 700}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			h := newHarness(t, size[0], size[1])
			dir := h.Repo()
			gittest.Write(t, filepath.Join(dir, "a.txt"), "one\n")
			h.Commit(dir, "Initial")
			gittest.Write(t, filepath.Join(dir, "a.txt"), "one\ntwo\n")
			gittest.Write(t, filepath.Join(dir, "new.txt"), "new\n")
			h.open(dir)
			h.tt.Frame()
			h.tt.Frame()

			const toolbar = chromeToolbarHeight
			const slack = 40 // room for padding and the first row's inset
			h0 := float32(size[1])

			filter := mustFind(t, h, "Filter files")
			if filter.Y > toolbar+slack {
				t.Errorf("file list starts at y=%.0f, want under the %d DIP toolbar", filter.Y, toolbar)
			}
			// The diff header's controls end at the window's right padding.
			wrap := mustFind(t, h, "Wrap lines")
			if right := wrap.X + wrap.W; right < float32(size[0])-80 {
				t.Errorf("diff controls end at x=%.0f, want the diff pane's right edge near %d", right, size[0])
			}
			unified := mustFind(t, h, "Unified")
			if unified.Y > toolbar+slack {
				t.Errorf("diff header starts at y=%.0f, want under the toolbar", unified.Y)
			}
			commit := mustFind(t, h, "Commit")
			if bottom := commit.Y + commit.H; bottom < h0-slack {
				t.Errorf("commit box ends at y=%.0f, want at the bottom of the %.0f DIP window", bottom, h0)
			}
			snapshot(t, h.tt, fmt.Sprintf("shell-fill-%dx%d", size[0], size[1]))
		})
	}
}

func mustFind(t *testing.T, h *harness, s string) ui.Rect {
	t.Helper()
	r, ok := h.tt.Find(s)
	if !ok {
		t.Fatalf("%q not on screen", s)
	}
	return r
}

// TestToolbarGroupsFollowTheDesign checks the wide toolbar's two groups: the
// remote and stash actions, then the editor, terminal and finder actions, on
// one row in the design's order.
func TestToolbarGroupsFollowTheDesign(t *testing.T) {
	h := newHarness(t, 1508, 944)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	h.tt.Frame()
	h.tt.Frame()
	order := []string{"Fetch", "Pull", "Push", "Stash", "Editor", "Terminal", "Finder"}
	var prev ui.Rect
	for i, s := range order {
		r := mustFind(t, h, s)
		if i > 0 {
			if d := r.Y - prev.Y; d > 2 || d < -2 {
				t.Errorf("%s at y=%.0f, want the toolbar row at y=%.0f", s, r.Y, prev.Y)
			}
			if r.X <= prev.X {
				t.Errorf("%s at x=%.0f is not after the previous action at x=%.0f", s, r.X, prev.X)
			}
		}
		prev = r
	}
	snapshot(t, h.tt, "toolbar-groups-1508x944")
}
