package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func unstagedOf(t *testing.T, snap Snapshot, path string) Change {
	t.Helper()
	for _, c := range append(append([]Change{}, snap.Unstaged...), snap.Untracked...) {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("%s not unstaged in %+v", path, snap)
	return Change{}
}

func TestDiscardUnstagedRestoresAllTrackedFiles(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), "one\n")
	write(t, filepath.Join(dir, "other.txt"), "o\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), "staged\n")
	e.git(dir, "add", "f.txt")
	write(t, filepath.Join(dir, "f.txt"), "staged\nunwanted\n")
	write(t, filepath.Join(dir, "other.txt"), "other edit\n")
	write(t, filepath.Join(dir, "new.txt"), "untracked\n")
	index := e.git(dir, "ls-files", "-s")
	head := e.rev(dir, "HEAD")
	repo, _ := e.open(dir)
	snap, err := repo.DiscardUnstaged(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "f.txt")) != "staged\n" {
		t.Fatal("not restored to the staged version")
	}
	if readFile(t, filepath.Join(dir, "other.txt")) != "o\n" {
		t.Fatal("unstaged other.txt not restored")
	}
	if e.git(dir, "ls-files", "-s") != index || e.rev(dir, "HEAD") != head {
		t.Fatal("discard changed the index or HEAD")
	}
	if readFile(t, filepath.Join(dir, "new.txt")) != "untracked\n" {
		t.Fatal("untracked file touched")
	}
	if len(snap.Unstaged) != 0 {
		t.Fatalf("left %+v", snap.Unstaged)
	}
}

func TestDiscardKeepsStagedVersion(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), "one\n")
	write(t, filepath.Join(dir, "other.txt"), "o\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), "staged\n")
	e.git(dir, "add", "f.txt")
	write(t, filepath.Join(dir, "f.txt"), "staged\nunwanted\n")
	write(t, filepath.Join(dir, "other.txt"), "other edit\n")
	write(t, filepath.Join(dir, "new.txt"), "untracked\n")
	index := e.git(dir, "ls-files", "-s")
	head := e.rev(dir, "HEAD")
	repo, snap := e.open(dir)
	p, err := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "f.txt"))
	if err != nil || !p.Staged || p.Deleted || p.Path != filepath.Join(dir, "f.txt") {
		t.Fatalf("plan %+v %v", p, err)
	}
	snap, err = repo.Discard(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "f.txt")) != "staged\n" {
		t.Fatal("not restored to the staged version")
	}
	if e.git(dir, "ls-files", "-s") != index || e.rev(dir, "HEAD") != head {
		t.Fatal("discard changed the index or HEAD")
	}
	if readFile(t, filepath.Join(dir, "other.txt")) != "other edit\n" || readFile(t, filepath.Join(dir, "new.txt")) != "untracked\n" {
		t.Fatal("discard touched another file")
	}
	for _, c := range snap.Unstaged {
		if c.Path == "f.txt" {
			t.Fatal("f.txt still unstaged")
		}
	}
}

func TestDiscardRestoresDeletionModeAndBinary(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "sub", "gone.txt"), "g\n")
	write(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n")
	write(t, filepath.Join(dir, "img.bin"), "\x00\x01\x02")
	e.commit(dir, "Initial")
	os.RemoveAll(filepath.Join(dir, "sub"))
	os.Chmod(filepath.Join(dir, "run.sh"), 0o755)
	write(t, filepath.Join(dir, "img.bin"), "\x00\x09\x09")
	repo, snap := e.open(dir)
	for _, path := range []string{"sub/gone.txt", "run.sh", "img.bin"} {
		p, err := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, path))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if path == "sub/gone.txt" && !p.Deleted || path == "img.bin" && !p.Binary {
			t.Fatalf("%s plan %+v", path, p)
		}
		if snap, err = repo.Discard(context.Background(), p); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if readFile(t, filepath.Join(dir, "sub", "gone.txt")) != "g\n" || readFile(t, filepath.Join(dir, "img.bin")) != "\x00\x01\x02" {
		t.Fatal("not restored")
	}
	if st, _ := os.Stat(filepath.Join(dir, "run.sh")); st.Mode().Perm()&0o100 != 0 {
		t.Fatal("mode not restored")
	}
	if len(snap.Unstaged) != 0 {
		t.Fatalf("left %+v", snap.Unstaged)
	}
}

func TestDiscardBeforeFirstCommit(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "staged\n")
	e.git(dir, "add", "a.txt")
	write(t, filepath.Join(dir, "a.txt"), "staged\nmore\n")
	repo, snap := e.open(dir)
	p, err := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Discard(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "a.txt")) != "staged\n" || e.git(dir, "diff", "--cached", "--name-only") != "a.txt\n" {
		t.Fatal("unborn discard")
	}
}

