package desktop

import (
	"path/filepath"
	"testing"

	"gitowl/internal/gittest"
)

// stageOne writes file in the worktree and stages it, so the commit panel
// has something to commit.
func (h *harness) stageOne(dir, file, content string) {
	h.T.Helper()
	gittest.Write(h.T, filepath.Join(dir, file), content)
	h.Git(dir, "add", file)
}

// TestPlainCommitDoesNotPush checks the ordinary Commit still commits
// locally only, leaving the remote untouched.
func TestPlainCommitDoesNotPush(t *testing.T) {
	h := newHarness(t, 1280, 800)
	bare, mine, _ := clones(t, h.Env)
	before := h.Git(bare, "rev-parse", "main")
	h.open(mine)
	h.stageOne(mine, "feature.txt", "feature\n")
	h.u.Refresh()
	h.tt.Frame()
	h.typeMessage("Local only")
	h.click("Commit")
	if h.Git(bare, "rev-parse", "main") != before {
		t.Fatal("plain Commit pushed to the remote")
	}
	if h.Git(mine, "log", "-1", "--format=%s") != "Local only\n" {
		t.Fatal("plain Commit did not commit locally")
	}
}

// commitCount returns the number of commits reachable from HEAD.
func (h *harness) commitCount(dir string) int {
	h.T.Helper()
	out := h.Git(dir, "rev-list", "--count", "HEAD")
	n := 0
	for _, r := range out {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		}
	}
	return n
}
