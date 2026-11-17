package desktop

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitowl/internal/gittest"
)

func TestRefTreeFoldersAndDepth(t *testing.T) {
	nodes := refTree([]string{"main", "feature/ui/tree", "feature/api", "a/b/c/d/e/f", "fix"})
	var got []string
	var show func(ns []*refNode, depth int)
	show = func(ns []*refNode, depth int) {
		for _, n := range ns {
			kind := "-"
			if n.folder {
				kind = "+"
			}
			got = append(got, strings.Repeat(" ", depth)+kind+n.label+"="+n.path)
			show(n.children, depth+1)
		}
	}
	show(nodes, 0)
	want := []string{
		"+a=a", " +b=a/b", "  +c=a/b/c", "   -d/e/f=a/b/c/d/e/f",
		"+feature=feature", " +ui=feature/ui", "  -tree=feature/ui/tree", " -api=feature/api",
		"-fix=fix", "-main=main",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("tree:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestLongBranchNamesInTheDrawer keeps long names in one line each in the
// drawer of a small window, with their actions reachable.
func TestLongBranchNamesInTheDrawer(t *testing.T) {
	h := newHarness(t, 560, 480)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Initial")
	long := "release/" + strings.Repeat("an-extremely-long-branch-name-", 4) + "end"
	h.Git(dir, "branch", long)
	h.open(dir)
	h.openBranchFolders()
	h.click(sidebarShowLabel(h.u))
	h.tt.Advance(time.Second) // the drawer slides in before its rows show
	r, ok := h.tt.Find(long)
	if !ok {
		t.Fatalf("no %q in %q", long, h.tt.Texts())
	}
	if r.H > rowHeight+8 {
		t.Fatalf("row %v wraps", r)
	}
	snapshot(t, h.tt, "branch-drawer-long")
	h.chooseBranchMenu(long, "Checkout")
	if h.u.snap.Branch != long {
		t.Fatalf("on %s", h.u.snap.Branch)
	}
}

// shown reports a text on screen exactly as s.
func shown(h *harness, s string) bool {
	_, ok := h.tt.Find(s)
	return ok
}
