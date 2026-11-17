package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitowl/internal/gittest"
)

func TestFirstCommitAndFollowUpKeepUnstagedWork(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	write(t, filepath.Join(dir, "b.txt"), "b\n")
	r, snap := e.open(dir)
	ctx := context.Background()
	snap, err := r.Stage(ctx, find(t, snap.Untracked, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	snap, c, err := r.Commit(ctx, "Add a\n\nThe first file.\n", snap.StagedFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Unborn() || c.Hash != snap.Head || c.Subject != "Add a" {
		t.Fatalf("first commit %+v %+v", c, snap)
	}
	if got := e.git(dir, "log", "-1", "--format=%B"); got != "Add a\n\nThe first file.\n\n" {
		t.Fatalf("message %q", got)
	}
	// b.txt was not staged and stays in the working tree.
	if join(paths(snap.Untracked)) != "b.txt" || len(snap.Staged) != 0 {
		t.Fatalf("after first commit %+v", snap)
	}

	write(t, filepath.Join(dir, "a.txt"), "a2\n")
	snap, _ = r.Status(ctx)
	snap, _ = r.Stage(ctx, find(t, snap.Unstaged, "a.txt"))
	write(t, filepath.Join(dir, "a.txt"), "a3 unstaged\n")
	snap, _ = r.Status(ctx)
	snap, c2, err := r.Commit(ctx, "Update a", snap.StagedFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.git(dir, "show", c2.Hash+":a.txt"); got != "a2\n" {
		t.Fatalf("committed content %q", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "a.txt")); string(data) != "a3 unstaged\n" {
		t.Fatalf("working tree %q", data)
	}
	if join(paths(snap.Unstaged)) != "a.txt" {
		t.Fatalf("unstaged after commit %v", paths(snap.Unstaged))
	}
	if n := strings.Count(e.git(dir, "log", "--oneline"), "\n"); n != 2 {
		t.Fatalf("%d commits", n)
	}
}

func TestCommitRefusesEmptyInput(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	r, snap := e.open(dir)
	ctx := context.Background()
	_, _, err := r.Commit(ctx, "Message", snap.StagedFingerprint)
	wantCode(t, err, CodeNothingStaged)
	snap, _ = r.Stage(ctx, find(t, snap.Untracked, "a.txt"))
	_, _, err = r.Commit(ctx, "  \n\t\n", snap.StagedFingerprint)
	wantCode(t, err, CodeEmptyMessage)
	if out := e.git(dir, "rev-list", "--all"); out != "" {
		t.Fatalf("a commit was created: %q", out)
	}
}

func TestCommitRefusesChangedStagedContent(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, snap := e.open(dir)
	write(t, filepath.Join(dir, "sneaky.txt"), "staged elsewhere\n")
	e.git(dir, "add", "sneaky.txt")
	fresh, _, err := r.Commit(context.Background(), "Add a", snap.StagedFingerprint)
	wantCode(t, err, CodeStateChanged)
	if len(fresh.Staged) != 2 {
		t.Fatalf("fresh state %v", paths(fresh.Staged))
	}
	if out := e.git(dir, "rev-list", "--all"); out != "" {
		t.Fatal("committed unreviewed content")
	}
	// The closed handle of a switched repository refuses to commit.
	e.open(e.repo())
	_, _, err = r.Commit(context.Background(), "Add a", fresh.StagedFingerprint)
	wantCode(t, err, CodeRepositoryClosed)
}

func TestCommitWithoutIdentityExplainsAndKeepsState(t *testing.T) {
	e := newEnv(t)
	gittest.Write(t, e.Config, "[init]\n\tdefaultBranch = main\n")
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, snap := e.open(dir)
	after, _, err := r.Commit(context.Background(), "Add a", snap.StagedFingerprint)
	wantCode(t, err, CodeIdentityMissing)
	if len(after.Staged) != 1 || !after.Unborn() {
		t.Fatalf("state after failure %+v", after)
	}
}

func TestCommitHookFailureReportsGit(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	write(t, hook, "#!/bin/sh\necho 'lint failed: fix me' >&2\nexit 1\n")
	os.Chmod(hook, 0o755)
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, snap := e.open(dir)
	after, _, err := r.Commit(context.Background(), "Add a", snap.StagedFingerprint)
	wantCode(t, err, CodeGitFailed)
	if !strings.Contains(err.Error(), "lint failed") || len(after.Staged) != 1 {
		t.Fatalf("%v %+v", err, after)
	}
}
