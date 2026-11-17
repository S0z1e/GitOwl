package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// linkedRepo returns a repository with one commit and a linked worktree
// on branch side, made with the Git command line.
func linkedRepo(t *testing.T, e *env) (main, linked string) {
	t.Helper()
	main = e.repo()
	write(t, filepath.Join(main, "a.txt"), "a\n")
	e.commit(main, "Initial")
	linked = filepath.Join(canonical(t.TempDir()), "linked")
	e.git(main, "worktree", "add", "-q", "-b", "side", linked)
	return main, linked
}

func worktreeAt(t *testing.T, list []Worktree, path string) Worktree {
	t.Helper()
	for _, w := range list {
		if w.Path == path {
			return w
		}
	}
	t.Fatalf("%s not in %+v", path, list)
	return Worktree{}
}

func TestWorktreesFromAnyWorktree(t *testing.T) {
	e := newEnv(t)
	main, linked := linkedRepo(t, e)
	detached := filepath.Join(canonical(t.TempDir()), "detached")
	e.git(main, "worktree", "add", "-q", "--detach", detached)
	e.git(main, "worktree", "lock", "--reason", "external disk", detached)
	write(t, filepath.Join(linked, "sub", "x.txt"), "x\n")

	// Opened from a subdirectory of the linked worktree.
	r, snap := e.open(filepath.Join(linked, "sub"))
	if r.Root() != linked || snap.Branch != "side" {
		t.Fatalf("root %s branch %s", r.Root(), snap.Branch)
	}
	rm, _ := e.open(main)
	if r.Common() == "" || r.Common() != rm.Common() {
		t.Fatalf("common %q vs %q", r.Common(), rm.Common())
	}
	r, _ = e.open(filepath.Join(linked, "sub"))
	list, err := r.Worktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Path != main || !list[0].Main || list[0].Current || list[0].Branch != "main" {
		t.Fatalf("list %+v", list)
	}
	if w := worktreeAt(t, list, linked); !w.Current || w.Main || w.Branch != "side" {
		t.Fatalf("linked %+v", w)
	}
	if w := worktreeAt(t, list, detached); !w.Detached || w.Branch != "" || !w.Locked || w.LockReason != "external disk" || w.Head == "" {
		t.Fatalf("detached %+v", w)
	}

	// Changes outside GitOwl show on the next read.
	e.git(main, "worktree", "unlock", detached)
	os.RemoveAll(linked)
	rm, _ = e.open(main)
	list, err = rm.Worktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if w := worktreeAt(t, list, detached); w.Locked {
		t.Fatalf("unlock not seen %+v", w)
	}
	if w := worktreeAt(t, list, linked); !w.Prunable || w.PrunableReason == "" {
		t.Fatalf("removed directory not prunable %+v", w)
	}
}

func TestDetachedWorktreeOpensAndKeepsItsOwnIndex(t *testing.T) {
	e := newEnv(t)
	main, _ := linkedRepo(t, e)
	detached := filepath.Join(canonical(t.TempDir()), "detached")
	e.git(main, "worktree", "add", "-q", "--detach", detached)
	write(t, filepath.Join(detached, "d.txt"), "d\n")
	e.git(detached, "add", "d.txt")
	r, snap := e.open(detached)
	if !snap.Detached || snap.Head == "" || len(snap.Staged) != 1 {
		t.Fatalf("detached %+v", snap)
	}
	if _, snap := e.open(main); len(snap.Staged)+len(snap.Untracked) != 0 {
		t.Fatalf("main sees the detached worktree's changes: %+v", snap)
	}
	_ = r
}

func TestBareRepositoryStaysRefused(t *testing.T) {
	e := newEnv(t)
	bare := filepath.Join(t.TempDir(), "bare.git")
	e.git(t.TempDir(), "init", "-q", "--bare", bare)
	_, _, err := e.app.Open(context.Background(), bare)
	wantCode(t, err, CodeBareRepository)
}
