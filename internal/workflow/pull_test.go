package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPullFastForwards(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "config", "pull.rebase", "true") // ignored: Pull only fast-forwards
	e.git(rs.mine, "config", "pull.ff", "false")
	rs.theirCommit(t, e, "b.txt", "b\n")
	theirs := rs.theirCommit(t, e, "c.txt", "c\n")
	repo, _ := e.open(rs.mine)
	snap, res, err := repo.Pull(context.Background(), "main")
	if err != nil || res.Commits != 2 || res.Upstream != "origin/main" {
		t.Fatalf("pull: %+v, %v", res, err)
	}
	if e.rev(rs.mine, "HEAD") != theirs || snap.Head != theirs || snap.Behind != 0 || !snap.Clean() {
		t.Fatalf("after pull: %+v", snap)
	}
	if readFile(t, filepath.Join(rs.mine, "c.txt")) != "c\n" {
		t.Fatal("files not updated")
	}
	// No merge commit: HEAD is their commit itself.
	if got := e.git(rs.mine, "rev-list", "--merges", "--count", "HEAD"); got != "0\n" {
		t.Fatalf("merges %s", got)
	}
	_, res, err = repo.Pull(context.Background(), "main")
	if err != nil || res.Commits != 0 {
		t.Fatalf("up to date pull: %+v, %v", res, err)
	}
}

func TestPullRefusesDivergedButKeepsFetch(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "m.txt"), "m\n")
	e.commit(rs.mine, "Mine")
	head := e.rev(rs.mine, "HEAD")
	repo, _ := e.open(rs.mine)
	snap, _, err := repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePullDiverged)
	if e.rev(rs.mine, "HEAD") != head || e.rev(rs.mine, "origin/main") != theirs {
		t.Fatal("diverged pull moved HEAD or did not fetch")
	}
	if snap.Ahead != 1 || snap.Behind != 1 {
		t.Fatalf("ahead/behind after diverged pull: %+v", snap)
	}
}

func TestPullNeedsCleanWorktree(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "b.txt", "b\n")
	head := e.rev(rs.mine, "HEAD")
	repo, _ := e.open(rs.mine)

	write(t, filepath.Join(rs.mine, "untracked.txt"), "u\n")
	_, _, err := repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePullDirty)
	os.Remove(filepath.Join(rs.mine, "untracked.txt"))

	write(t, filepath.Join(rs.mine, "a.txt"), "edited\n")
	_, _, err = repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePullDirty)
	e.git(rs.mine, "add", "a.txt")
	_, _, err = repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePullDirty)
	if e.rev(rs.mine, "HEAD") != head || readFile(t, filepath.Join(rs.mine, "a.txt")) != "edited\n" {
		t.Fatal("refused pull changed something")
	}
	// Ignored files are not work in progress.
	e.git(rs.mine, "reset", "-q", "--hard")
	write(t, filepath.Join(rs.mine, ".git", "info", "exclude"), "*.log\n")
	write(t, filepath.Join(rs.mine, "build.log"), "x\n")
	if _, _, err := repo.Pull(context.Background(), "main"); err != nil {
		t.Fatal(err)
	}
}

func TestPullRefusesInProgressAndMissingUpstream(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	repo, _ := e.open(rs.mine)
	write(t, filepath.Join(rs.mine, ".git", "MERGE_HEAD"), e.rev(rs.mine, "HEAD")+"\n")
	_, _, err := repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePullInProgress)
	os.Remove(filepath.Join(rs.mine, ".git", "MERGE_HEAD"))

	e.git(rs.mine, "branch", "--unset-upstream")
	_, _, err = repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePullNoUpstream)

	e.git(rs.mine, "switch", "-q", "--detach")
	_, _, err = repo.Pull(context.Background(), "main")
	wantCode(t, err, CodePushDetached)

	_, _, err = repo.Pull(context.Background(), "other")
	wantCode(t, err, CodePushDetached)
}

