package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// readState returns the bytes of the repository's HEAD and index files, so a
// test can prove RecentStatus changed neither.
func readState(t *testing.T, dir string) (head, index []byte) {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	// An unborn repository has no index yet; treat a missing one as empty.
	index, err = os.ReadFile(filepath.Join(gitDir, "index"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read index: %v", err)
	}
	return head, index
}

// assertReadOnly runs RecentStatus and fails if HEAD or the index changed.
func assertReadOnly(t *testing.T, app *Application, dir string) RecentStatus {
	t.Helper()
	head, index := readState(t, dir)
	got := app.RecentStatus(context.Background(), dir)
	head2, index2 := readState(t, dir)
	if string(head) != string(head2) {
		t.Fatalf("HEAD changed: %q -> %q", head, head2)
	}
	if string(index) != string(index2) {
		t.Fatal("index changed")
	}
	return got
}

func TestRecentStatusClean(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	got := assertReadOnly(t, e.app, dir)
	if got.Kind != RecentOK || got.Branch != "main" || got.Detached || got.Unborn {
		t.Fatalf("clean status %+v", got)
	}
	if got.Modified != 0 {
		t.Fatalf("clean modified = %d, want 0", got.Modified)
	}
}

func TestRecentStatusCountsSameFileInTwoAreasOnce(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "both.txt"), "one\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "both.txt"), "two\n")
	e.git(dir, "add", "both.txt")
	write(t, filepath.Join(dir, "both.txt"), "three\n")
	got := assertReadOnly(t, e.app, dir)
	if got.Kind != RecentOK || got.Modified != 1 {
		t.Fatalf("staged+unstaged same file %+v", got)
	}
}

func TestRecentStatusCountsUntracked(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "a.txt"), "changed\n")
	write(t, filepath.Join(dir, "new.txt"), "new\n")
	got := assertReadOnly(t, e.app, dir)
	if got.Kind != RecentOK || got.Modified != 2 {
		t.Fatalf("untracked counted %+v", got)
	}
}

func TestRecentStatusDetached(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	head := e.git(dir, "rev-parse", "HEAD")
	head = head[:len(head)-1] // drop trailing newline
	e.git(dir, "checkout", "-q", "--detach", "HEAD")
	got := assertReadOnly(t, e.app, dir)
	if got.Kind != RecentOK || !got.Detached || got.Branch != "" || got.Unborn {
		t.Fatalf("detached status %+v", got)
	}
	if got.Short != head[:7] {
		t.Fatalf("short hash %q, want %q", got.Short, head[:7])
	}
}

func TestRecentStatusUnborn(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	got := assertReadOnly(t, e.app, dir)
	if got.Kind != RecentOK || !got.Unborn || got.Branch != "main" || got.Detached {
		t.Fatalf("unborn status %+v", got)
	}
	if got.Modified != 0 || got.Short != "" {
		t.Fatalf("unborn fields %+v", got)
	}
}

func TestRecentStatusMissing(t *testing.T) {
	e := newEnv(t)
	got := e.app.RecentStatus(context.Background(), filepath.Join(t.TempDir(), "gone"))
	if got.Kind != RecentMissing {
		t.Fatalf("missing path kind %v", got.Kind)
	}
	if empty := e.app.RecentStatus(context.Background(), ""); empty.Kind != RecentMissing {
		t.Fatalf("empty path kind %v", empty.Kind)
	}
}

func TestRecentStatusPlainFolder(t *testing.T) {
	e := newEnv(t)
	got := e.app.RecentStatus(context.Background(), t.TempDir())
	if got.Kind != RecentNotRepository {
		t.Fatalf("plain folder kind %v", got.Kind)
	}
}

func TestRecentStatusUnreadableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	e := newEnv(t)
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	got := e.app.RecentStatus(context.Background(), locked)
	if got.Kind != RecentPermissionDenied {
		t.Fatalf("unreadable folder kind %v", got.Kind)
	}
}
