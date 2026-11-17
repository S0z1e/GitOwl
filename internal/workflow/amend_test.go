package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// amendReady commits a single file and returns the repository, its snapshot
// and the AmendTarget, failing the test unless the commit can be amended.
func amendReady(t *testing.T, e *env, dir string) (*Repository, Snapshot, Amendable) {
	t.Helper()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.git(dir, "add", "a.txt")
	r, snap := e.open(dir)
	ctx := context.Background()
	snap, c, err := r.Commit(ctx, "First\n\nBody.\n", snap.StagedFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ = r.Status(ctx)
	tgt, err := r.AmendTarget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.CanAmend || tgt.Hash != c.Hash || tgt.Subject != "First" {
		t.Fatalf("not amendable: %+v", tgt)
	}
	return r, snap, tgt
}

func TestAmendMessageOnlyKeepsParentAndTree(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, snap, tgt := amendReady(t, e, dir)
	parent := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^"))
	tree := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}"))
	ctx := context.Background()
	after, c, err := r.Amend(ctx, AmendRequest{
		Message: "Reworded\n\nNew body.\n", Head: tgt.Hash,
		Staged: snap.StagedFingerprint, MessageOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Subject != "Reworded" || c.Hash == tgt.Hash {
		t.Fatalf("amend result %+v", c)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^")); got != parent {
		t.Fatalf("parent changed: %s", got)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}")); got != tree {
		t.Fatalf("tree changed: %s", got)
	}
	if got := e.git(dir, "log", "-1", "--format=%B"); got != "Reworded\n\nNew body.\n\n" {
		t.Fatalf("message %q", got)
	}
	if c.Hash != after.Head {
		t.Fatalf("snapshot head %s != %s", after.Head, c.Hash)
	}
}

func TestAmendWithStagedUpdatesTree(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, _, tgt := amendReady(t, e, dir)
	parent := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^"))
	oldTree := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}"))
	// Stage a further change to fold into the last commit.
	write(t, filepath.Join(dir, "a.txt"), "a changed\n")
	e.git(dir, "add", "a.txt")
	ctx := context.Background()
	snap, _ := r.Status(ctx)
	after, c, err := r.Amend(ctx, AmendRequest{
		Message: "First plus more\n", Head: tgt.Hash,
		Staged: snap.StagedFingerprint, MessageOnly: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^")); got != parent {
		t.Fatalf("parent changed: %s", got)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}")); got == oldTree {
		t.Fatal("tree unchanged after staged amend")
	}
	if got := e.git(dir, "show", "HEAD:a.txt"); got != "a changed\n" {
		t.Fatalf("committed content %q", got)
	}
	if len(after.Staged) != 0 || c.Subject != "First plus more" {
		t.Fatalf("after amend %+v %+v", after, c)
	}
}

func TestAmendMessageOnlyIgnoresStaged(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, _, tgt := amendReady(t, e, dir)
	tree := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}"))
	// A staged change must stay staged after a message-only amend.
	write(t, filepath.Join(dir, "a.txt"), "a changed\n")
	e.git(dir, "add", "a.txt")
	ctx := context.Background()
	snap, _ := r.Status(ctx)
	after, _, err := r.Amend(ctx, AmendRequest{
		Message: "Reworded\n", Head: tgt.Hash,
		Staged: snap.StagedFingerprint, MessageOnly: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}")); got != tree {
		t.Fatalf("tree changed by message-only amend: %s", got)
	}
	if got := e.git(dir, "show", "HEAD:a.txt"); got != "a\n" {
		t.Fatalf("committed content changed %q", got)
	}
	if len(after.Staged) != 1 {
		t.Fatalf("staged change lost: %+v", paths(after.Staged))
	}
}

