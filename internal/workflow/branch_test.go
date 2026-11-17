package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func names(b Branches) string {
	var out []string
	for _, br := range b.Local {
		n := br.Name
		if br.Current {
			n = "*" + n
		}
		out = append(out, n)
	}
	return join(out)
}

func TestBranchesCreateSwitchDelete(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	r, _ := e.open(dir)
	ctx := context.Background()

	b, err := r.Branches(ctx)
	if err != nil || names(b) != "*main" {
		t.Fatalf("branches %s %v", names(b), err)
	}
	snap, b, err := r.CreateBranch(ctx, "feature/x", false)
	if err != nil || names(b) != "feature/x,*main" || snap.Branch != "main" {
		t.Fatalf("create %s %v", names(b), err)
	}
	snap, b, err = r.CreateBranch(ctx, "work", true)
	if err != nil || snap.Branch != "work" || names(b) != "feature/x,main,*work" {
		t.Fatalf("create and switch %s %s %v", snap.Branch, names(b), err)
	}
	snap, _, err = r.SwitchBranch(ctx, "main")
	if err != nil || snap.Branch != "main" {
		t.Fatalf("switch %s %v", snap.Branch, err)
	}
	_, b, err = r.DeleteBranch(ctx, "feature/x")
	if err != nil || names(b) != "*main,work" {
		t.Fatalf("delete %s %v", names(b), err)
	}
}

func TestBranchNameErrors(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	r, _ := e.open(dir)
	ctx := context.Background()
	for _, name := range []string{"", "bad..name", "-x", "has space", "end.lock", " lead"} {
		_, _, err := r.CreateBranch(ctx, name, false)
		wantCode(t, err, CodeInvalidBranchName)
	}
	_, _, err := r.CreateBranch(ctx, "main", false)
	wantCode(t, err, CodeBranchExists)
	b, _ := r.Branches(ctx)
	if names(b) != "*main" {
		t.Fatalf("branches changed: %s", names(b))
	}
}

func TestDeleteRefusesCurrentAndUnmerged(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	e.git(dir, "switch", "-q", "-c", "unmerged")
	write(t, filepath.Join(dir, "u.txt"), "u\n")
	e.commit(dir, "Only here")
	e.git(dir, "switch", "-q", "main")
	r, _ := e.open(dir)
	ctx := context.Background()
	_, _, err := r.DeleteBranch(ctx, "main")
	wantCode(t, err, CodeDeleteCurrent)
	_, b, err := r.DeleteBranch(ctx, "unmerged")
	wantCode(t, err, CodeBranchNotMerged)
	if names(b) != "*main,unmerged" {
		t.Fatalf("branches %s", names(b))
	}
	_, _, err = r.DeleteBranch(ctx, "missing")
	wantCode(t, err, CodeBranchNotFound)
}

func TestSwitchThatWouldOverwriteChangesFails(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "main\n")
	e.commit(dir, "Initial")
	e.git(dir, "switch", "-q", "-c", "other")
	write(t, filepath.Join(dir, "a.txt"), "other\n")
	e.commit(dir, "Change on other")
	e.git(dir, "switch", "-q", "main")
	write(t, filepath.Join(dir, "a.txt"), "my local work\n")
	write(t, filepath.Join(dir, "staged.txt"), "staged\n")
	e.git(dir, "add", "staged.txt")
	r, _ := e.open(dir)
	snap, _, err := r.SwitchBranch(context.Background(), "other")
	wantCode(t, err, CodeSwitchBlocked)
	if snap.Branch != "main" {
		t.Fatalf("branch %s", snap.Branch)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "my local work\n" {
		t.Fatalf("local change lost: %q", data)
	}
	if join(paths(snap.Staged)) != "staged.txt" || join(paths(snap.Unstaged)) != "a.txt" {
		t.Fatalf("state after failed switch %+v", snap)
	}
	if out := e.git(dir, "stash", "list"); out != "" {
		t.Fatalf("stashed: %q", out)
	}
}

func TestSwitchCarriesNonConflictingChanges(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	e.git(dir, "branch", "other")
	write(t, filepath.Join(dir, "a.txt"), "edited\n")
	r, _ := e.open(dir)
	snap, _, err := r.SwitchBranch(context.Background(), "other")
	if err != nil || snap.Branch != "other" || join(paths(snap.Unstaged)) != "a.txt" {
		t.Fatalf("%v %+v", err, snap)
	}
}

func TestBranchesInUnbornRepositoryAndRemotes(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	r, _ := e.open(dir)
	ctx := context.Background()
	b, err := r.Branches(ctx)
	if err != nil || len(b.Local) != 0 {
		t.Fatalf("unborn branches %+v %v", b, err)
	}
	_, _, err = r.CreateBranch(ctx, "x", false)
	wantCode(t, err, CodeNoCommitsForBranch)

	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	e.git(dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	e.git(dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	e.git(dir, "config", "remote.origin.url", "https://example.invalid/repo.git")
	e.git(dir, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	e.git(dir, "config", "branch.main.remote", "origin")
	e.git(dir, "config", "branch.main.merge", "refs/heads/main")
	write(t, filepath.Join(dir, "a.txt"), "b\n")
	e.commit(dir, "Ahead")
	b, err = r.Branches(ctx)
	if err != nil || len(b.Remote) != 1 || b.Remote[0].Name != "origin/main" {
		t.Fatalf("remotes %+v %v", b.Remote, err)
	}
	if m := b.Local[0]; m.Upstream != "origin/main" || m.Ahead != 1 || m.Behind != 0 {
		t.Fatalf("tracking %+v", m)
	}
	e.git(dir, "update-ref", "-d", "refs/remotes/origin/main")
	b, _ = r.Branches(ctx)
	if !b.Local[0].Gone {
		t.Fatalf("gone upstream %+v", b.Local[0])
	}
}

func TestBranchOpOnClosedRepository(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	r, _ := e.open(dir)
	e.open(e.repo())
	_, _, err := r.CreateBranch(context.Background(), "x", false)
	wantCode(t, err, CodeRepositoryClosed)
}

func TestBranchCheckedOutInAnotherWorktree(t *testing.T) {
	e := newEnv(t)
	main, linked := linkedRepo(t, e)
	r, _ := e.open(main)
	ctx := context.Background()
	_, b, err := r.DeleteBranch(ctx, "side")
	wantCode(t, err, CodeBranchInWorktree)
	var we *Error
	if errors.As(err, &we); we.Detail != linked {
		t.Fatalf("detail %q, want %q", we.Detail, linked)
	}
	if names(b) != "*main,side" {
		t.Fatalf("branches %s", names(b))
	}
	_, _, err = r.SwitchBranch(ctx, "side")
	wantCode(t, err, CodeBranchInWorktree)
	if got := strings.TrimSpace(e.git(main, "branch", "--show-current")); got != "main" {
		t.Fatalf("main worktree on %q", got)
	}
}
