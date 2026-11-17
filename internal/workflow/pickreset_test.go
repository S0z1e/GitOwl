package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCherryPickAppliesCommitToCurrentBranch(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Base")
	e.git(dir, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(dir, "picked.txt"), "picked\n")
	e.commit(dir, "Add picked")
	hash := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	e.git(dir, "switch", "-q", "main")
	r, snap := e.open(dir)
	ctx := context.Background()

	after, err := r.CherryPick(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	if after.Branch != "main" || after.Head == snap.Head || !after.Clean() {
		t.Fatalf("after cherry-pick: %+v", after)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "picked.txt")); err != nil || string(data) != "picked\n" {
		t.Fatalf("picked file %q %v", data, err)
	}
	subject := strings.TrimSpace(e.git(dir, "log", "-1", "--format=%s"))
	if subject != "Add picked" {
		t.Fatalf("subject %q", subject)
	}
}

func TestCherryPickConflictMarksCurrentChanges(t *testing.T) {
	e := newEnv(t)
	r, head, feature := divergedRepo(t, e, "main\n", "feature\n")
	ctx := context.Background()

	snap, err := r.CherryPick(ctx, feature)
	if err != nil {
		t.Fatalf("conflict should return the snapshot, not an error: %v", err)
	}
	if snap.Head != head {
		t.Fatalf("HEAD moved during a conflicted cherry-pick: %s", snap.Head)
	}
	if n := conflictedPaths(snap); join(n) != "a.txt" {
		t.Fatalf("conflicted %v", n)
	}
}

func TestMixedResetKeepsWorktreeFiles(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	e.commit(dir, "First")
	first := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	write(t, filepath.Join(dir, "a.txt"), "two\n")
	e.commit(dir, "Second")
	write(t, filepath.Join(dir, "a.txt"), "kept\n")
	write(t, filepath.Join(dir, "local.txt"), "local\n")
	r, _ := e.open(dir)

	snap, err := r.MixedReset(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "main" || snap.Head != first {
		t.Fatalf("branch did not move: %+v", snap)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "kept\n" {
		t.Fatalf("worktree overwritten: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "local.txt")); string(data) != "local\n" {
		t.Fatalf("untracked file lost: %q", data)
	}
	if staged := e.git(dir, "show", ":a.txt"); staged != "one\n" {
		t.Fatalf("index not at reset commit: %q", staged)
	}
}
