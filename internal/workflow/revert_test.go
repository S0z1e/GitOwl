package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// revertRepo builds a repository with two commits: "Base" writes a.txt="a\n",
// then "Add line" appends to it. It returns the env, the repo, the latest
// snapshot and the hash of the second commit (the one to revert).
func revertRepo(t *testing.T, e *env) (*Repository, Snapshot, string) {
	t.Helper()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Base")
	write(t, filepath.Join(dir, "a.txt"), "a\nb\n")
	e.commit(dir, "Add line")
	target := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	r, snap := e.open(dir)
	return r, snap, target
}

func req(snap Snapshot, target string) RevertRequest {
	return RevertRequest{Target: target, Head: snap.Head, Branch: snap.Branch}
}

func TestRevertUndoesContent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, snap, target := revertRepo(t, e)
	after, status, err := r.Revert(ctx, req(snap, target))
	if err != nil {
		t.Fatal(err)
	}
	if status.Active {
		t.Fatalf("unexpected in-progress: %+v", status)
	}
	if !after.Clean() {
		t.Fatalf("worktree not clean: %+v", after)
	}
	got, err := os.ReadFile(filepath.Join(r.Root(), "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\n" {
		t.Fatalf("content not reverted: %q", got)
	}
	// A new commit was made whose parent is the former HEAD.
	if after.Head == target {
		t.Fatalf("HEAD did not advance")
	}
	parent := strings.TrimSpace(e.git(r.Root(), "rev-parse", "HEAD^"))
	if parent != target {
		t.Fatalf("revert parent %s, want %s", parent, target)
	}
}

func TestRevertRejectsMergeCommit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Base")
	e.git(dir, "checkout", "-q", "-b", "side")
	write(t, filepath.Join(dir, "b.txt"), "b\n")
	e.commit(dir, "Side")
	e.git(dir, "checkout", "-q", "main")
	write(t, filepath.Join(dir, "c.txt"), "c\n")
	e.commit(dir, "Main")
	e.git(dir, "merge", "--no-ff", "-m", "Merge side", "side")
	merge := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	r, snap := e.open(dir)
	_, _, err := r.Revert(ctx, req(snap, merge))
	wantCode(t, err, CodeRevertMerge)
}

func TestRevertRejectsDirtyAndDetached(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, _, target := revertRepo(t, e)
	write(t, filepath.Join(r.Root(), "a.txt"), "dirty\n")
	dirty, _ := r.Status(ctx)
	_, _, err := r.Revert(ctx, req(dirty, target))
	wantCode(t, err, CodeRevertDirty)

	// Restore and detach HEAD.
	e.git(r.Root(), "checkout", "-q", "--", "a.txt")
	e.git(r.Root(), "checkout", "-q", "--detach")
	det, _ := r.Status(ctx)
	_, _, err = r.Revert(ctx, RevertRequest{Target: target, Head: det.Head, Branch: det.Branch})
	wantCode(t, err, CodeRevertDetached)
}

func TestRevertRejectsUnknownTarget(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, snap, _ := revertRepo(t, e)
	_, _, err := r.Revert(ctx, req(snap, strings.Repeat("0", 40)))
	wantCode(t, err, CodeRevertNotFound)
}

// conflictRepo builds a repo where reverting the middle commit conflicts:
// a.txt goes "a" -> "one" (target) -> "two" (HEAD). It returns the repo,
// its snapshot and the target hash.
func conflictRepo(t *testing.T, e *env) (*Repository, Snapshot, string) {
	t.Helper()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Base")
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	e.commit(dir, "One")
	target := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	write(t, filepath.Join(dir, "a.txt"), "two\n")
	e.commit(dir, "Two")
	r, snap := e.open(dir)
	return r, snap, target
}

func TestRevertConflictLeavesReadableState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, snap, target := conflictRepo(t, e)
	head := snap.Head
	after, status, err := r.Revert(ctx, req(snap, target))
	if err != nil {
		t.Fatalf("revert errored instead of reporting conflict: %v", err)
	}
	if !status.Active || status.Target != target {
		t.Fatalf("not in progress: %+v", status)
	}
	if len(status.Conflicted) != 1 || status.Conflicted[0] != "a.txt" {
		t.Fatalf("conflicted paths %v", status.Conflicted)
	}
	if after.Head != head {
		t.Fatalf("HEAD moved during conflicted revert: %s", after.Head)
	}
	// A freshly opened handle recognises the revert in progress.
	r2, _ := e.open(r.Root())
	s2, err := r2.RevertState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Active || s2.Target != target || len(s2.Conflicted) != 1 {
		t.Fatalf("reopened state %+v", s2)
	}
}

