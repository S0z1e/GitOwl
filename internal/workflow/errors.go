package workflow

import (
	"context"
	"errors"

	"gitowl/internal/git"
)

// Code names a failure the interface explains to the user. Codes double as
// message keys, prefixed with "error".
type Code string

const (
	CodeGitMissing       Code = "GitMissing"
	CodePathNotFound     Code = "PathNotFound"
	CodeNotDirectory     Code = "NotDirectory"
	CodeNoAccess         Code = "NoAccess"
	CodeNotRepository    Code = "NotRepository"
	CodeBareRepository   Code = "BareRepository"
	CodeGitFailed        Code = "GitFailed"
	CodeRepositoryClosed Code = "RepositoryClosed"
	CodeStateChanged     Code = "StateChanged"
	CodeCancelled        Code = "Cancelled"

	// CodeAmendDetached: HEAD is not on a branch, so there is no last
	// commit of a branch to amend.
	CodeAmendDetached Code = "AmendDetached"
	// CodeAmendUnborn: the branch has no commits yet.
	CodeAmendUnborn Code = "AmendUnborn"
	// CodeAmendPushed: the upstream (as last fetched) already contains the
	// commit, so amending would rewrite shared history.
	CodeAmendPushed Code = "AmendPushed"
	// CodeAmendUnknown: the branch has a remote but no upstream GitOwl can
	// read, so whether the commit was pushed cannot be judged; fetch or
	// set an upstream first.
	CodeAmendUnknown Code = "AmendUnknown"
	// CodeAmendNotAllowed: the commit may not be amended, as AmendTarget
	// explains; the state was rechecked at the write.
	CodeAmendNotAllowed Code = "AmendNotAllowed"
)

// Error is a failure with a code the interface can explain and Git's or the
// system's own detail.
type Error struct {
	Code   Code
	Detail string
	Err    error
}

func (e *Error) Error() string {
	if e.Detail != "" {
		return string(e.Code) + ": " + e.Detail
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Err }

// CodeOf returns err's code, CodeGitFailed for an unexplained error.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeGitFailed
}

func fail(code Code, err error) *Error {
	e := &Error{Code: code, Err: err}
	var g *git.Error
	switch {
	case errors.As(err, &g):
		e.Detail = g.Message()
	case err != nil:
		e.Detail = err.Error()
	}
	return e
}

// gitFailure classifies a failed Git command.
func gitFailure(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, git.ErrNotFound):
		return fail(CodeGitMissing, err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fail(CodeCancelled, err)
	}
	var e *Error
	if errors.As(err, &e) {
		return err
	}
	return fail(CodeGitFailed, err)
}

const (
	// CodeCommitPushNoTarget: CommitAndPush has no usable push target, so
	// nothing is committed: a branch without an upstream needs a Publish
	// target (remote and remote branch), and one with an upstream must not
	// carry a Publish target.
	CodeCommitPushNoTarget Code = "CommitPushNoTarget"
	// CodeCommitPushStale: RetryPush found the state no longer matches the
	// commit it was asked to push (HEAD moved or the branch changed); the
	// user must review the state and decide again.
	CodeCommitPushStale Code = "CommitPushStale"
)

const (
	// CodeCheckoutStartNotFound: CreateBranchAt's start commit does not
	// exist, so there is nothing to branch from.
	CodeCheckoutStartNotFound Code = "CheckoutStartNotFound"
	// CodeCheckoutNotSwitched: CreateBranchAt created the branch but the
	// switch to it was refused (such as by uncommitted work that would be
	// overwritten); the branch stays and nothing else changed.
	CodeCheckoutNotSwitched Code = "CheckoutNotSwitched"
)
