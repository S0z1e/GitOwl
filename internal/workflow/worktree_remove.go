package workflow

import (
	"context"
	"path/filepath"
)

const (
	CodeWorktreeNotFound Code = "WorktreeNotFound"
	CodeRemoveMain       Code = "RemoveMain"
	CodeRemoveCurrent    Code = "RemoveCurrent"
	CodeWorktreeLocked   Code = "WorktreeLocked"
	CodeWorktreeDirty    Code = "WorktreeDirty"
	CodeWorktreeMissing  Code = "WorktreeMissing"
)

// RemoveWorktree removes the linked worktree at path: its directory and
// Git's record of it, as git worktree remove does without force. Its
// branch stays. It refuses the main worktree, the one open in GitOwl, a
// locked one, a missing one (prune its record instead) and one with
// staged, unstaged or untracked changes, checking them again now rather
// than trusting an earlier list. It returns the worktrees read afterwards,
// also after a failure.
func (r *Repository) RemoveWorktree(ctx context.Context, path string) ([]Worktree, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return nil, &Error{Code: CodeRepositoryClosed}
	}
	opErr := r.removeWorktree(ctx, canonical(path))
	list, err := r.Worktrees(ctx)
	if err != nil && opErr == nil {
		opErr = err
	}
	return list, opErr
}

func (r *Repository) removeWorktree(ctx context.Context, path string) error {
	list, err := r.Worktrees(ctx)
	if err != nil {
		return err
	}
	var w *Worktree
	for i := range list {
		if list[i].Path == path {
			w = &list[i]
		}
	}
	switch {
	case w == nil:
		return &Error{Code: CodeWorktreeNotFound, Detail: path}
	case w.Main:
		return &Error{Code: CodeRemoveMain, Detail: path}
	case w.Current:
		return &Error{Code: CodeRemoveCurrent, Detail: path}
	case w.Locked:
		return &Error{Code: CodeWorktreeLocked, Detail: orReason(w.LockReason, path)}
	case w.Prunable:
		return &Error{Code: CodeWorktreeMissing, Detail: orReason(w.PrunableReason, path)}
	}
	out, err := r.app.git.Run(ctx, path, nil, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return gitFailure(err)
	}
	if len(out) > 0 {
		return &Error{Code: CodeWorktreeDirty, Detail: path}
	}
	if _, err := r.run(ctx, "worktree", "remove", "--", path); err != nil {
		return err
	}
	return nil
}

func orReason(reason, fallback string) string {
	if reason != "" {
		return reason
	}
	return fallback
}

// PruneCandidates are the worktree records Git would prune now: those
// whose directory is gone and that are not locked.
func (r *Repository) PruneCandidates(ctx context.Context) ([]Worktree, error) {
	list, err := r.Worktrees(ctx)
	if err != nil {
		return nil, err
	}
	var out []Worktree
	for _, w := range list {
		if w.Prunable {
			out = append(out, w)
		}
	}
	return out, nil
}

// Prune removes the records of worktrees Git finds prunable, as git
// worktree prune does: it never removes a directory that exists, unlocks a
// worktree or forces anything. It returns the records it pruned and the
// worktrees read afterwards, also after a failure.
func (r *Repository) Prune(ctx context.Context) (pruned, list []Worktree, err error) {
	defer r.lockRepository()()
	if !r.Current() {
		return nil, nil, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.Worktrees(ctx)
	if err != nil {
		return nil, nil, err
	}
	_, opErr := r.run(ctx, "worktree", "prune")
	list, err = r.Worktrees(ctx)
	if err != nil {
		if opErr == nil {
			opErr = err
		}
		return nil, before, opErr
	}
	left := map[string]bool{}
	for _, w := range list {
		left[filepath.Clean(w.Path)] = true
	}
	for _, w := range before {
		if !left[w.Path] {
			pruned = append(pruned, w)
		}
	}
	return pruned, list, opErr
}
