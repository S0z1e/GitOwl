package desktop

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// TestWindowWelcomeUsesCustomShell is ticket 15's first tracer: the
// product window() is the custom chrome shell, not Onevra's plain header.
func TestWindowWelcomeUsesCustomShell(t *testing.T) {
	u := New(Options{Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := ui.NewTester(window(u), 1280, 800)
	tt.Frame()
	for _, want := range []string{
		"No repository open",
		"Pick a repository from the left, or open one from disk to review its changes, history and branches.",
		"Open Repository…",
		"or drop a folder anywhere in this window",
		"Pinned",
		"Repositories",
		"Search",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Find("GitOwl brand"); !ok {
		t.Fatalf("owl missing in %q", tt.Texts())
	}
	for _, s := range []string{
		"Close Repository and Return to Welcome",
		"Open an existing local Git repository to review its changes, history and branches.",
	} {
		if tt.HasText(s) {
			t.Fatalf("Onevra welcome leaked %q in %q", s, tt.Texts())
		}
	}
}

// TestWindowOpenUsesCustomShell asserts Current changes, the commit box
// and All commits after a repository opens, all on chromeWindow.
func TestWindowOpenUsesCustomShell(t *testing.T) {
	g := gittest.New(t)
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	h := &harness{Env: g, u: u, tt: ui.NewTester(window(u), 1280, 800)}
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "s\n")
	h.Commit(dir, "Initial")
	gittest.Write(t, filepath.Join(dir, "seed.txt"), "edit\n")
	h.open(dir)

	h.want("Current changes", "All commits", "Unstaged changes", "seed.txt",
		"Summary", "Description", "Generate Draft", "Amend previous commit", "Commit",
		"Fetch", "Pull", "Push")
	for _, s := range []string{"Close Repository and Return to Welcome", "No staged changes", "1 unstaged change"} {
		if h.tt.HasText(s) {
			t.Fatalf("Onevra review leaked %q in %q", s, h.tt.Texts())
		}
	}
	h.click("All commits")
	h.want("All commits", "Initial")
}
