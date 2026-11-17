package workflow

import (
	"context"
	"path/filepath"

	"gitowl/internal/git"
)

// Worktree is one working directory of the open repository.
type Worktree struct {
	// Path is the worktree's root, with symbolic links resolved.
	Path string
	// Head is the checked-out commit, empty before the first commit.
	Head string
	// Branch is empty when HEAD is detached.
	Branch   string
	Detached bool
	// Main is the repository's main worktree, which cannot be removed.
	Main bool
	// Current is the worktree open in GitOwl.
	Current bool
	// Locked and Prunable are as Git records them, with their reasons.
	Locked         bool
	LockReason     string
	Prunable       bool
	PrunableReason string
}

// Name is the directory's base name.
func (w Worktree) Name() string { return filepath.Base(w.Path) }

// Worktrees lists the repository's worktrees as Git reports them now:
// the main one first, then linked ones, however they were created. The
// main worktree of a repository whose Git directory is bare is left out,
// as GitOwl does not open bare repositories.
func (r *Repository) Worktrees(ctx context.Context) ([]Worktree, error) {
	out, err := r.run(ctx, git.WorktreeArgs...)
	if err != nil {
		return nil, err
	}
	var list []Worktree
	for i, w := range git.ParseWorktrees(out) {
		if w.Bare {
			continue
		}
		p := canonical(w.Path)
		list = append(list, Worktree{
			Path: p, Head: w.Head, Branch: w.Branch, Detached: w.Detached,
			Main: i == 0, Current: p == r.root,
			Locked: w.Locked, LockReason: w.LockReason,
			Prunable: w.Prunable, PrunableReason: w.PrunableReason,
		})
	}
	return list, nil
}