func TestDiscardRefusesChangesSinceConfirmed(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), "one\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), "two\n")
	repo, snap := e.open(dir)
	p, _ := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "f.txt"))
	// Edited again after the confirmation showed.
	write(t, filepath.Join(dir, "f.txt"), "three\n")
	_, err := repo.Discard(context.Background(), p)
	wantCode(t, err, CodeDiscardStale)
	if readFile(t, filepath.Join(dir, "f.txt")) != "three\n" {
		t.Fatal("discarded a newer edit")
	}
	// Staged meanwhile: the staged version moved.
	p, _ = repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "f.txt"))
	write(t, filepath.Join(dir, "f.txt"), "four\n")
	e.git(dir, "add", "f.txt")
	write(t, filepath.Join(dir, "f.txt"), "three\n")
	_, err = repo.Discard(context.Background(), p)
	wantCode(t, err, CodeDiscardStale)
	// A commit moved HEAD.
	e.git(dir, "reset", "-q")
	p, _ = repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "f.txt"))
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Meanwhile")
	_, err = repo.Discard(context.Background(), p)
	wantCode(t, err, CodeDiscardStale)
	if readFile(t, filepath.Join(dir, "f.txt")) != "three\n" {
		t.Fatal("stale discard changed the file")
	}
}

func TestDiscardUnsupported(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "old.txt"), "o\n")
	write(t, filepath.Join(dir, "file"), "f\n")
	if err := os.Symlink("old.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "untracked.txt"), "u\n")
	e.git(dir, "mv", "old.txt", "new.txt")
	write(t, filepath.Join(dir, "new.txt"), "o\nmore\n")
	os.Remove(filepath.Join(dir, "link"))
	os.Symlink("file", filepath.Join(dir, "link"))
	os.Remove(filepath.Join(dir, "file"))
	os.Symlink("old.txt", filepath.Join(dir, "file"))
	repo, snap := e.open(dir)
	for path, code := range map[string]Code{"untracked.txt": CodeDiscardUntracked, "new.txt": CodeDiscardRenamed, "link": CodeDiscardSymlink, "file": CodeDiscardSymlink} {
		_, err := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, path))
		if CodeOf(err) != code {
			t.Errorf("%s: %v, want %s", path, err, code)
		}
	}
	if readFile(t, filepath.Join(dir, "untracked.txt")) != "u\n" {
		t.Fatal("untracked file touched")
	}
}

func TestDiscardRefusesFolderReplacedByLink(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "sub", "f.txt"), "one\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "sub", "f.txt"), "two\n")
	repo, snap := e.open(dir)
	p, err := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "sub/f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// The folder becomes a link to a place outside the worktree.
	outside := t.TempDir()
	write(t, filepath.Join(outside, "f.txt"), "outside\n")
	os.RemoveAll(filepath.Join(dir, "sub"))
	os.Symlink(outside, filepath.Join(dir, "sub"))
	_, err = repo.Discard(context.Background(), p)
	if err == nil {
		t.Fatal("discarded through a link")
	}
	if readFile(t, filepath.Join(outside, "f.txt")) != "outside\n" {
		t.Fatal("wrote outside the worktree")
	}
}

func TestDiscardKeepsOtherWorktree(t *testing.T) {
	e := newEnv(t)
	main, linked := linkedRepo(t, e)
	write(t, filepath.Join(main, "a.txt"), "main edit\n")
	write(t, filepath.Join(linked, "a.txt"), "linked edit\n")
	repo, snap := e.open(main)
	p, _ := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "a.txt"))
	if _, err := repo.Discard(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(main, "a.txt")) != "a\n" || readFile(t, filepath.Join(linked, "a.txt")) != "linked edit\n" {
		t.Fatal("wrong worktree")
	}
}

func TestDiscardPermissionFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores permissions")
	}
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "sub", "f.txt"), "one\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "sub", "f.txt"), "two\n")
	repo, snap := e.open(dir)
	p, _ := repo.PlanDiscard(context.Background(), unstagedOf(t, snap, "sub/f.txt"))
	os.Chmod(filepath.Join(dir, "sub"), 0o555)
	defer os.Chmod(filepath.Join(dir, "sub"), 0o755)
	snap, err := repo.Discard(context.Background(), p)
	if err == nil || snap.Root == "" {
		t.Fatalf("permission: %v %+v", err, snap)
	}
	os.Chmod(filepath.Join(dir, "sub"), 0o755)
	if readFile(t, filepath.Join(dir, "sub", "f.txt")) != "two\n" {
		t.Fatal("file changed")
	}
}
