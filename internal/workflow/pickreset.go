package workflow

import (
	"context"
)

// CherryPick applies hash onto the current branch. A conflict returns the
// snapshot with Current changes conflict markers and no error; there is no
// Continue or Abort API.
func (r *Repository) CherryPick(ctx context.Context, hash string) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if snap.Detached {
		return snap, &Error{Code: CodeIntegrateDetached}
	}
	if snap.Unborn() {
		return snap, &Error{Code: CodeNoCommitsForBranch}
	}
	if !snap.Clean() {
		return snap, &Error{Code: CodeIntegrateDirty}
	}
	if busy, err := r.inProgress(ctx); err != nil || busy {
		if err == nil {
			err = &Error{Code: CodeIntegrateInProgress}
		}
		return snap, err
	}
	if target, err := r.verifyRef(ctx, hash); err != nil {
		return snap, err
	} else if target == "" {
		return snap, &Error{Code: CodeRefNotFound, Detail: hash}
	}
	_, _, err = r.app.git.RunStatus(ctx, r.root, nil, []int{1},
		"cherry-pick", "--no-edit", "--", hash)
	after, aerr := r.read(ctx)
	if aerr != nil {
		if err != nil {
			return snap, gitFailure(err)
		}
		return snap, aerr
	}
	if conflicted(after) {
		return after, nil
	}
	if err != nil {
		return after, gitFailure(err)
	}
	return after, nil
}

// MixedReset moves the current branch and the index to hash and leaves
// the worktree as it is. It does not offer soft or hard reset.
func (r *Repository) MixedReset(ctx context.Context, hash string) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if snap.Detached {
		return snap, &Error{Code: CodeIntegrateDetached}
	}
	if snap.Unborn() {
		return snap, &Error{Code: CodeNoCommitsForBranch}
	}
	if busy, err := r.inProgress(ctx); err != nil || busy {
		if err == nil {
			err = &Error{Code: CodeIntegrateInProgress}
		}
		return snap, err
	}
	if target, err := r.verifyRef(ctx, hash); err != nil {
		return snap, err
	} else if target == "" {
		return snap, &Error{Code: CodeRefNotFound, Detail: hash}
	}
	if _, err := r.run(ctx, "reset", "--mixed", "--quiet", hash); err != nil {
		after, _ := r.read(ctx)
		return after, err
	}
	after, err := r.read(ctx)
	if err != nil {
		return snap, err
	}
	return after, nil
}