func TestAbortRevertRestoresState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, snap, target := conflictRepo(t, e)
	head := snap.Head
	before, err := os.ReadFile(filepath.Join(r.Root(), "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Revert(ctx, req(snap, target)); err != nil {
		t.Fatal(err)
	}
	after, err := r.AbortRevert(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Head != head || !after.Clean() {
		t.Fatalf("abort did not restore: head=%s clean=%v", after.Head, after.Clean())
	}
	got, err := os.ReadFile(filepath.Join(r.Root(), "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(before) {
		t.Fatalf("file bytes not restored: %q want %q", got, before)
	}
	// No revert is in progress now.
	s, _ := r.RevertState(ctx)
	if s.Active {
		t.Fatalf("still in progress after abort")
	}
	if _, err := r.AbortRevert(ctx); CodeOf(err) != CodeRevertNoProgress {
		t.Fatalf("abort without progress: %v", err)
	}
}

func TestContinueRevertNeedsAllResolved(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	r, snap, target := conflictRepo(t, e)
	if _, status, err := r.Revert(ctx, req(snap, target)); err != nil || !status.Active {
		t.Fatalf("setup revert: %v %+v", err, status)
	}
	// Continue refused while the conflict is unresolved.
	rr := RevertRequest{Target: target, Head: snap.Head, Branch: snap.Branch}
	if _, _, err := r.ContinueRevert(ctx, rr); CodeOf(err) != CodeRevertConflicts {
		t.Fatalf("continue with conflict: %v", err)
	}
	// A file that still holds conflict markers cannot be marked resolved.
	if _, err := r.MarkResolved(ctx, "a.txt"); CodeOf(err) != CodeRevertConflicts {
		t.Fatalf("mark resolved with markers: %v", err)
	}
	// Resolve the file, mark it, then continue to a commit.
	write(t, filepath.Join(r.Root(), "a.txt"), "a\n")
	if _, err := r.MarkResolved(ctx, "a.txt"); err != nil {
		t.Fatal(err)
	}
	state, _ := r.RevertState(ctx)
	if !state.AllResolved() {
		t.Fatalf("not all resolved: %+v", state)
	}
	after, status, err := r.ContinueRevert(ctx, rr)
	if err != nil {
		t.Fatal(err)
	}
	if status.Active {
		t.Fatalf("still in progress after continue: %+v", status)
	}
	if after.Head == snap.Head {
		t.Fatalf("no commit produced")
	}
	got, err := os.ReadFile(filepath.Join(r.Root(), "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a\n" {
		t.Fatalf("resolved content %q", got)
	}
}

func TestMarkResolvedHandlesDeleteConflict(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Base")
	write(t, filepath.Join(dir, "a.txt"), "a\nmid\n")
	e.commit(dir, "Middle")
	target := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	// Delete the file in the newest commit, so reverting Middle conflicts
	// with the deletion.
	e.git(dir, "rm", "-q", "a.txt")
	e.commit(dir, "Remove")
	r, snap := e.open(dir)
	_, status, err := r.Revert(ctx, req(snap, target))
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || len(status.Conflicted) == 0 {
		t.Fatalf("expected delete conflict: %+v", status)
	}
	// Resolve by keeping it deleted.
	os.Remove(filepath.Join(r.Root(), "a.txt"))
	if _, err := r.MarkResolved(ctx, status.Conflicted[0]); err != nil {
		t.Fatalf("mark delete resolved: %v", err)
	}
	s, _ := r.RevertState(ctx)
	if !s.AllResolved() {
		t.Fatalf("not resolved after rm: %+v", s)
	}
}

func TestRevertInOneWorktreeLeavesAnotherAlone(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	// Build a main worktree whose middle commit conflicts on revert.
	main := e.repo()
	write(t, filepath.Join(main, "a.txt"), "a\n")
	e.commit(main, "Base")
	write(t, filepath.Join(main, "a.txt"), "one\n")
	e.commit(main, "One")
	target := strings.TrimSpace(e.git(main, "rev-parse", "HEAD"))
	write(t, filepath.Join(main, "a.txt"), "two\n")
	e.commit(main, "Two")
	linked := filepath.Join(canonical(t.TempDir()), "linked")
	e.git(main, "worktree", "add", "-q", "-b", "side", linked)

	rm, snap := e.open(main)
	if _, status, err := rm.Revert(ctx, req(snap, target)); err != nil || !status.Active {
		t.Fatalf("revert in main: %v %+v", err, status)
	}
	// The linked worktree has no revert in progress of its own.
	rl, _ := e.open(linked)
	s, err := rl.RevertState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Active {
		t.Fatalf("linked worktree sees main's revert: %+v", s)
	}
	// The main worktree's revert is still there, found through --git-path.
	rm2, _ := e.open(main)
	sm, err := rm2.RevertState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !sm.Active || sm.Target != target {
		t.Fatalf("main revert lost: %+v", sm)
	}
}
