package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// twoCommits builds a repository with two commits and returns the handle, the
// first commit's full hash and the second (HEAD) full hash.
func twoCommits(t *testing.T, e *env) (r *Repository, first, head string) {
	t.Helper()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	e.commit(dir, "First")
	first = strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	write(t, filepath.Join(dir, "a.txt"), "two\n")
	e.commit(dir, "Second")
	head = strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	r, _ = e.open(dir)
	return r, first, head
}

func TestCheckoutCommitDetaches(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	snap, _, err := r.CheckoutCommit(ctx, CheckoutRequest{Hash: first, Head: head})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if !snap.Detached || snap.Head != first || snap.Branch != "" {
		t.Fatalf("not detached at first: %+v", snap)
	}
}

func TestCheckoutCommitRefusesWhenHeadMoved(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	// HEAD moved outside GitOwl since the user reviewed it.
	e.git(r.Root(), "switch", "-q", "-c", "elsewhere")
	write(t, filepath.Join(r.Root(), "a.txt"), "three\n")
	e.commit(r.Root(), "Third")
	e.git(r.Root(), "switch", "-q", "main")
	_ = head
	moved := strings.TrimSpace(e.git(r.Root(), "rev-parse", "HEAD"))
	snap, _, err := r.CheckoutCommit(ctx, CheckoutRequest{Hash: first, Head: "stale"})
	wantCode(t, err, CodeStateChanged)
	if snap.Detached || snap.Head != moved {
		t.Fatalf("state changed by refused checkout: %+v", snap)
	}
}

func TestCheckoutCommitKeepsUncommittedWork(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	// A local edit to a.txt, which first and head differ on, would be
	// overwritten by checking out first.
	write(t, filepath.Join(r.Root(), "a.txt"), "my work\n")
	snap, _, err := r.CheckoutCommit(ctx, CheckoutRequest{Hash: first, Head: head})
	wantCode(t, err, CodeSwitchBlocked)
	if snap.Detached {
		t.Fatalf("detached despite refusal: %+v", snap)
	}
	if data, _ := os.ReadFile(filepath.Join(r.Root(), "a.txt")); string(data) != "my work\n" {
		t.Fatalf("local work lost: %q", data)
	}
}

func TestCheckoutCommitSwitchesBackToBranch(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	if _, _, err := r.CheckoutCommit(ctx, CheckoutRequest{Hash: first, Head: head}); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	snap, _, err := r.SwitchBranch(ctx, "main")
	if err != nil || snap.Detached || snap.Branch != "main" || snap.Head != head {
		t.Fatalf("switch back: %v %+v", err, snap)
	}
}

func TestCreateBranchAtWithoutSwitch(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	snap, b, err := r.CreateBranchAt(ctx, BranchAtRequest{Name: "from-first", Start: first, Switch: false, Head: head})
	if err != nil {
		t.Fatalf("branch at: %v", err)
	}
	if snap.Branch != "main" || snap.Head != head {
		t.Fatalf("HEAD moved by a no-switch create: %+v", snap)
	}
	if _, ok := b.local("from-first"); !ok {
		t.Fatalf("branch not listed: %s", names(b))
	}
	if got := strings.TrimSpace(e.git(r.Root(), "rev-parse", "from-first")); got != first {
		t.Fatalf("branch at %s, want %s", got, first)
	}
}

func TestCreateBranchAtWithSwitch(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	snap, b, err := r.CreateBranchAt(ctx, BranchAtRequest{Name: "work", Start: first, Switch: true, Head: head})
	if err != nil {
		t.Fatalf("branch at and switch: %v", err)
	}
	if snap.Branch != "work" || snap.Head != first || snap.Detached {
		t.Fatalf("not switched to new branch: %+v", snap)
	}
	if _, ok := b.local("work"); !ok {
		t.Fatalf("branch not listed: %s", names(b))
	}
}

func TestCreateBranchAtRefusesDuplicateName(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	_, b, err := r.CreateBranchAt(ctx, BranchAtRequest{Name: "main", Start: first, Head: head})
	wantCode(t, err, CodeBranchExists)
	if names(b) != "*main" {
		t.Fatalf("branches changed: %s", names(b))
	}
}

func TestCreateBranchAtRefusesMissingStart(t *testing.T) {
	e := newEnv(t)
	r, _, head := twoCommits(t, e)
	ctx := context.Background()
	absent := strings.Repeat("0", 40)
	_, b, err := r.CreateBranchAt(ctx, BranchAtRequest{Name: "x", Start: absent, Head: head})
	wantCode(t, err, CodeCheckoutStartNotFound)
	if _, ok := b.local("x"); ok {
		t.Fatalf("branch created from a missing start: %s", names(b))
	}
}

func TestCreateBranchAtRefusesWhenHeadMoved(t *testing.T) {
	e := newEnv(t)
	r, first, _ := twoCommits(t, e)
	ctx := context.Background()
	_, b, err := r.CreateBranchAt(ctx, BranchAtRequest{Name: "x", Start: first, Head: "stale"})
	wantCode(t, err, CodeStateChanged)
	if _, ok := b.local("x"); ok {
		t.Fatalf("branch created against a stale HEAD: %s", names(b))
	}
}

func TestCreateBranchAtReportsCreatedButNotSwitched(t *testing.T) {
	e := newEnv(t)
	r, first, head := twoCommits(t, e)
	ctx := context.Background()
	// A local edit that switching to the new branch (at first) would
	// overwrite, since first and head differ on a.txt.
	write(t, filepath.Join(r.Root(), "a.txt"), "my work\n")
	snap, b, err := r.CreateBranchAt(ctx, BranchAtRequest{Name: "stay", Start: first, Switch: true, Head: head})
	wantCode(t, err, CodeCheckoutNotSwitched)
	if snap.Branch != "main" || snap.Detached {
		t.Fatalf("switched despite refusal: %+v", snap)
	}
	if _, ok := b.local("stay"); !ok {
		t.Fatalf("branch not created: %s", names(b))
	}
	if data, _ := os.ReadFile(filepath.Join(r.Root(), "a.txt")); string(data) != "my work\n" {
		t.Fatalf("local work lost: %q", data)
	}
}

func TestCheckoutThenReturnRefusesBranchInOtherWorktree(t *testing.T) {
	e := newEnv(t)
	main, linked := linkedRepo(t, e)
	r, _ := e.open(main)
	ctx := context.Background()
	head := strings.TrimSpace(e.git(main, "rev-parse", "HEAD"))
	if _, _, err := r.CheckoutCommit(ctx, CheckoutRequest{Hash: head, Head: head}); err != nil {
		t.Fatalf("checkout: %v", err)
	}
	// side is held by the linked worktree; returning to it is refused with
	// the same protection SwitchBranch already applies.
	snap, _, err := r.SwitchBranch(ctx, "side")
	wantCode(t, err, CodeBranchInWorktree)
	if !snap.Detached {
		t.Fatalf("left detached despite refusal: %+v", snap)
	}
	_ = linked
}
