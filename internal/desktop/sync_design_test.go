package desktop

import (
	"path/filepath"
	"testing"

	"gitowl/internal/gittest"
)

// TestBranchBadgeIsTextOnlyPill checks the toolbar branch capsule is
// plain text at chromeBranchHeight, with no branch icon control.
func TestBranchBadgeIsTextOnlyPill(t *testing.T) {
	h := newHarness(t, 1280, 800)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	h.open(dir)
	r, ok := h.tt.Find("main")
	if !ok {
		t.Fatalf("branch text not found in %q", h.tt.Texts())
	}
	if r.H > chromeBranchHeight+4 {
		t.Fatalf("branch text height %v exceeds the %d capsule", r.H, chromeBranchHeight)
	}
	snapshot(t, h.tt, "branch-badge")
}
