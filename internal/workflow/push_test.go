package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (e *env) rev(dir, ref string) string {
	e.t.Helper()
	return strings.TrimSpace(e.git(dir, "rev-parse", ref))
}

// config returns the configuration entries matching pattern, "" for none.
func (e *env) config(dir, pattern string) string {
	e.t.Helper()
	out, _, err := e.Runner.RunStatus(context.Background(), dir, nil, []int{1}, "config", "--get-regexp", pattern)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(out)
}

func chmodX(path string) error { return os.Chmod(path, 0o755) }

// refs lists every ref of a repository with its object.
func (e *env) refs(dir string) string {
	e.t.Helper()
	return e.git(dir, "for-each-ref", "--format=%(refname) %(objectname)")
}

func TestPushUpstream(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	write(t, filepath.Join(rs.mine, "a.txt"), "unstaged\n")
	index := e.git(rs.mine, "ls-files", "-s")
	repo, snap := e.open(rs.mine)
	if snap.Ahead != 1 {
		t.Fatalf("ahead = %d", snap.Ahead)
	}
	info, _ := repo.Remotes(context.Background())
	if info.Push == nil || info.Push.String() != "origin/main" {
		t.Fatalf("push target %+v", info.Push)
	}
	snap, res, err := repo.Push(context.Background(), "main", *info.Push, false)
	if err != nil || res.UpToDate {
		t.Fatalf("push: %+v, %v", res, err)
	}
	if e.rev(rs.bare, "main") != e.rev(rs.mine, "HEAD") || snap.Ahead != 0 {
		t.Fatalf("remote main %s, ahead %d", e.rev(rs.bare, "main"), snap.Ahead)
	}
	if e.git(rs.mine, "ls-files", "-s") != index || readFile(t, filepath.Join(rs.mine, "a.txt")) != "unstaged\n" {
		t.Fatal("push changed the index or a file")
	}
	_, res, err = repo.Push(context.Background(), "main", *info.Push, false)
	if err != nil || !res.UpToDate {
		t.Fatalf("second push: %+v, %v", res, err)
	}
}

func TestPublishBranchSetsUpstream(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "switch", "-q", "-c", "feature/x")
	write(t, filepath.Join(rs.mine, "b.txt"), "b\n")
	e.commit(rs.mine, "Feature")
	repo, _ := e.open(rs.mine)
	info, _ := repo.Remotes(context.Background())
	if info.Push != nil || info.PublishRemote() != "origin" || info.Branch != "feature/x" {
		t.Fatalf("unpublished branch: %+v", info)
	}
	// A failed publish writes no upstream.
	_, _, err := repo.Push(context.Background(), "feature/x", PushTarget{Remote: "origin", Branch: "bad..name"}, true)
	wantCode(t, err, CodeInvalidBranchName)
	if e.config(rs.mine, "^branch\\.feature") != "" {
		t.Fatal("upstream written after a failure")
	}
	_, res, err := repo.Push(context.Background(), "feature/x", PushTarget{Remote: "origin", Branch: "team/x"}, true)
	if err != nil || !res.Tracking {
		t.Fatalf("publish: %+v, %v", res, err)
	}
	if e.rev(rs.bare, "team/x") != e.rev(rs.mine, "HEAD") {
		t.Fatal("remote branch not created")
	}
	if got := strings.TrimSpace(e.git(rs.mine, "rev-parse", "--abbrev-ref", "feature/x@{upstream}")); got != "origin/team/x" {
		t.Fatalf("upstream %q", got)
	}
	info, _ = repo.Remotes(context.Background())
	if info.Push == nil || info.Push.String() != "origin/team/x" {
		t.Fatalf("after publish: %+v", info.Push)
	}
}

func TestPublishUpToDateStillTracks(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "branch", "--unset-upstream")
	repo, _ := e.open(rs.mine)
	_, res, err := repo.Push(context.Background(), "main", PushTarget{Remote: "origin", Branch: "main"}, true)
	if err != nil || !res.UpToDate || !res.Tracking {
		t.Fatalf("publish up to date: %+v, %v", res, err)
	}
	if got := strings.TrimSpace(e.git(rs.mine, "rev-parse", "--abbrev-ref", "main@{upstream}")); got != "origin/main" {
		t.Fatalf("upstream %q", got)
	}
}

