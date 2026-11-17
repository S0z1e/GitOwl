package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSummarizesWorkingTree(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "tracked.txt"), "one\n")
	write(t, filepath.Join(dir, "both.txt"), "one\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "tracked.txt"), "two\n")
	write(t, filepath.Join(dir, "both.txt"), "two\n")
	e.git(dir, "add", "both.txt")
	write(t, filepath.Join(dir, "both.txt"), "three\n")
	write(t, filepath.Join(dir, "new dir", "new file.txt"), "new\n")

	r, snap := e.open(dir)
	if r.Root() != dir || snap.Root != dir {
		t.Fatalf("root %q %q, want %q", r.Root(), snap.Root, dir)
	}
	if snap.Branch != "main" || snap.Unborn() || snap.Detached {
		t.Fatalf("branch %+v", snap)
	}
	if got := join(paths(snap.Staged)); got != "both.txt" {
		t.Errorf("staged %s", got)
	}
	if got := join(paths(snap.Unstaged)); got != "both.txt,tracked.txt" {
		t.Errorf("unstaged %s", got)
	}
	if got := join(paths(snap.Untracked)); got != "new dir/new file.txt" {
		t.Errorf("untracked %s", got)
	}
}

func TestOpenRepositoryWithoutCommits(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	_, snap := e.open(dir)
	if !snap.Unborn() || snap.Branch != "main" || !snap.Clean() {
		t.Fatalf("unborn snapshot %+v", snap)
	}
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	_, snap = e.open(dir)
	if got := join(paths(snap.Staged)); got != "a.txt" {
		t.Fatalf("staged in unborn repository: %s", got)
	}
}

func TestOpenResolvesSubdirectoryToRoot(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	r, _ := e.open(sub)
	if r.Root() != dir {
		t.Fatalf("root %q, want %q", r.Root(), dir)
	}
}

func TestOpenRenameWithSpacesCountsOnce(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "old name.txt"), "same content\nfor rename detection\n")
	e.commit(dir, "Initial")
	e.git(dir, "mv", "old name.txt", "new name.txt")
	_, snap := e.open(dir)
	if len(snap.Staged) != 1 || snap.Staged[0].Path != "new name.txt" || snap.Staged[0].OrigPath != "old name.txt" {
		t.Fatalf("staged rename %+v", snap.Staged)
	}
	if len(snap.Unstaged) != 0 || len(snap.Untracked) != 0 {
		t.Fatalf("rename counted elsewhere: %+v %+v", snap.Unstaged, snap.Untracked)
	}
}

func TestOpenRejectsInvalidLocations(t *testing.T) {
	e := newEnv(t)
	plain := t.TempDir()
	file := filepath.Join(plain, "file.txt")
	write(t, file, "x")
	locked := filepath.Join(plain, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	bare := filepath.Join(t.TempDir(), "bare.git")
	e.git(plain, "init", "-q", "--bare", bare)

	for _, tc := range []struct {
		path string
		code Code
	}{
		{filepath.Join(plain, "missing"), CodePathNotFound},
		{file, CodeNotDirectory},
		{plain, CodeNotRepository},
		{bare, CodeBareRepository},
	} {
		_, _, err := e.app.Open(context.Background(), tc.path)
		wantCode(t, err, tc.code)
	}
	if os.Geteuid() != 0 {
		_, _, err := e.app.Open(context.Background(), locked)
		wantCode(t, err, CodeNoAccess)
	}
	if e.app.Current() != nil {
		t.Fatal("a failed open became current")
	}
}

func TestOpenFailureKeepsCurrentRepository(t *testing.T) {
	e := newEnv(t)
	r, _ := e.open(e.repo())
	if _, _, err := e.app.Open(context.Background(), t.TempDir()); err == nil {
		t.Fatal("plain directory opened")
	}
	if !r.Current() {
		t.Fatal("failed open closed the current repository")
	}
}

func TestOpenWithoutGit(t *testing.T) {
	app := New(nil)
	_, _, err := app.Open(context.Background(), t.TempDir())
	wantCode(t, err, CodeGitMissing)
}

func TestCloseEndsHandleAndPendingOpens(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "a.txt"), "b\n")
	repo, snap := e.open(dir)
	index := e.git(dir, "ls-files", "-s")
	e.app.Close()
	if repo.Current() || e.app.Current() != nil {
		t.Fatal("still current after Close")
	}
	if _, err := repo.Stage(context.Background(), snap.Unstaged[0]); CodeOf(err) != CodeRepositoryClosed {
		t.Fatalf("closed handle staged: %v", err)
	}
	if e.git(dir, "ls-files", "-s") != index || readFile(t, filepath.Join(dir, "a.txt")) != "b\n" {
		t.Fatal("Close changed the repository")
	}
	// A new open works and is a new handle.
	again, _ := e.open(dir)
	if again == repo || !again.Current() {
		t.Fatal("reopen")
	}
}

func TestCloseMakesRequestedOpenStale(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	req := e.app.RequestOpen()
	e.app.Close()
	_, _, err := e.app.OpenRequested(context.Background(), req, dir)
	wantCode(t, err, CodeRepositoryClosed)
	if e.app.Current() != nil {
		t.Fatal("stale open became current")
	}
}
