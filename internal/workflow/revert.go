package workflow

import (
	"context"
	"os"
	"strings"
)

const (
	// CodeRevertDetached: HEAD is not on a branch, so there is no branch
	// to revert a commit on.
	CodeRevertDetached Code = "RevertDetached"
	// CodeRevertUnborn: the branch has no commits yet.
	CodeRevertUnborn Code = "RevertUnborn"
	// CodeRevertDirty: the worktree has changes, untracked files included,
	// so a revert cannot start cleanly.
	CodeRevertDirty Code = "RevertDirty"
	// CodeRevertInProgress: a merge, rebase, cherry-pick or revert is
	// already under way.
	CodeRevertInProgress Code = "RevertInProgress"
	// CodeRevertNoProgress: no revert is in progress, so there is nothing
	// to abort, continue or mark resolved.
	CodeRevertNoProgress Code = "RevertNoProgress"
	// CodeRevertMerge: the target is a merge commit, which GitOwl does not
	// revert (it would need a parent chosen).
	CodeRevertMerge Code = "RevertMerge"
	// CodeRevertNotFound: the target hash names no commit in this
	// repository.
	CodeRevertNotFound Code = "RevertNotFound"
	// CodeRevertConflicts: the working tree still has unresolved conflict
	// markers, so the revert cannot continue.
	CodeRevertConflicts Code = "RevertConflicts"
)

// RevertStatus is a revert in progress in this worktree, as GitOwl last
// read it, so the interface can show and continue or abort it. Active is
// false when no revert is under way.
type RevertStatus struct {
	// Active reports whether a revert is in progress.
	Active bool
	// Target is the full hash of the commit being reverted, from
	// REVERT_HEAD.
	Target string
	// Conflicted lists the unmerged paths still to resolve.
	Conflicted []string
	// Resolved lists paths that had conflicts and are now staged.
	Resolved []string
}

// AllResolved reports a revert whose conflicts are all resolved, ready to
// continue.
func (s RevertStatus) AllResolved() bool { return s.Active && len(s.Conflicted) == 0 }

// RevertRequest asks to revert one commit on the current branch, or to
// continue a revert in progress. The expected branch and HEAD guard
// against the worktree having moved since the user reviewed it.
type RevertRequest struct {
	// Target is the full hash of the commit to revert. ContinueRevert
	// ignores it, using REVERT_HEAD.
	Target string
	// Head is the commit the user reviewed HEAD at; the operation refuses
	// when HEAD has moved.
	Head string
	// Branch is the branch the user reviewed; the operation refuses when a
	// different branch is checked out.
	Branch string
}

// gitPath resolves name under this worktree's Git directory, following
// --git-path so a linked worktree's own REVERT_HEAD is found, not the main
// worktree's.
func (r *Repository) gitPath(ctx context.Context, name string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--path-format=absolute", "--git-path", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RevertState reports whether a revert is in progress in this worktree and,
// if so, the commit being reverted and the conflicts left. It reads
// REVERT_HEAD through --git-path, so it recognises a revert started before
// GitOwl opened the repository and one in a linked worktree. It changes
// nothing.
func (r *Repository) RevertState(ctx context.Context) (RevertStatus, error) {
	if !r.Current() {
		return RevertStatus{}, &Error{Code: CodeRepositoryClosed}
	}
	return r.revertState(ctx)
}

func (r *Repository) revertState(ctx context.Context) (RevertStatus, error) {
	var s RevertStatus
	head, err := r.gitPath(ctx, "REVERT_HEAD")
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(head)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, fail(CodeNoAccess, err)
	}
	s.Active = true
	s.Target = strings.TrimSpace(string(data))
	snap, err := r.read(ctx)
	if err != nil {
		return s, err
	}
	seen := map[string]bool{}
	for _, ch := range snap.Unstaged {
		if ch.Conflicted && !seen[ch.Path] {
			seen[ch.Path] = true
			s.Conflicted = append(s.Conflicted, ch.Path)
		}
	}
	// A path that was conflicted and is now staged counts as resolved: the
	// revert's own tree differs from HEAD at it.
	for _, ch := range snap.Staged {
		if !seen[ch.Path] {
			s.Resolved = append(s.Resolved, ch.Path)
		}
	}
	return s, nil
}

// Revert reverts req.Target on the current branch. Under the worktree lock
// it rechecks the state and refuses, changing no ref, when HEAD is
// detached or unborn, when HEAD or the branch differ from req, when the
// worktree is not clean (untracked files included), when another operation
// is in progress, when the target names no commit or is a merge commit.
// It runs git revert --no-edit. With no conflict it returns the new state
// and a false RevertStatus.Active; on conflict it returns the in-progress
// RevertStatus without reporting success.
func (r *Repository) Revert(ctx context.Context, req RevertRequest) (Snapshot, RevertStatus, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, RevertStatus{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, RevertStatus{}, err
	}
	if code := r.revertBlock(before, req); code != "" {
		return before, RevertStatus{}, &Error{Code: code}
	}
	if busy, err := r.inProgress(ctx); err != nil || busy {
		if err == nil {
			err = &Error{Code: CodeRevertInProgress}
		}
		return before, RevertStatus{}, err
	}
	if code, err := r.checkTarget(ctx, req.Target); err != nil {
		return before, RevertStatus{}, err
	} else if code != "" {
		return before, RevertStatus{}, &Error{Code: code}
	}
	_, _, gerr := r.app.git.RunStatus(ctx, r.root, nil, []int{1},
		"revert", "--no-edit", req.Target)
	status, serr := r.revertState(ctx)
	if serr != nil {
		return before, RevertStatus{}, serr
	}
	after, aerr := r.read(ctx)
	if aerr != nil {
		return before, status, aerr
	}
	if status.Active {
		// A conflict left the revert in progress: not a success.
		return after, status, nil
	}
	if gerr != nil {
		return after, RevertStatus{}, gitFailure(gerr)
	}
	return after, RevertStatus{}, nil
}

