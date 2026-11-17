package workflow

import (
	"context"
)

const (
	// CodeRefNotFound: a named side is not a commit in this repository.
	CodeRefNotFound Code = "RefNotFound"
	// CodeIntegrateDirty: the worktree has changes, so merge or rebase
	// cannot start cleanly.
	CodeIntegrateDirty Code = "IntegrateDirty"
	// CodeIntegrateInProgress: a merge, rebase, cherry-pick or revert is
	// already under way.
	CodeIntegrateInProgress Code = "IntegrateInProgress"
	// CodeIntegrateDetached: HEAD is not on a branch, so there is no
	// branch to merge into or rebase.
	CodeIntegrateDetached Code = "IntegrateDetached"
)

// Merge merges theirs into ours. Both names are the sides the menu wrote.
// A conflict returns the snapshot with Current changes conflict markers
// and no error; there is no Continue or Abort API.
func (r *Repository) Merge(ctx context.Context, ours, theirs string) (Snapshot, error) {
	return r.integrate(ctx, ours, func(ctx context.Context) error {
		if hash, err := r.verifyRef(ctx, theirs); err != nil {
			return err
		} else if hash == "" {
			return &Error{Code: CodeRefNotFound, Detail: theirs}
		}
		_, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1},
			"-c", "merge.autoStash=false", "merge", "--no-autostash", "--no-edit", "--no-stat", "--quiet", "--", theirs)
		return err
	})
}

// Rebase replays branch onto onto. A conflict returns the snapshot with
// Current changes conflict markers and no error; there is no Continue or
// Abort API.
func (r *Repository) Rebase(ctx context.Context, branch, onto string) (Snapshot, error) {
	return r.integrate(ctx, branch, func(ctx context.Context) error {
		if hash, err := r.verifyRef(ctx, onto); err != nil {
			return err
		} else if hash == "" {
			return &Error{Code: CodeRefNotFound, Detail: onto}
		}
		_, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1},
			"rebase", "--quiet", "--", onto)
		return err
	})
}

// integrate switches to branch when needed, then runs op. Conflicts that
// leave unmerged paths are returned as a snapshot, not an error.
func (r *Repository) integrate(ctx context.Context, branch string, op func(context.Context) error) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
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
	if snap.Branch != branch {
		if snap.Detached && branch == "" {
			return snap, &Error{Code: CodeIntegrateDetached}
		}
		b, err := r.Branches(ctx)
		if err != nil {
			return snap, err
		}
		if _, ok := b.local(branch); !ok {
			return snap, &Error{Code: CodeBranchNotFound, Detail: branch}
		}
		if err := r.notInOtherWorktree(ctx, branch); err != nil {
			return snap, err
		}
		if _, err := r.run(ctx, "switch", "--quiet", "--no-guess", branch); err != nil {
			after, _ := r.read(ctx)
			return after, switchFailure(err)
		}
		snap, err = r.read(ctx)
		if err != nil {
			return Snapshot{}, err
		}
	}
	if snap.Detached {
		return snap, &Error{Code: CodeIntegrateDetached}
	}
	if !snap.Clean() {
		return snap, &Error{Code: CodeIntegrateDirty}
	}
	opErr := op(ctx)
	after, aerr := r.read(ctx)
	if aerr != nil {
		if opErr != nil {
			return snap, gitFailure(opErr)
		}
		return snap, aerr
	}
	if conflicted(after) {
		return after, nil
	}
	if opErr != nil {
		return after, gitFailure(opErr)
	}
	return after, nil
}

func conflicted(snap Snapshot) bool {
	for _, ch := range snap.Unstaged {
		if ch.Conflicted {
			return true
		}
	}
	return false
}