func TestAmendTargetRefusesPushedCommit(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	// HEAD of mine equals origin/main after cloning: it is already pushed.
	r, _ := e.open(rs.mine)
	tgt, err := r.AmendTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tgt.CanAmend || tgt.Reason != CodeAmendPushed {
		t.Fatalf("pushed commit judged amendable: %+v", tgt)
	}
	// Amend itself rechecks and refuses with the same reason.
	snap, _ := r.Status(context.Background())
	_, _, err = r.Amend(context.Background(), AmendRequest{
		Message: "Reword", Head: tgt.Hash, Staged: snap.StagedFingerprint, MessageOnly: true,
	})
	wantCode(t, err, CodeAmendPushed)
}

func TestAmendTargetAllowsUnpushedAboveUpstream(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	// A local commit that the upstream does not contain yet.
	write(t, filepath.Join(rs.mine, "local.txt"), "local\n")
	e.commit(rs.mine, "Local only")
	r, _ := e.open(rs.mine)
	tgt, err := r.AmendTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !tgt.CanAmend || tgt.Subject != "Local only" {
		t.Fatalf("unpushed commit refused: %+v", tgt)
	}
}

func TestAmendTargetUnknownWithoutUpstream(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	// A branch with a remote but no upstream cannot be judged.
	e.git(rs.mine, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(rs.mine, "f.txt"), "f\n")
	e.commit(rs.mine, "Feature")
	r, _ := e.open(rs.mine)
	tgt, err := r.AmendTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tgt.CanAmend || tgt.Reason != CodeAmendUnknown {
		t.Fatalf("unknown upstream judged amendable: %+v", tgt)
	}
}

func TestAmendTargetAllowsWithoutRemote(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, _, tgt := amendReady(t, e, dir)
	_ = r
	if !tgt.CanAmend || tgt.Reason != "" {
		t.Fatalf("no-remote repo refused: %+v", tgt)
	}
}

func TestAmendRefusesMovedHead(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, snap, tgt := amendReady(t, e, dir)
	// HEAD moves behind GitOwl's back.
	write(t, filepath.Join(dir, "c.txt"), "c\n")
	e.commit(dir, "Sneaky")
	moved := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	_, _, err := r.Amend(context.Background(), AmendRequest{
		Message: "Reword", Head: tgt.Hash, Staged: snap.StagedFingerprint, MessageOnly: true,
	})
	wantCode(t, err, CodeStateChanged)
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD")); got != moved {
		t.Fatalf("HEAD changed by refused amend: %s", got)
	}
}

func TestAmendRefusesChangedStaged(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, snap, tgt := amendReady(t, e, dir)
	head := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	// The staged basis changes outside GitOwl.
	write(t, filepath.Join(dir, "d.txt"), "d\n")
	e.git(dir, "add", "d.txt")
	_, _, err := r.Amend(context.Background(), AmendRequest{
		Message: "First plus more", Head: tgt.Hash, Staged: snap.StagedFingerprint, MessageOnly: false,
	})
	wantCode(t, err, CodeStateChanged)
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD")); got != head {
		t.Fatalf("HEAD changed by refused amend: %s", got)
	}
}

func TestAmendTargetDetachedAndUnborn(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	r, _ := e.open(dir)
	// Unborn: no commit yet.
	tgt, err := r.AmendTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tgt.CanAmend || tgt.Reason != CodeAmendUnborn {
		t.Fatalf("unborn judged amendable: %+v", tgt)
	}
	// Detached HEAD after two commits.
	e.commit(dir, "One")
	write(t, filepath.Join(dir, "b.txt"), "b\n")
	e.commit(dir, "Two")
	e.git(dir, "checkout", "-q", "--detach", "HEAD^")
	tgt, err = r.AmendTarget(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tgt.CanAmend || tgt.Reason != CodeAmendDetached {
		t.Fatalf("detached judged amendable: %+v", tgt)
	}
}

func TestAmendRefusesEmptyMessage(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "base.txt"), "base\n")
	e.commit(dir, "Base")
	r, snap, tgt := amendReady(t, e, dir)
	_, _, err := r.Amend(context.Background(), AmendRequest{
		Message: "  \n\t\n", Head: tgt.Hash, Staged: snap.StagedFingerprint, MessageOnly: true,
	})
	wantCode(t, err, CodeEmptyMessage)
}
