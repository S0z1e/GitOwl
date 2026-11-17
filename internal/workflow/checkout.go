package workflow

import (
	"context"
	"strings"
)

// CheckoutRequest asks to check out a commit in a detached HEAD.
type CheckoutRequest struct {
	// Hash is the full commit object name to check out.
	Hash string
	// Head is the commit the user reviewed; CheckoutCommit refuses when
	// HEAD has moved since.
	Head string
}

// CheckoutCommit checks out req.Hash, leaving a detached HEAD. It refuses,
// with no change, when HEAD has moved from req.Head, and keeps the same
// protection for uncommitted work as switching a branch: Git refuses when
// local changes would be overwritten, and they are neither stashed nor
// discarded. A commit that does not exist is reported as a Git failure.
func (r *Repository) CheckoutCommit(ctx context.Context, req CheckoutRequest) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		if snap.Head != req.Head {
			return &Error{Code: CodeStateChanged}
		}
		_, err := r.run(ctx, "switch", "--quiet", "--detach", req.Hash)
		return switchFailure(err)
	})
}

// BranchAtRequest asks to create a branch at a given commit.
type BranchAtRequest struct {
	// Name is the new branch's name.
	Name string
	// Start is the full commit object name the branch starts at.
	Start string
	// Switch asks to check the new branch out once it is created.
	Switch bool
	// Head is the commit the user reviewed; CreateBranchAt refuses when
	// HEAD has moved since.
	Head string
}

// CreateBranchAt creates req.Name at req.Start and, when req.Switch is set,
// checks it out. It refuses, with no change, when HEAD has moved from
// req.Head, when the name is invalid or already a branch, or when the start
// commit does not exist. The branch is created first; when the switch is
// then refused (such as by uncommitted work that would be overwritten), the
// branch stays and CodeCheckoutNotSwitched reports that it was created but
// not switched to, so the interface can say so.
func (r *Repository) CreateBranchAt(ctx context.Context, req BranchAtRequest) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		if snap.Head != req.Head {
			return &Error{Code: CodeStateChanged}
		}
		if err := r.validBranchName(ctx, req.Name, b); err != nil {
			return err
		}
		if _, err := r.run(ctx, "rev-parse", "--verify", "--quiet", req.Start+"^{commit}"); err != nil {
			return fail(CodeCheckoutStartNotFound, err)
		}
		if req.Switch {
			if _, err := r.run(ctx, "branch", "--no-track", req.Name, req.Start); err != nil {
				return err
			}
			if _, err := r.run(ctx, "switch", "--quiet", "--no-guess", req.Name); err != nil {
				if CodeOf(switchFailure(err)) == CodeSwitchBlocked {
					return &Error{Code: CodeCheckoutNotSwitched, Detail: req.Name}
				}
				return switchFailure(err)
			}
			return nil
		}
		_, err := r.run(ctx, "branch", "--no-track", req.Name, req.Start)
		return err
	})
}

// validBranchName applies CreateBranch's name checks: a trimmed, non-empty
// name Git accepts and that is not already a local branch.
func (r *Repository) validBranchName(ctx context.Context, name string, b Branches) error {
	if strings.TrimSpace(name) != name || name == "" {
		return &Error{Code: CodeInvalidBranchName, Detail: name}
	}
	if _, err := r.run(ctx, "check-ref-format", "--branch", name); err != nil {
		return fail(CodeInvalidBranchName, err)
	}
	if _, ok := b.local(name); ok {
		return &Error{Code: CodeBranchExists, Detail: name}
	}
	return nil
}