func TestPullCancelKeepsHead(t *testing.T) {
	e := newEnv(t)
	started := filepath.Join(t.TempDir(), "started")
	fakeSSH(t, e, "touch "+started+"\nsleep 30")
	repo := sshRepo(t, e)
	dir := repo.Root()
	e.git(dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	e.git(dir, "branch", "--set-upstream-to=origin/main")
	head := e.rev(dir, "HEAD")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancel once the fetch is talking to the "remote".
		for range 200 {
			if _, err := os.Stat(started); err == nil {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		cancel()
	}()
	snap, _, err := repo.Pull(ctx, "main")
	wantCode(t, err, CodeCancelled)
	if e.rev(dir, "HEAD") != head || snap.Root == "" {
		t.Fatal("cancelled pull moved HEAD")
	}
}

func TestPullKeepsOtherWorktree(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	linked := filepath.Join(canonical(t.TempDir()), "linked")
	e.git(rs.mine, "worktree", "add", "-q", "-b", "side", linked)
	write(t, filepath.Join(linked, "a.txt"), "side work\n")
	rs.theirCommit(t, e, "b.txt", "b\n")
	repo, _ := e.open(rs.mine)
	if _, _, err := repo.Pull(context.Background(), "main"); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(linked, "a.txt")) != "side work\n" {
		t.Fatal("pull changed another worktree")
	}
}

func TestPullStopsWhenHeadMovesDuringFetch(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "b.txt", "b\n")
	// The fetch's upload-pack moves mine's HEAD before serving, as a commit
	// made in Terminal while GitOwl fetched would.
	script := filepath.Join(t.TempDir(), "upload-pack")
	git := e.Runner.Path
	body := "#!/bin/sh\n" +
		"GIT_DIR='" + filepath.Join(rs.mine, ".git") + "' '" + git + "' update-ref refs/heads/main \"$('" + git + "' -C '" + rs.mine + "' commit-tree -p HEAD -m outside 'HEAD^{tree}')\"\n" +
		"exec '" + git + "' upload-pack \"$@\"\n"
	write(t, script, body)
	if err := chmodX(script); err != nil {
		t.Fatal(err)
	}
	e.git(rs.mine, "config", "remote.origin.uploadpack", script)
	repo, _ := e.open(rs.mine)
	_, _, err := repo.Pull(context.Background(), "main")
	wantCode(t, err, CodeStateChanged)
	if got := e.git(rs.mine, "log", "-1", "--format=%s"); got != "outside\n" {
		t.Fatalf("HEAD %q: pull moved it on", got)
	}
}

func TestPullWithFastForwardOnly(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "config", "pull.rebase", "true")
	e.git(rs.mine, "config", "pull.ff", "false")
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	repo, _ := e.open(rs.mine)
	snap, res, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullFastForwardOnly,
	})
	if err != nil || res.Commits != 1 || res.Upstream != "origin/main" {
		t.Fatalf("pull with: %+v, %v", res, err)
	}
	if e.rev(rs.mine, "HEAD") != theirs || snap.Head != theirs || snap.Behind != 0 {
		t.Fatalf("after pull with: %+v", snap)
	}
	if got := e.git(rs.mine, "rev-list", "--merges", "--count", "HEAD"); got != "0\n" {
		t.Fatalf("merges %s", got)
	}
}

func TestPullWithFastForwardOnlyRefusesDiverged(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "m.txt"), "m\n")
	e.commit(rs.mine, "Mine")
	head := e.rev(rs.mine, "HEAD")
	repo, _ := e.open(rs.mine)
	_, _, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullFastForwardOnly,
	})
	wantCode(t, err, CodePullDiverged)
	if e.rev(rs.mine, "HEAD") != head || e.rev(rs.mine, "origin/main") != theirs {
		t.Fatal("diverged pull with moved HEAD or did not fetch")
	}
}