// revertBlock returns why a revert cannot start in snap for req, or "".
func (r *Repository) revertBlock(snap Snapshot, req RevertRequest) Code {
	switch {
	case snap.Detached:
		return CodeRevertDetached
	case snap.Unborn():
		return CodeRevertUnborn
	case snap.Branch != req.Branch || snap.Head != req.Head:
		return CodeStateChanged
	case !snap.Clean():
		return CodeRevertDirty
	}
	return ""
}

// checkTarget returns a Code when target names no commit or is a merge
// commit, or a non-nil error for an unexpected Git failure.
func (r *Repository) checkTarget(ctx context.Context, target string) (Code, error) {
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1, 128},
		"rev-parse", "--verify", "--quiet", target+"^{commit}")
	if err != nil {
		return "", gitFailure(err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return CodeRevertNotFound, nil
	}
	parents, err := r.run(ctx, "rev-list", "--parents", "-n", "1", target)
	if err != nil {
		return "", err
	}
	if n := len(strings.Fields(strings.TrimSpace(string(parents)))); n > 2 {
		// commit + more than one parent: a merge.
		return CodeRevertMerge, nil
	}
	return "", nil
}

// AbortRevert aborts a revert in progress, restoring HEAD, the index and
// the working tree to where they were before it began (git revert
// --abort). It refuses, changing nothing, when no revert is in progress.
// The state read afterwards is returned.
func (r *Repository) AbortRevert(ctx context.Context) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	status, err := r.revertState(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !status.Active {
		before, _ := r.read(ctx)
		return before, &Error{Code: CodeRevertNoProgress}
	}
	if _, err := r.run(ctx, "revert", "--abort"); err != nil {
		after, _ := r.read(ctx)
		return after, err
	}
	return r.read(ctx)
}

// MarkResolved stages path as the user has resolved it: git add when the
// working-tree file is present, git rm when it is gone (a delete
// conflict). It refuses, staging nothing, when no revert is in progress,
// when path is not an unmerged file, or when the file still holds conflict
// markers. The state read afterwards is returned.
func (r *Repository) MarkResolved(ctx context.Context, path string) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	status, err := r.revertState(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !status.Active {
		return snap, &Error{Code: CodeRevertNoProgress}
	}
	conflicted := false
	for _, p := range status.Conflicted {
		if p == path {
			conflicted = true
		}
	}
	if !conflicted {
		return snap, &Error{Code: CodeStateChanged, Detail: path}
	}
	abs, err := r.insidePath(path)
	if err != nil {
		return snap, err
	}
	if _, serr := os.Stat(abs); os.IsNotExist(serr) {
		if _, err := r.run(ctx, "--literal-pathspecs", "rm", "-q", "--", path); err != nil {
			after, _ := r.read(ctx)
			return after, err
		}
		return r.read(ctx)
	}
	if marked, err := hasConflictMarkers(abs); err != nil {
		return snap, err
	} else if marked {
		return snap, &Error{Code: CodeRevertConflicts, Detail: path}
	}
	if _, err := r.run(ctx, "--literal-pathspecs", "add", "--", path); err != nil {
		after, _ := r.read(ctx)
		return after, err
	}
	return r.read(ctx)
}

// hasConflictMarkers reports whether a file still has a Git conflict marker
// at the start of a line.
func hasConflictMarkers(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fail(CodeNoAccess, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "<<<<<<< "),
			strings.HasPrefix(line, ">>>>>>> "),
			line == "=======":
			return true, nil
		}
	}
	return false, nil
}

// ContinueRevert finishes a revert whose conflicts are resolved, producing
// the revert commit (git revert --continue, with an editor that writes the
// default message without prompting). It rechecks that a revert is in
// progress for req.Target on req.Branch at req.Head and that every
// conflict is resolved; otherwise it changes nothing. The state read
// afterwards is returned, with a false RevertStatus.Active on success.
func (r *Repository) ContinueRevert(ctx context.Context, req RevertRequest) (Snapshot, RevertStatus, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, RevertStatus{}, &Error{Code: CodeRepositoryClosed}
	}
	status, err := r.revertState(ctx)
	if err != nil {
		return Snapshot{}, RevertStatus{}, err
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, RevertStatus{}, err
	}
	switch {
	case !status.Active:
		return snap, status, &Error{Code: CodeRevertNoProgress}
	case snap.Branch != req.Branch || snap.Detached:
		return snap, status, &Error{Code: CodeStateChanged, Detail: snap.Branch}
	case snap.Head != req.Head:
		return snap, status, &Error{Code: CodeStateChanged, Detail: snap.Head}
	case status.Target != req.Target:
		return snap, status, &Error{Code: CodeStateChanged, Detail: status.Target}
	case len(status.Conflicted) > 0:
		return snap, status, &Error{Code: CodeRevertConflicts}
	}
	if _, err := r.run(ctx, "-c", "core.editor=true", "revert", "--continue"); err != nil {
		after, _ := r.read(ctx)
		now, _ := r.revertState(ctx)
		return after, now, err
	}
	after, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, RevertStatus{}, err
	}
	return after, RevertStatus{}, nil
}
