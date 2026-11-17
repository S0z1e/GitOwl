package workflow

import (
	"context"
	"strings"
)

// Amendable describes whether the current branch's HEAD can have its last
// commit amended, and names what to show the user about it.
type Amendable struct {
	// Hash, Subject and Message are the current HEAD commit, when there is
	// one on a named branch.
	Hash, Subject, Message string
	// CanAmend reports whether Amend is allowed. When false, Reason is a
	// Code explaining why, so the interface can phrase it.
	CanAmend bool
	Reason   Code
}

// AmendRequest asks to rewrite the last commit.
type AmendRequest struct {
	// Message is the new commit message, as the user wrote it.
	Message string
	// Head is the commit the user reviewed; Amend refuses when HEAD has
	// moved since.
	Head string
	// Staged is the StagedFingerprint the user reviewed; Amend refuses
	// when the staged content differs.
	Staged string
	// MessageOnly rewords the commit without taking in any staged change.
	MessageOnly bool
}

// AmendTarget reports whether the last commit of the current named branch
// can be amended. A detached or unborn HEAD cannot. A commit the upstream
// (as last fetched) already contains is treated as pushed and refused. A
// branch with a remote but no usable upstream cannot be judged, so Amend is
// refused until the user fetches or sets an upstream. Without any remote,
// amending is allowed.
func (r *Repository) AmendTarget(ctx context.Context) (Amendable, error) {
	if !r.Current() {
		return Amendable{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Amendable{}, err
	}
	a := Amendable{Hash: snap.Head}
	switch {
	case snap.Detached:
		a.Reason = CodeAmendDetached
		return a, nil
	case snap.Unborn():
		a.Reason = CodeAmendUnborn
		return a, nil
	}
	msg, err := r.run(ctx, "log", "-1", "--format=%B", snap.Head)
	if err != nil {
		return Amendable{}, err
	}
	a.Message = strings.TrimRight(string(msg), "\n") + "\n"
	a.Subject, _, _ = strings.Cut(strings.TrimSpace(string(msg)), "\n")
	a.CanAmend, a.Reason = r.amendAllowed(ctx, snap)
	return a, nil
}

// amendAllowed judges a non-detached, born HEAD against the remotes: it is
// refused once the upstream contains it, and refused as unknown when a
// remote exists without a usable upstream. Without remotes it is allowed.
func (r *Repository) amendAllowed(ctx context.Context, snap Snapshot) (bool, Code) {
	info, err := r.Remotes(ctx)
	if err != nil || len(info.Remotes) == 0 {
		return true, ""
	}
	if info.UpstreamRemote == "" || info.UpstreamBranch == "" {
		return false, CodeAmendUnknown
	}
	upstream := info.UpstreamRemote + "/" + info.UpstreamBranch
	if _, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "refs/remotes/"+upstream); err != nil {
		return false, CodeAmendUnknown
	}
	// The upstream ref exists: when HEAD is an ancestor of it, the commit
	// has been pushed and must not be amended. --is-ancestor exits 0 for
	// an ancestor and 1 otherwise; any other exit is a real error.
	_, code, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1},
		"merge-base", "--is-ancestor", snap.Head, "refs/remotes/"+upstream)
	switch {
	case err != nil:
		return false, CodeAmendUnknown
	case code == 0:
		return false, CodeAmendPushed
	}
	return true, ""
}

// Amend rewrites the last commit of the current named branch with req's
// message. It re-reads the state under the worktree lock and refuses, with
// no change to any ref, when HEAD has moved from req.Head, when the staged
// content differs from req.Staged, or when the commit may no longer be
// amended. A message-only amend keeps the existing tree and needs no
// staged change; otherwise the staged changes are taken into the commit.
// It never fetches or pushes. The state read afterwards is returned.
func (r *Repository) Amend(ctx context.Context, req AmendRequest) (Snapshot, Commit, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, Commit{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Commit{}, err
	}
	switch {
	case strings.TrimSpace(req.Message) == "":
		return before, Commit{}, &Error{Code: CodeEmptyMessage}
	case before.Head != req.Head:
		return before, Commit{}, &Error{Code: CodeStateChanged}
	case before.StagedFingerprint != req.Staged:
		return before, Commit{}, &Error{Code: CodeStateChanged}
	case req.MessageOnly && len(before.Staged) == 0:
		// Allowed: a reword needs no staged change.
	case !req.MessageOnly && len(before.Staged) == 0:
		return before, Commit{}, &Error{Code: CodeNothingStaged}
	}
	if ok, reason := r.amendAllowed(ctx, before); !ok {
		return before, Commit{}, &Error{Code: reason}
	}
	args := []string{"-c", "user.useConfigOnly=true", "commit", "--amend",
		"--quiet", "--file=-", "--cleanup=whitespace"}
	if req.MessageOnly {
		args = append(args, "--only")
	}
	if _, err := r.app.git.Run(ctx, r.root, strings.NewReader(req.Message), args...); err != nil {
		after, rerr := r.read(ctx)
		if rerr != nil {
			after = before
		}
		return after, Commit{}, commitFailure(err)
	}
	after, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Commit{}, err
	}
	subject, _, _ := strings.Cut(strings.TrimSpace(req.Message), "\n")
	return after, Commit{Hash: after.Head, Subject: strings.TrimSpace(subject)}, nil
}
