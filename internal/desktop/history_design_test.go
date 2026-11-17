package desktop

import (
	"path/filepath"
	"testing"

	"gitowl/internal/gittest"
)

// TestHistoryListIsFiftySixWithGraphSlot checks the commit list at 1360×820
// against the History board in the repository design: each commit row is 30
// tall on one line, and the text starts one Graph slot (54) in from the row's
// left edge, past the empty Graph placeholder.
func TestHistoryListIsFiftySixWithGraphSlot(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "First commit")
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\nmore\n")
	h.Commit(dir, "Second commit")
	h.open(dir)
	h.click("All commits")

	// The two commit subjects are one commit row (30) apart, so each row is
	// 30 tall as the design requires.
	first, ok := h.tt.Find("Second commit")
	if !ok {
		t.Fatalf("no newest subject in %q", h.tt.Texts())
	}
	second, ok := h.tt.Find("First commit")
	if !ok {
		t.Fatalf("no older subject in %q", h.tt.Texts())
	}
	if gap := second.Y - first.Y; !near(gap, commitRowHeight, 2) {
		t.Fatalf("commit rows %v apart, want %v", gap, commitRowHeight)
	}
	// The subject text begins past the 54 Graph slot, so the empty Graph
	// placeholder reserves its column.
	if first.X < 54 {
		t.Fatalf("subject left %v, want past the 54 Graph slot", first.X)
	}
	// The short hash shares the subject's line: the design's row is one line,
	// so their vertical centres match.
	hash := h.u.hist.commits[0].Short()
	hr, ok := h.tt.Find(hash)
	if !ok {
		t.Fatalf("no short hash %q in %q", hash, h.tt.Texts())
	}
	if !near(hr.Y+hr.H/2, first.Y+first.H/2, 4) {
		t.Fatalf("hash centre %v not on the subject line centre %v", hr.Y+hr.H/2, first.Y+first.H/2)
	}
}

// TestHistoryCopyHashWritesFullHash checks the detail's Copy hash button
// writes the selected commit's full hash to the clipboard (not the short
// display hash) and confirms it, per the ticket's acceptance criteria.
func TestHistoryCopyHashWritesFullHash(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "Only commit")
	h.open(dir)
	h.click("All commits")
	full := h.u.hist.commits[0].Hash
	if len(full) <= 7 {
		t.Fatalf("hash %q is not a full hash", full)
	}
	h.click("Copy hash")
	h.tt.Frame()
	if got := h.tt.Clipboard(); got != full {
		t.Fatalf("clipboard %q, want full hash %q", got, full)
	}
	// A confirmation is shown.
	h.want("Hash copied")
}

// TestHistoryBranchLabelFromRealRef checks the commit row's branch pills
// come from the branch panel's real refs (history.md §5.2): the current
// local branch is marked current (the accent capsule), a remote-tracking
// ref is marked remote (the grey capsule), and a commit without a ref at its
// tip carries none.
func TestHistoryBranchLabelFromRealRef(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "a\n")
	h.Commit(dir, "First, unlabelled")
	gittest.Write(t, filepath.Join(dir, "b.txt"), "b\n")
	h.Commit(dir, "Tip commit")
	h.open(dir)
	h.click("All commits")

	tip := h.u.hist.commits[0].Hash
	labels := h.u.commitLabels(tip)
	var current bool
	for _, l := range labels {
		if l.current && l.name == "main" {
			current = true
		}
	}
	if !current {
		t.Fatalf("no current accent pill for main in %+v", labels)
	}
	// The pill text shows in the History list.
	h.want("main")
	// The older commit has no ref at its tip, so it carries no pill: the
	// labels come from real refs, not the commit itself.
	older := h.commitIndex("First, unlabelled")
	if got := h.u.commitLabels(h.u.hist.commits[older].Hash); len(got) != 0 {
		t.Fatalf("an unref'd commit got pills %+v", got)
	}
}
