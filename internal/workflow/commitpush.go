package workflow

import "context"

// CommitPushRequest records the staged changes and then pushes the branch
// in one action. The commit fields mirror Commit: Message as the user
// wrote it and Staged, the StagedFingerprint the user reviewed. The push
// fields name the branch to push and its Target: with an existing upstream
// the Target comes from RemoteInfo.Push; without one Publish must be set
// with the remote and remote branch the user confirmed, and the target
// becomes the branch's upstream once the remote has its commits.
type CommitPushRequest struct {
	Message string
	Staged  string
	Branch  string
	Target  PushTarget
	// Publish sets the target as the branch's upstream after a successful
	// push, for a branch that had none. It must be set exactly when the
	// branch has no upstream yet.
	Publish bool
}

// CommitPushResult is what CommitAndPush did. Committed is always the new
// commit when the commit succeeded. Push and PushErr report the push
// separately: a refused or uncertain push leaves Committed in place, local,
// to be retried with RetryPush.
type CommitPushResult struct {
	Committed Commit
	Push      PushResult
	PushErr   error
}

// CommitAndPush commits the staged changes, then pushes the branch. The
// push target must be known before the commit: a branch without an upstream
// needs a Publish target, else the commit is refused and nothing is
// created. The commit is made first; a failed commit returns its error and
// nothing is pushed. Once committed, the push is attempted and its outcome
// is reported in the result's Push and PushErr, not as the function's
// error: the commit stands whatever the push did. The state read after is
// returned.
func (r *Repository) CommitAndPush(ctx context.Context, req CommitPushRequest) (Snapshot, CommitPushResult, error) {
	var res CommitPushResult
	if err := r.checkPushTarget(ctx, req); err != nil {
		snap, rerr := r.read(ctx)
		if rerr != nil {
			return Snapshot{}, res, rerr
		}
		return snap, res, err
	}
	snap, commit, err := r.Commit(ctx, req.Message, req.Staged)
	if err != nil {
		return snap, res, err
	}
	res.Committed = commit
	snap, res.Push, res.PushErr = r.Push(ctx, req.Branch, req.Target, req.Publish)
	return snap, res, nil
}

// checkPushTarget confirms req names a usable push target before any
// commit: the branch must be the current one, and Publish must be set
// exactly when the branch has no upstream on a configured remote. It does
// not reach the remote; Push rechecks the branch and remote at the write.
func (r *Repository) checkPushTarget(ctx context.Context, req CommitPushRequest) error {
	if !r.Current() {
		return &Error{Code: CodeRepositoryClosed}
	}
	info, err := r.Remotes(ctx)
	if err != nil {
		return err
	}
	switch {
	case info.Branch == "":
		return &Error{Code: CodePushDetached}
	case req.Branch != info.Branch:
		return &Error{Code: CodeBranchChanged, Detail: info.Branch}
	case info.Push == nil && !req.Publish:
		return &Error{Code: CodeCommitPushNoTarget, Detail: info.Branch}
	case info.Push != nil && req.Publish:
		return &Error{Code: CodeCommitPushNoTarget, Detail: info.Branch}
	}
	if req.Target.Remote == "" || req.Target.Branch == "" {
		return &Error{Code: CodeCommitPushNoTarget, Detail: req.Target.String()}
	}
	return nil
}

// RetryPush pushes the commit CommitAndPush made but could not push,
// without committing again. It refuses unless the repository's state still
// matches req: HEAD is the commit CommitAndPush recorded, the current
// branch is req.Branch and the target is unchanged. A moved HEAD or a
// changed branch returns CodeCommitPushStale: the user must review the
// state and decide again. The state read after is returned.
func (r *Repository) RetryPush(ctx context.Context, req CommitPushRequest, committed Commit) (Snapshot, PushResult, error) {
	var res PushResult
	if err := r.checkPushTarget(ctx, req); err != nil {
		snap, rerr := r.read(ctx)
		if rerr != nil {
			return Snapshot{}, res, rerr
		}
		return snap, res, err
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, res, err
	}
	if committed.Hash == "" || snap.Head != committed.Hash {
		return snap, res, &Error{Code: CodeCommitPushStale, Detail: snap.Head}
	}
	return r.Push(ctx, req.Branch, req.Target, req.Publish)
}
