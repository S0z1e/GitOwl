package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageAndUnstageWholeFiles(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	write(t, filepath.Join(dir, "gone.txt"), "bye\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "a.txt"), "two\n")
	os.Remove(filepath.Join(dir, "gone.txt"))
	write(t, filepath.Join(dir, "new file.txt"), "new\n")
	r, snap := e.open(dir)
	ctx := context.Background()

	snap, err := r.Stage(ctx, find(t, snap.Unstaged, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := e.git(dir, "show", ":a.txt"); got != "two\n" {
		t.Fatalf("index after stage %q", got)
	}
	snap, err = r.Stage(ctx, find(t, snap.Unstaged, "gone.txt"))
	if err != nil {
		t.Fatal(err)
	}
	snap, err = r.Stage(ctx, find(t, snap.Untracked, "new file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := join(paths(snap.Staged)); got != "a.txt,gone.txt,new file.txt" {
		t.Fatalf("staged %s", got)
	}
	if len(snap.Unstaged)+len(snap.Untracked) != 0 {
		t.Fatalf("left over %v %v", paths(snap.Unstaged), paths(snap.Untracked))
	}

	snap, err = r.Unstage(ctx, find(t, snap.Staged, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := e.git(dir, "show", ":a.txt"); got != "one\n" {
		t.Fatalf("index after unstage %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "two\n" {
		t.Fatalf("working tree changed: %q", data)
	}
	snap, err = r.Unstage(ctx, find(t, snap.Staged, "new file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	find(t, snap.Untracked, "new file.txt")
}

func TestStagePartOfMixedFileStagesWholeFile(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "m.txt"), "1\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "m.txt"), "2\n")
	e.git(dir, "add", "m.txt")
	write(t, filepath.Join(dir, "m.txt"), "3\n")
	r, snap := e.open(dir)
	snap, err := r.Stage(context.Background(), find(t, snap.Unstaged, "m.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := e.git(dir, "show", ":m.txt"); got != "3\n" || len(snap.Unstaged) != 0 {
		t.Fatalf("index %q, unstaged %v", got, paths(snap.Unstaged))
	}
}

func TestUnstageInRepositoryWithoutCommitsKeepsFile(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "first.txt"), "keep me\n")
	e.git(dir, "add", "first.txt")
	r, snap := e.open(dir)
	snap, err := r.Unstage(context.Background(), find(t, snap.Staged, "first.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Staged) != 0 || join(paths(snap.Untracked)) != "first.txt" {
		t.Fatalf("after unstage %+v", snap)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "first.txt")); string(data) != "keep me\n" {
		t.Fatalf("file content %q", data)
	}
}

func TestUnstageRenameRestoresBothPaths(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "old.txt"), "a\nb\nc\nd\n")
	e.commit(dir, "Initial")
	e.git(dir, "mv", "old.txt", "new.txt")
	r, snap := e.open(dir)
	snap, err := r.Unstage(context.Background(), find(t, snap.Staged, "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Staged) != 0 {
		t.Fatalf("still staged %v", paths(snap.Staged))
	}
	find(t, snap.Unstaged, "old.txt")
	find(t, snap.Untracked, "new.txt")
}

func TestStaleChangeIsNotApplied(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "1\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "a.txt"), "2\n")
	r, snap := e.open(dir)
	ch := find(t, snap.Unstaged, "a.txt")
	// Outside GitOwl, the change is staged and then reverted in the worktree.
	e.git(dir, "add", "a.txt")
	write(t, filepath.Join(dir, "a.txt"), "1\n")
	fresh, err := r.Stage(context.Background(), ch)
	wantCode(t, err, CodeStateChanged)
	if got := e.git(dir, "show", ":a.txt"); got != "2\n" {
		t.Fatalf("stale stage ran: index %q", got)
	}
	if len(fresh.Staged) != 1 || len(fresh.Unstaged) != 1 {
		t.Fatalf("fresh state %+v", fresh)
	}
	// A closed repository refuses changes.
	e.open(e.repo())
	_, err = r.Unstage(context.Background(), find(t, fresh.Staged, "a.txt"))
	wantCode(t, err, CodeRepositoryClosed)
}

func TestStageFailureReportsGit(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "1\n")
	r, snap := e.open(dir)
	lock := filepath.Join(dir, ".git", "index.lock")
	write(t, lock, "")
	_, err := r.Stage(context.Background(), find(t, snap.Untracked, "a.txt"))
	wantCode(t, err, CodeGitFailed)
	if !strings.Contains(err.Error(), "index.lock") {
		t.Fatalf("detail %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "1\n" {
		t.Fatal("file changed")
	}
}
