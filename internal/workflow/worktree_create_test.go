package workflow

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestWorktreePath(t *testing.T) {
	parent := canonical(t.TempDir())
	main := filepath.Join(parent, "app")
	if got := SuggestWorktreePath(main, "feature/login form"); got != filepath.Join(parent, "app-feature-login-form") {
		t.Fatal(got)
	}
	if got := SuggestWorktreePath(main, "修复/登录"); got != filepath.Join(parent, "app-修复-登录") {
		t.Fatal(got)
	}
	os.Mkdir(filepath.Join(parent, "app-x"), 0o755)
	os.WriteFile(filepath.Join(parent, "app-x-2"), nil, 0o644)
	if got := SuggestWorktreePath(main, "x"); got != filepath.Join(parent, "app-x-3") {
		t.Fatal(got)
	}
	if got := SuggestWorktreePath(main, "../.."); got != filepath.Join(parent, "app-worktree") && filepath.Dir(got) != parent {
		t.Fatal(got)
	}
	if got := SuggestWorktreePath(main, "../.."); filepath.Dir(got) != parent {
		t.Fatalf("escaped the parent: %s", got)
	}
}

func TestCreateWorktreeFromNewAndExistingBranch(t *testing.T) {
	e := newEnv(t)
	main := e.repo()
	write(t, filepath.Join(main, "a.txt"), "a\n")
	e.commit(main, "Initial")
	e.git(main, "branch", "existing")
	write(t, filepath.Join(main, "a.txt"), "second\n")
	e.commit(main, "Second")
	write(t, filepath.Join(main, "wip.txt"), "uncommitted\n")
	e.git(main, "add", "wip.txt")
	r, _ := e.open(main)
	ctx := context.Background()
	parent := filepath.Dir(main)

	// A new branch at the current HEAD.
	path := SuggestWorktreePath(main, "feature/new")
	list, branches, root, err := r.CreateWorktree(ctx, NewWorktree{Branch: "feature/new", Create: true, Path: path})
	if err != nil || root != path || filepath.Dir(root) != parent {
		t.Fatalf("%v %s", err, root)
	}
	if w := worktreeAt(t, list, root); w.Branch != "feature/new" || w.Head != e.git(main, "rev-parse", "HEAD")[:40] {
		t.Fatalf("new %+v", w)
	}
	if _, ok := branches.local("feature/new"); !ok {
		t.Fatal("branch not listed")
	}
	// The current worktree's index and files are as they were.
	if e.git(main, "diff", "--cached", "--name-only") != "wip.txt\n" || readFile(t, filepath.Join(main, "a.txt")) != "second\n" {
		t.Fatal("current worktree changed")
	}
	if _, err := os.Stat(filepath.Join(root, "wip.txt")); !os.IsNotExist(err) {
		t.Fatal("uncommitted file copied")
	}

	// An existing branch, at its own commit.
	other := filepath.Join(parent, "other place")
	list, _, root, err = r.CreateWorktree(ctx, NewWorktree{Branch: "existing", Path: other})
	if err != nil || readFile(t, filepath.Join(root, "a.txt")) != "a\n" || worktreeAt(t, list, root).Branch != "existing" {
		t.Fatalf("existing: %v", err)
	}

	// Git's rules stand.
	for _, tc := range []struct {
		spec NewWorktree
		code Code
	}{
		{NewWorktree{Branch: "existing", Path: filepath.Join(parent, "again")}, CodeBranchInUse},
		{NewWorktree{Branch: "main", Path: filepath.Join(parent, "main-again")}, CodeBranchInUse},
		{NewWorktree{Branch: "nope", Path: filepath.Join(parent, "nope")}, CodeBranchNotFound},
		{NewWorktree{Branch: "existing", Create: true, Path: filepath.Join(parent, "dup")}, CodeBranchExists},
		{NewWorktree{Branch: "bad..name", Create: true, Path: filepath.Join(parent, "bad")}, CodeInvalidBranchName},
		{NewWorktree{Branch: "fresh", Create: true, Path: other}, CodePathExists},
		{NewWorktree{Branch: "fresh", Create: true, Path: "relative/dir"}, CodeInvalidWorktreePath},
	} {
		list, _, _, err := r.CreateWorktree(ctx, tc.spec)
		wantCode(t, err, tc.code)
		if len(list) != 3 {
			t.Fatalf("%v: list after failure %+v", tc.code, list)
		}
	}
	if out := e.git(main, "branch", "--list", "fresh"); out != "" {
		t.Fatal("a failed creation made a branch")
	}
	// A directory that cannot be written.
	locked := filepath.Join(parent, "readonly")
	os.Mkdir(locked, 0o555)
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	_, _, _, err = r.CreateWorktree(ctx, NewWorktree{Branch: "fresh2", Create: true, Path: filepath.Join(locked, "wt")})
	if c := CodeOf(err); c != CodeNoAccess && c != CodeGitFailed {
		t.Fatalf("read-only parent: %v", err)
	}
	if _, err := os.Stat(filepath.Join(locked, "wt")); !os.IsNotExist(err) {
		t.Fatal("directory made in a read-only parent")
	}
}

func TestCreateWorktreeNeedsACommit(t *testing.T) {
	e := newEnv(t)
	main := e.repo()
	r, _ := e.open(main)
	_, _, _, err := r.CreateWorktree(context.Background(), NewWorktree{Branch: "x", Create: true, Path: filepath.Join(filepath.Dir(main), "x")})
	wantCode(t, err, CodeNoCommitsForWorktree)
	// A closed handle refuses.
	e.open(e.repo())
	_, _, _, err = r.CreateWorktree(context.Background(), NewWorktree{Branch: "x", Create: true, Path: filepath.Join(filepath.Dir(main), "y")})
	wantCode(t, err, CodeRepositoryClosed)
}