func TestPullWithMerge(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "m.txt"), "m\n")
	e.commit(rs.mine, "Mine")
	repo, _ := e.open(rs.mine)
	snap, res, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullMerge,
	})
	if err != nil || res.Commits != 1 || res.Upstream != "origin/main" {
		t.Fatalf("merge pull: %+v, %v", res, err)
	}
	if snap.Behind != 0 || !snap.Clean() {
		t.Fatalf("after merge pull: %+v", snap)
	}
	if got := e.git(rs.mine, "rev-list", "--merges", "--count", "HEAD"); got != "1\n" {
		t.Fatalf("want a merge commit, got %s", got)
	}
	if readFile(t, filepath.Join(rs.mine, "b.txt")) != "b\n" || readFile(t, filepath.Join(rs.mine, "m.txt")) != "m\n" {
		t.Fatal("merge pull dropped a side")
	}
}

func TestPullWithRebase(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "m.txt"), "m\n")
	e.commit(rs.mine, "Mine")
	repo, _ := e.open(rs.mine)
	snap, res, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullRebase,
	})
	if err != nil || res.Commits != 1 || res.Upstream != "origin/main" {
		t.Fatalf("rebase pull: %+v, %v", res, err)
	}
	if snap.Behind != 0 || !snap.Clean() {
		t.Fatalf("after rebase pull: %+v", snap)
	}
	if got := e.git(rs.mine, "rev-list", "--merges", "--count", "HEAD"); got != "0\n" {
		t.Fatalf("rebase made a merge: %s", got)
	}
	if !strings.Contains(e.git(rs.mine, "log", "--format=%s"), "Mine") {
		t.Fatal("rebase dropped the local commit")
	}
	if parent := e.rev(rs.mine, "HEAD^"); parent != theirs {
		t.Fatalf("HEAD^ %s, want %s", parent, theirs)
	}
	if readFile(t, filepath.Join(rs.mine, "b.txt")) != "b\n" || readFile(t, filepath.Join(rs.mine, "m.txt")) != "m\n" {
		t.Fatal("rebase pull dropped a side")
	}
}

func TestPullWithAutostashAllowsDirtyWorktree(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "a.txt"), "edited\n")
	repo, _ := e.open(rs.mine)
	_, _, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullFastForwardOnly,
	})
	wantCode(t, err, CodePullDirty)
	snap, res, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullFastForwardOnly, Autostash: true,
	})
	if err != nil || res.Commits != 1 {
		t.Fatalf("autostash pull: %+v, %v", res, err)
	}
	if e.rev(rs.mine, "HEAD") != theirs || snap.Head != theirs {
		t.Fatalf("autostash pull did not fast-forward: %+v", snap)
	}
	if readFile(t, filepath.Join(rs.mine, "a.txt")) != "edited\n" {
		t.Fatal("autostash pull dropped local edits")
	}
	if readFile(t, filepath.Join(rs.mine, "b.txt")) != "b\n" {
		t.Fatal("autostash pull did not bring their file")
	}
}

func TestPullWithPrune(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.theirs, "branch", "gone")
	e.git(rs.theirs, "push", "-q", "origin", "gone")
	repo, _ := e.open(rs.mine)
	if _, err := repo.Fetch(context.Background(), "origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Runner.Run(context.Background(), rs.mine, nil, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/gone"); err != nil {
		t.Fatal("origin/gone missing after fetch")
	}
	e.git(rs.theirs, "push", "-q", "origin", "--delete", "gone")
	if _, _, err := repo.PullWith(context.Background(), PullOptions{
		Remote: "origin", Branch: "main", Strategy: PullFastForwardOnly, Prune: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Runner.Run(context.Background(), rs.mine, nil, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/gone"); err == nil {
		t.Fatal("prune left origin/gone")
	}
}

func TestPullWithEmptySelection(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	repo, _ := e.open(rs.mine)
	_, _, err := repo.PullWith(context.Background(), PullOptions{Branch: "main", Strategy: PullFastForwardOnly})
	wantCode(t, err, CodeChooseRemote)
	_, _, err = repo.PullWith(context.Background(), PullOptions{Remote: "origin", Strategy: PullFastForwardOnly})
	wantCode(t, err, CodeInvalidBranchName)
}
