package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// staged stages a new file and returns the fresh snapshot with its
// StagedFingerprint, ready for a commit.
func staged(t *testing.T, r *Repository, dir, file, content string) Snapshot {
	t.Helper()
	write(t, filepath.Join(dir, file), content)
	snap, err := r.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snap, err = r.Stage(context.Background(), find(t, snap.Untracked, file))
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestCommitAndPushUpstream(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	info, _ := repo.Remotes(ctx)
	if info.Push == nil {
		t.Fatalf("no push target: %+v", info)
	}
	snap := staged(t, repo, rs.mine, "new.txt", "mine\n")
	req := CommitPushRequest{
		Message: "Add a", Staged: snap.StagedFingerprint,
		Branch: info.Branch, Target: *info.Push,
	}
	after, res, err := repo.CommitAndPush(ctx, req)
	if err != nil {
		t.Fatalf("commit and push: %v", err)
	}
	if res.Committed.Subject != "Add a" || res.PushErr != nil {
		t.Fatalf("result %+v", res)
	}
	if e.rev(rs.bare, "main") != e.rev(rs.mine, "HEAD") || after.Ahead != 0 {
		t.Fatalf("remote main %s, ahead %d", e.rev(rs.bare, "main"), after.Ahead)
	}
}

func TestCommitAndPushPublishesNewBranch(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "switch", "-q", "-c", "feature/x")
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	info, _ := repo.Remotes(ctx)
	if info.Push != nil {
		t.Fatalf("unexpected push target %+v", info.Push)
	}
	snap := staged(t, repo, rs.mine, "b.txt", "b\n")
	req := CommitPushRequest{
		Message: "Feature", Staged: snap.StagedFingerprint, Branch: "feature/x",
		Target: PushTarget{Remote: "origin", Branch: "team/x"}, Publish: true,
	}
	_, res, err := repo.CommitAndPush(ctx, req)
	if err != nil || res.PushErr != nil || !res.Push.Tracking {
		t.Fatalf("publish: %+v, %v", res, err)
	}
	if e.rev(rs.bare, "team/x") != e.rev(rs.mine, "HEAD") {
		t.Fatal("remote branch not created")
	}
	if got := strings.TrimSpace(e.git(rs.mine, "rev-parse", "--abbrev-ref", "feature/x@{upstream}")); got != "origin/team/x" {
		t.Fatalf("upstream %q", got)
	}
}

func TestCommitAndPushRefusesMissingTargetWithoutCommitting(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "switch", "-q", "-c", "feature/x")
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	snap := staged(t, repo, rs.mine, "b.txt", "b\n")
	// No upstream and no Publish target: refuse before committing.
	req := CommitPushRequest{Message: "Feature", Staged: snap.StagedFingerprint, Branch: "feature/x"}
	after, res, err := repo.CommitAndPush(ctx, req)
	wantCode(t, err, CodeCommitPushNoTarget)
	if res.Committed.Hash != "" {
		t.Fatalf("committed anyway: %+v", res)
	}
	if out := strings.TrimSpace(e.git(rs.mine, "rev-list", "--count", "HEAD")); out != "1" {
		t.Fatalf("history changed: %q commits", out)
	}
	// The staged change is still staged, uncommitted.
	if len(after.Staged) != 1 {
		t.Fatalf("staged lost: %+v", paths(after.Staged))
	}
}

func TestCommitAndPushKeepsCommitWhenPushRejected(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "c.txt", "c\n")
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	info, _ := repo.Remotes(ctx)
	snap := staged(t, repo, rs.mine, "new.txt", "mine\n")
	req := CommitPushRequest{
		Message: "Add new", Staged: snap.StagedFingerprint,
		Branch: info.Branch, Target: *info.Push,
	}
	after, res, err := repo.CommitAndPush(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Committed.Hash == "" || res.PushErr == nil {
		t.Fatalf("want commit kept and push error: %+v", res)
	}
	wantCode(t, res.PushErr, CodePushRejected)
	if after.Head != res.Committed.Hash {
		t.Fatalf("HEAD %s != commit %s", after.Head, res.Committed.Hash)
	}
}

func TestCommitAndPushCommitFailureDoesNotPush(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	info, _ := repo.Remotes(ctx)
	snap, _ := repo.Status(ctx)
	// Nothing staged: the commit fails, so nothing is pushed.
	req := CommitPushRequest{
		Message: "Nothing", Staged: snap.StagedFingerprint,
		Branch: info.Branch, Target: *info.Push,
	}
	before := e.rev(rs.bare, "main")
	_, res, err := repo.CommitAndPush(ctx, req)
	wantCode(t, err, CodeNothingStaged)
	if res.Committed.Hash != "" || res.PushErr != nil {
		t.Fatalf("pushed after a failed commit: %+v", res)
	}
	if e.rev(rs.bare, "main") != before {
		t.Fatal("remote changed after a failed commit")
	}
}

func TestRetryPushSucceedsAfterResolving(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "c.txt", "c\n")
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	info, _ := repo.Remotes(ctx)
	theirs := e.rev(rs.bare, "main")
	snap := staged(t, repo, rs.mine, "new.txt", "mine\n")
	req := CommitPushRequest{
		Message: "Add new", Staged: snap.StagedFingerprint,
		Branch: info.Branch, Target: *info.Push,
	}
	_, res, _ := repo.CommitAndPush(ctx, req)
	wantCode(t, res.PushErr, CodePushRejected)
	// Resolve by fast-forwarding onto their commit, keeping our commit on top.
	e.git(rs.mine, "fetch", "-q", "origin")
	e.git(rs.mine, "rebase", "-q", "origin/main")
	head := e.rev(rs.mine, "HEAD")
	after, pres, err := repo.RetryPush(ctx, req, Commit{Hash: head})
	if err != nil || pres.UpToDate {
		t.Fatalf("retry push: %+v, %v", pres, err)
	}
	if e.rev(rs.bare, "main") != head || after.Ahead != 0 {
		t.Fatalf("remote main %s, ahead %d", e.rev(rs.bare, "main"), after.Ahead)
	}
	// The remote has exactly one new commit over their commit.
	if n := strings.Count(e.git(rs.bare, "log", "--oneline", theirs+"..main"), "\n"); n != 1 {
		t.Fatalf("remote has %d new commits", n)
	}
}

func TestRetryPushRefusesWhenHeadMoved(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "c.txt", "c\n")
	repo, _ := e.open(rs.mine)
	ctx := context.Background()
	info, _ := repo.Remotes(ctx)
	snap := staged(t, repo, rs.mine, "new.txt", "mine\n")
	req := CommitPushRequest{
		Message: "Add new", Staged: snap.StagedFingerprint,
		Branch: info.Branch, Target: *info.Push,
	}
	_, res, _ := repo.CommitAndPush(ctx, req)
	original := res.Committed
	// A new commit moves HEAD away from the one RetryPush was given.
	snap2 := staged(t, repo, rs.mine, "more.txt", "more\n")
	if _, _, err := repo.Commit(ctx, "More", snap2.StagedFingerprint); err != nil {
		t.Fatal(err)
	}
	_, _, err := repo.RetryPush(ctx, req, original)
	wantCode(t, err, CodeCommitPushStale)
}
