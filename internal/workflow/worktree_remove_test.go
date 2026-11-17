package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addWorktree adds a linked worktree on a new branch with the Git command
// line, as another tool would.
func addWorktree(t *testing.T, e *env, main, name string) string {
	t.Helper()
	p := filepath.Join(filepath.Dir(main), name)
	e.git(main, "worktree", "add", "-q", "-b", name, p)
	return canonical(p)
}

func TestRemoveWorktreeRules(t *testing.T) {
	e := newEnv(t)
	main := e.repo()
	write(t, filepath.Join(main, "a.txt"), "a\n")
	e.commit(main, "Initial")
	clean := addWorktree(t, e, main, "clean")
	staged := addWorktree(t, e, main, "staged")
	unstaged := addWorktree(t, e, main, "unstaged")
	untracked := addWorktree(t, e, main, "untracked")
	locked := addWorktree(t, e, main, "locked")
	current := addWorktree(t, e, main, "current")
	write(t, filepath.Join(staged, "s.txt"), "s\n")
	e.git(staged, "add", "s.txt")
	write(t, filepath.Join(unstaged, "a.txt"), "changed\n")
	write(t, filepath.Join(untracked, "new.txt"), "new\n")
	e.git(main, "worktree", "lock", "--reason", "on a disk", locked)
	write(t, filepath.Join(main, "main-work.txt"), "keep\n")
	e.git(main, "add", "main-work.txt")
	r, _ := e.open(current)
	ctx := context.Background()

	for path, code := range map[string]Code{
		main:      CodeRemoveMain,
		current:   CodeRemoveCurrent,
		locked:    CodeWorktreeLocked,
		staged:    CodeWorktreeDirty,
		unstaged:  CodeWorktreeDirty,
		untracked: CodeWorktreeDirty,
		filepath.Join(filepath.Dir(main), "nowhere"): CodeWorktreeNotFound,
	} {
		list, err := r.RemoveWorktree(ctx, path)
		wantCode(t, err, code)
		if len(list) != 7 {
			t.Fatalf("%s: list %d", code, len(list))
		}
		if _, err := os.Stat(path); code != CodeWorktreeNotFound && err != nil {
			t.Fatalf("%s: %s gone", code, path)
		}
	}
	if readFile(t, filepath.Join(untracked, "new.txt")) != "new\n" || readFile(t, filepath.Join(unstaged, "a.txt")) != "changed\n" {
		t.Fatal("refused removal changed files")
	}

	// A clean, unlocked linked worktree goes; its branch stays.
	list, err := r.RemoveWorktree(ctx, clean)
	if err != nil || len(list) != 6 {
		t.Fatalf("%v %d", err, len(list))
	}
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Fatal("directory kept")
	}
	if e.git(main, "branch", "--list", "clean") == "" {
		t.Fatal("branch deleted")
	}
	// Other worktrees' files and indexes are as they were.
	if e.git(main, "diff", "--cached", "--name-only") != "main-work.txt\n" || e.git(staged, "diff", "--cached", "--name-only") != "s.txt\n" {
		t.Fatal("another index changed")
	}
}

func TestRemoveChecksAgainNotTheEarlierList(t *testing.T) {
	e := newEnv(t)
	main := e.repo()
	write(t, filepath.Join(main, "a.txt"), "a\n")
	e.commit(main, "Initial")
	target := addWorktree(t, e, main, "target")
	r, _ := e.open(main)
	ctx := context.Background()
	list, _ := r.Worktrees(ctx)
	if w := worktreeAt(t, list, target); w.Locked {
		t.Fatal("locked already")
	}
	// While the user confirms, the worktree gets work, then a lock.
	write(t, filepath.Join(target, "late.txt"), "late\n")
	_, err := r.RemoveWorktree(ctx, target)
	wantCode(t, err, CodeWorktreeDirty)
	os.Remove(filepath.Join(target, "late.txt"))
	e.git(main, "worktree", "lock", target)
	_, err = r.RemoveWorktree(ctx, target)
	wantCode(t, err, CodeWorktreeLocked)
	// Removed outside GitOwl: it is missing, to prune rather than remove.
	e.git(main, "worktree", "unlock", target)
	os.RemoveAll(target)
	_, err = r.RemoveWorktree(ctx, target)
	wantCode(t, err, CodeWorktreeMissing)
}

func TestPruneOnlyStaleUnlockedRecords(t *testing.T) {
	e := newEnv(t)
	main := e.repo()
	write(t, filepath.Join(main, "a.txt"), "a\n")
	e.commit(main, "Initial")
	gone := addWorktree(t, e, main, "gone")
	goneLocked := addWorktree(t, e, main, "gone-locked")
	kept := addWorktree(t, e, main, "kept")
	e.git(main, "worktree", "lock", goneLocked)
	os.RemoveAll(gone)
	os.RemoveAll(goneLocked)
	r, _ := e.open(main)
	ctx := context.Background()

	cands, err := r.PruneCandidates(ctx)
	if err != nil || len(cands) != 1 || cands[0].Path != gone || cands[0].PrunableReason == "" {
		t.Fatalf("candidates %+v %v", cands, err)
	}
	pruned, list, err := r.Prune(ctx)
	if err != nil || len(pruned) != 1 || pruned[0].Path != gone || len(list) != 3 {
		t.Fatalf("pruned %+v list %d %v", pruned, len(list), err)
	}
	worktreeAt(t, list, goneLocked) // the lock is respected
	worktreeAt(t, list, kept)       // a real directory stays
	if _, err := os.Stat(kept); err != nil {
		t.Fatal(err)
	}
	if e.git(main, "branch", "--list", "gone") == "" {
		t.Fatal("prune deleted a branch")
	}
	// Nothing left to prune: an empty result, not a fake success.
	cands, _ = r.PruneCandidates(ctx)
	pruned, _, err = r.Prune(ctx)
	if err != nil || len(cands) != 0 || len(pruned) != 0 {
		t.Fatalf("second prune %+v %+v %v", cands, pruned, err)
	}
	if !strings.Contains(e.git(main, "worktree", "list"), "gone-locked") {
		t.Fatal("locked record pruned")
	}
}
