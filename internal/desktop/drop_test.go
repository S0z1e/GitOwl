package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// dropAt drops paths at the center of the window, the Landing being a
// drop target over its whole area, and renders the result.
func dropAt(tt *ui.Tester, paths ...string) bool {
	taken := tt.DropFiles(680, 410, paths...)
	tt.Frame()
	return taken
}

// TestDropFolderOpensRepository: a single valid repository folder dropped
// on the Landing opens it and records it in Recent.
func TestDropFolderOpensRepository(t *testing.T) {
	g := gittest.New(t)
	store := &savedPrefs{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, SavePreferences: store.save})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	dir := g.Repo()
	gittest.Write(t, filepath.Join(dir, "f.txt"), "one\n")
	g.Commit(dir, "Initial")

	if !dropAt(tt, dir) {
		t.Fatal("the Landing did not take the dropped folder")
	}
	if u.repo == nil || u.repo.Root() != dir {
		t.Fatalf("repository not opened: %+v (%+v)", u.repo, u.notice)
	}
	if len(store.p.Recent) != 1 || store.p.Recent[0] != dir {
		t.Fatalf("recent after drop: %q", store.p.Recent)
	}
}

// TestDropNonGitFolderExplained: a folder that is not a Git repository is
// explained and leaves the Landing with no Recent entry.
func TestDropNonGitFolderExplained(t *testing.T) {
	g := gittest.New(t)
	store := &savedPrefs{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, SavePreferences: store.save})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	plain := t.TempDir()

	dropAt(tt, plain)
	if u.repo != nil {
		t.Fatal("a non-Git folder opened a repository")
	}
	if !tt.HasText("That folder is not inside a Git repository.") {
		t.Fatalf("no explanation in %q", tt.Texts())
	}
	if len(store.p.Recent) != 0 {
		t.Fatalf("a failed drop recorded Recent: %q", store.p.Recent)
	}
}

// TestDropPlainFileRejected: a plain file (not a folder) is rejected with
// an explanation, and never passed to OpenPath.
func TestDropPlainFileRejected(t *testing.T) {
	g := gittest.New(t)
	store := &savedPrefs{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }, SavePreferences: store.save})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	file := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	dropAt(tt, file)
	if u.repo != nil || u.opening {
		t.Fatal("a plain file started an open")
	}
	if !tt.HasText("Drop a folder, not a file.") {
		t.Fatalf("no file explanation in %q", tt.Texts())
	}
	if len(store.p.Recent) != 0 {
		t.Fatalf("a rejected file recorded Recent: %q", store.p.Recent)
	}
}

// TestDropMultipleItemsRejected: more than one item is rejected with an
// explanation and opens nothing.
func TestDropMultipleItemsRejected(t *testing.T) {
	g := gittest.New(t)
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	a, b := g.Repo(), g.Repo()

	dropAt(tt, a, b)
	if u.repo != nil || u.opening {
		t.Fatal("multiple items started an open")
	}
	if !tt.HasText("Drop a single folder.") {
		t.Fatalf("no multiple-items explanation in %q", tt.Texts())
	}
}

// TestDropIgnoredWhileOpening: a drop is ignored while an open is in
// flight, so a slow open is not interrupted by a second drop.
func TestDropIgnoredWhileOpening(t *testing.T) {
	g := gittest.New(t)
	q := &queued{}
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: q.Go})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	a, b := g.Repo(), g.Repo()
	for _, d := range []string{a, b} {
		gittest.Write(t, filepath.Join(d, "f.txt"), "one\n")
		g.Commit(d, "Initial")
	}

	// First drop queues an open; it has not finished (work is queued).
	dropAt(tt, a)
	if !u.opening {
		t.Fatal("first drop did not start an open")
	}
	// A second drop while opening is ignored.
	dropAt(tt, b)
	q.drain()
	tt.Frame()
	if u.repo == nil || u.repo.Root() != a {
		t.Fatalf("second drop was not ignored: %+v", u.repo)
	}
}

// TestDropTwoInARowTakesLatest: two quick valid drops open the latest.
func TestDropTwoInARowTakesLatest(t *testing.T) {
	g := gittest.New(t)
	u := New(Options{Workflow: workflow.New(g.Runner), Preferred: []string{"en-US"}, Go: func(fn func()) { fn() }})
	tt := ui.NewTester(window(u), 1360, 820)
	tt.Frame()
	a, b := g.Repo(), g.Repo()
	for _, d := range []string{a, b} {
		gittest.Write(t, filepath.Join(d, "f.txt"), "one\n")
		g.Commit(d, "Initial")
	}

	// With synchronous work, the first drop finishes and clears opening;
	// the Landing is gone, so re-open from the menu to drop again is not
	// how a user drops twice. Instead close and drop b.
	dropAt(tt, a)
	if u.repo == nil || u.repo.Root() != a {
		t.Fatalf("first drop: %+v", u.repo)
	}
	u.CloseRepository()
	tt.Frame()
	dropAt(tt, b)
	if u.repo == nil || u.repo.Root() != b {
		t.Fatalf("second drop did not take b: %+v", u.repo)
	}
}