func TestPushRejectedKeepsEverything(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	head := e.rev(rs.mine, "HEAD")
	repo, _ := e.open(rs.mine)
	snap, _, err := repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, false)
	wantCode(t, err, CodePushRejected)
	if e.rev(rs.bare, "main") != theirs || e.rev(rs.mine, "HEAD") != head || snap.Root == "" {
		t.Fatal("rejected push moved a branch")
	}
}

func TestPushRemoteRejected(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	write(t, filepath.Join(rs.bare, "hooks", "pre-receive"), "#!/bin/sh\necho 'main is protected' >&2\nexit 1\n")
	e.git(rs.bare, "config", "core.hooksPath", filepath.Join(rs.bare, "hooks"))
	if err := chmodX(filepath.Join(rs.bare, "hooks", "pre-receive")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	before := e.rev(rs.bare, "main")
	repo, _ := e.open(rs.mine)
	_, _, err := repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, false)
	wantCode(t, err, CodePushRemoteRejected)
	if e.rev(rs.bare, "main") != before {
		t.Fatal("remote updated")
	}
}

func TestPushTouchesOneBranchOnly(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	// Settings that would widen a plain git push.
	e.git(rs.mine, "config", "push.default", "matching")
	e.git(rs.mine, "config", "push.followTags", "true")
	e.git(rs.mine, "config", "remote.origin.push", "refs/heads/*:refs/heads/*")
	e.git(rs.mine, "branch", "other")
	e.git(rs.bare, "branch", "other")
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	e.git(rs.mine, "update-ref", "refs/heads/other", "HEAD")
	e.git(rs.mine, "tag", "-a", "-m", "v1", "v1")
	before := e.refs(rs.bare)
	repo, _ := e.open(rs.mine)
	if _, _, err := repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, false); err != nil {
		t.Fatal(err)
	}
	after := e.refs(rs.bare)
	changed := 0
	for _, line := range strings.Split(after, "\n") {
		if !strings.Contains(before, line) {
			changed++
			if !strings.HasPrefix(line, "refs/heads/main ") {
				t.Fatalf("push changed %s", line)
			}
		}
	}
	if changed != 1 {
		t.Fatalf("%d refs changed:\n%s", changed, after)
	}
}

func TestPushRemoteAndDifferentNames(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	fork := filepath.Join(canonical(t.TempDir()), "fork.git")
	e.git(rs.mine, "clone", "-q", "--bare", rs.bare, fork)
	e.git(rs.mine, "remote", "add", "fork", fork)
	e.git(rs.mine, "switch", "-q", "-c", "local-name", "--track", "origin/main")
	repo, _ := e.open(rs.mine)
	info, _ := repo.Remotes(context.Background())
	// Same remote as the upstream: the upstream branch, whatever its name.
	if info.Push == nil || info.Push.String() != "origin/main" {
		t.Fatalf("target %+v", info.Push)
	}
	e.git(rs.mine, "config", "branch.local-name.pushRemote", "fork")
	info, _ = repo.Remotes(context.Background())
	// Another remote: a branch of the same name there.
	if info.Push == nil || info.Push.String() != "fork/local-name" {
		t.Fatalf("pushRemote target %+v", info.Push)
	}
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	originMain := e.rev(rs.bare, "main")
	if _, _, err := repo.Push(context.Background(), "local-name", *info.Push, false); err != nil {
		t.Fatal(err)
	}
	if e.rev(fork, "local-name") != e.rev(rs.mine, "HEAD") || e.rev(rs.bare, "main") != originMain {
		t.Fatal("pushed to the wrong place")
	}
}

