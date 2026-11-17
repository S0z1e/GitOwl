package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// divergedRepo is main and feature that share a base and then each change
// a.txt differently, plus feature adding extra.txt. The handle is on main.
func divergedRepo(t *testing.T, e *env, mainText, featureText string) (*Repository, string, string) {
	t.Helper()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "base\n")
	e.commit(dir, "Base")
	e.git(dir, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(dir, "a.txt"), featureText)
	write(t, filepath.Join(dir, "extra.txt"), "extra\n")
	e.commit(dir, "On feature")
	feature := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	e.git(dir, "switch", "-q", "main")
	write(t, filepath.Join(dir, "a.txt"), mainText)
	e.commit(dir, "On main")
	main := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	r, _ := e.open(dir)
	return r, main, feature
}

func TestMergeJoinsTheNamedSides(t *testing.T) {
	e := newEnv(t)
	r, _, _ := divergedRepo(t, e, "base\nmain\n", "base\n")
	ctx := context.Background()

	snap, err := r.Merge(ctx, "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "main" || !snap.Clean() {
		t.Fatalf("after merge: %+v", snap)
	}
	if _, err := os.Stat(filepath.Join(r.Root(), "extra.txt")); err != nil {
		t.Fatalf("feature file missing: %v", err)
	}
	parents := strings.Fields(strings.TrimSpace(e.git(r.Root(), "rev-list", "--parents", "-n", "1", "HEAD")))
	if len(parents) != 3 {
		t.Fatalf("want a merge commit, parents %v", parents)
	}
}

func TestMergeSwitchesToOurs(t *testing.T) {
	e := newEnv(t)
	r, main, _ := divergedRepo(t, e, "base\nmain\n", "base\n")
	ctx := context.Background()

	snap, err := r.Merge(ctx, "feature", "main")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "feature" || !snap.Clean() {
		t.Fatalf("after merge into feature: %+v", snap)
	}
	parent := strings.TrimSpace(e.git(r.Root(), "rev-parse", "HEAD^2"))
	if parent != main {
		t.Fatalf("merged main as second parent, got %s", parent)
	}
}

func TestMergeConflictMarksCurrentChanges(t *testing.T) {
	e := newEnv(t)
	r, head, _ := divergedRepo(t, e, "main\n", "feature\n")
	ctx := context.Background()

	snap, err := r.Merge(ctx, "main", "feature")
	if err != nil {
		t.Fatalf("conflict should return the snapshot, not an error: %v", err)
	}
	if snap.Head != head {
		t.Fatalf("HEAD moved during a conflicted merge: %s", snap.Head)
	}
	if n := conflictedPaths(snap); join(n) != "a.txt" {
		t.Fatalf("conflicted %v", n)
	}
}

func TestRebaseReplaysBranchOntoNamedBase(t *testing.T) {
	e := newEnv(t)
	r, main, _ := divergedRepo(t, e, "base\nmain\n", "base\n")
	ctx := context.Background()

	snap, err := r.Rebase(ctx, "feature", "main")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "feature" || !snap.Clean() {
		t.Fatalf("after rebase: %+v", snap)
	}
	parent := strings.TrimSpace(e.git(r.Root(), "rev-parse", "HEAD^"))
	if parent != main {
		t.Fatalf("rebased parent %s, want %s", parent, main)
	}
	if _, err := os.Stat(filepath.Join(r.Root(), "extra.txt")); err != nil {
		t.Fatalf("replayed file missing: %v", err)
	}
}

func TestRebaseConflictMarksCurrentChanges(t *testing.T) {
	e := newEnv(t)
	r, _, _ := divergedRepo(t, e, "main\n", "feature\n")
	ctx := context.Background()

	snap, err := r.Rebase(ctx, "feature", "main")
	if err != nil {
		t.Fatalf("conflict should return the snapshot, not an error: %v", err)
	}
	if n := conflictedPaths(snap); join(n) != "a.txt" {
		t.Fatalf("conflicted %v", n)
	}
}

func conflictedPaths(snap Snapshot) []string {
	var out []string
	for _, ch := range snap.Unstaged {
		if ch.Conflicted {
			out = append(out, ch.Path)
		}
	}
	return out
}
