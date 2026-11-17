package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseWorktrees(t *testing.T) {
	out := "worktree /src/app\x00HEAD 1111111111111111111111111111111111111111\x00branch refs/heads/main\x00\x00" +
		"worktree /src/app-feature x\x00HEAD 2222222222222222222222222222222222222222\x00branch refs/heads/feature/x\x00locked moved to a disk\x00\x00" +
		"worktree /src/app-detached\x00HEAD 3333333333333333333333333333333333333333\x00detached\x00\x00" +
		"worktree /gone\x00HEAD 4444444444444444444444444444444444444444\x00branch refs/heads/old\x00prunable gitdir file points to non-existent location\x00\x00" +
		"worktree /src/new\x00HEAD 0000000000000000000000000000000000000000\x00branch refs/heads/unborn\x00locked\x00\x00"
	got := ParseWorktrees([]byte(out))
	if len(got) != 5 {
		t.Fatalf("%d entries: %+v", len(got), got)
	}
	if w := got[0]; w.Path != "/src/app" || w.Branch != "main" || w.Detached || w.Locked {
		t.Errorf("main %+v", w)
	}
	if w := got[1]; w.Path != "/src/app-feature x" || w.Branch != "feature/x" || !w.Locked || w.LockReason != "moved to a disk" {
		t.Errorf("locked %+v", w)
	}
	if w := got[2]; !w.Detached || w.Branch != "" || w.Head == "" {
		t.Errorf("detached %+v", w)
	}
	if w := got[3]; !w.Prunable || w.PrunableReason != "gitdir file points to non-existent location" {
		t.Errorf("prunable %+v", w)
	}
	if w := got[4]; w.Head != "" || !w.Locked || w.LockReason != "" || w.Branch != "unborn" {
		t.Errorf("unborn %+v", w)
	}
}

func TestWorktreeListFromGit(t *testing.T) {
	r, dir := testRunner(t)
	ctx := context.Background()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644)
	run := func(args ...string) {
		t.Helper()
		if _, err := r.Run(ctx, dir, nil, append([]string{"-c", "user.name=T", "-c", "user.email=t@t.invalid"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", "a")
	linked := filepath.Join(t.TempDir(), "linked dir")
	run("worktree", "add", "-q", "--detach", linked)
	run("worktree", "lock", "--reason", "on purpose", linked)
	out, err := r.Run(ctx, dir, nil, WorktreeArgs...)
	if err != nil {
		t.Fatal(err)
	}
	got := ParseWorktrees(out)
	if len(got) != 2 || !got[1].Detached || !got[1].Locked || got[1].LockReason != "on purpose" || filepath.Base(got[1].Path) != "linked dir" {
		t.Fatalf("%+v", got)
	}
}