func TestPushRefusesDetachedUnbornAndChangedBranch(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	repo, _ := e.open(rs.mine)
	_, _, err := repo.Push(context.Background(), "other", PushTarget{"origin", "main"}, false)
	wantCode(t, err, CodeBranchChanged)
	_, _, err = repo.Push(context.Background(), "main", PushTarget{"--mirror", "main"}, false)
	wantCode(t, err, CodeRemoteNotFound)
	e.git(rs.mine, "switch", "-q", "--detach")
	_, _, err = repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, false)
	wantCode(t, err, CodePushDetached)

	unborn := e.repo()
	e.git(unborn, "remote", "add", "origin", rs.bare)
	repo, _ = e.open(unborn)
	_, _, err = repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, true)
	wantCode(t, err, CodePushUnborn)
}

func TestPushTimeoutIsUncertain(t *testing.T) {
	e := newEnv(t)
	fakeSSH(t, e, "sleep 30")
	repo := sshRepo(t, e)
	e.app.SetNetworkTimeout(300 * time.Millisecond)
	_, _, err := repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, true)
	wantCode(t, err, CodePushUncertain)
	if e.config(repo.Root(), "^branch\\.main") != "" {
		t.Fatal("upstream written after an uncertain push")
	}
}

func TestPushAuthFailure(t *testing.T) {
	e := newEnv(t)
	fakeSSH(t, e, "echo 'git@example.invalid: Permission denied (publickey).' >&2\nexit 255")
	repo := sshRepo(t, e)
	_, _, err := repo.Push(context.Background(), "main", PushTarget{"origin", "main"}, false)
	wantCode(t, err, CodeAuthFailed)
}

func TestPushWithForceWithLease(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	repo, _ := e.open(rs.mine)
	if _, err := repo.Fetch(context.Background(), "origin"); err != nil {
		t.Fatal(err)
	}
	_, _, err := repo.PushWith(context.Background(), PushOptions{Remote: "origin", Branch: "main"})
	wantCode(t, err, CodePushRejected)
	if e.rev(rs.bare, "main") != theirs {
		t.Fatal("rejected push moved the remote")
	}
	mine := e.rev(rs.mine, "HEAD")
	_, res, err := repo.PushWith(context.Background(), PushOptions{
		Remote: "origin", Branch: "main", ForceWithLease: true,
	})
	if err != nil || res.UpToDate {
		t.Fatalf("force with lease: %+v, %v", res, err)
	}
	if e.rev(rs.bare, "main") != mine {
		t.Fatal("force with lease did not update the remote")
	}
}

func TestPushWithTags(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	e.git(rs.mine, "tag", "v1")
	repo, _ := e.open(rs.mine)
	if _, _, err := repo.PushWith(context.Background(), PushOptions{Remote: "origin", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.refs(rs.bare), "refs/tags/v1 ") {
		t.Fatal("push without tags sent the tag")
	}
	_, _, err := repo.PushWith(context.Background(), PushOptions{
		Remote: "origin", Branch: "main", PushTags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	after := e.refs(rs.bare)
	if !strings.Contains(after, "refs/tags/v1 ") {
		t.Fatalf("push tags did not send v1:\n%s", after)
	}
	if !strings.Contains(after, "refs/heads/main ") {
		t.Fatal("push tags dropped the branch")
	}
}

func TestPushWithCreatesNewRemoteBranch(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	write(t, filepath.Join(rs.mine, "a.txt"), "mine\n")
	e.commit(rs.mine, "Mine")
	repo, _ := e.open(rs.mine)
	mine := e.rev(rs.mine, "HEAD")
	originMain := e.rev(rs.bare, "main")
	_, res, err := repo.PushWith(context.Background(), PushOptions{
		Remote: "origin", Branch: "new-on-origin",
	})
	if err != nil || res.Target.String() != "origin/new-on-origin" {
		t.Fatalf("push new remote branch: %+v, %v", res, err)
	}
	if e.rev(rs.bare, "new-on-origin") != mine || e.rev(rs.bare, "main") != originMain {
		t.Fatal("new remote branch not created, or main moved")
	}
}
