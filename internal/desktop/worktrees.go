package desktop

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// worktreePanel is the Worktrees section of the left column.
type worktreePanel struct {
	list   []workflow.Worktree
	loaded bool
	seq    uint64
	// busy is set while a worktree is created, removed or pruned.
	busy bool

	createOpen bool
	// createMode is 0 for a new branch, 1 for an existing one.
	createMode int
	newBranch  string
	branch     string
	path       string
	// pathEdited keeps a path the user typed or chose from suggestions.
	pathEdited bool
	createErr  *notice

	// removeOpen confirms removing removing; removeErr is why it failed.
	removeOpen bool
	removing   workflow.Worktree
	removeErr  *notice
	// pruneOpen shows the records prune would remove, read into candidates.
	pruneOpen  bool
	candidates []workflow.Worktree
	pruneRead  bool
	pruneErr   *notice

	// selected is the path of the worktree row with the selection.
	selected string
	// pruned is how many records the last prune removed, -1 before one.
	pruned int
}

// mainWorktree returns the main worktree's root, or the open one's when the
// list is not read yet.
func (u *UI) mainWorktree() string {
	for _, w := range u.trees.list {
		if w.Main {
			return w.Path
		}
	}
	return u.repo.Root()
}

// inUse returns the worktree that has branch checked out, or "".
func (u *UI) inUse(branch string) string {
	for _, w := range u.trees.list {
		if w.Branch == branch {
			return w.Path
		}
	}
	return ""
}

// freeBranches are the local branches no worktree has checked out.
func (u *UI) freeBranches() []string {
	var out []string
	for _, b := range u.br.data.Local {
		if u.inUse(b.Name) == "" {
			out = append(out, b.Name)
		}
	}
	return out
}

func (u *UI) createBranchName() string {
	if u.trees.createMode == 0 {
		return u.trees.newBranch
	}
	return u.trees.branch
}

// suggestPath fills the path from the branch, unless the user set it.
func (u *UI) suggestPath() {
	if !u.trees.pathEdited {
		u.trees.path = workflow.SuggestWorktreePath(u.mainWorktree(), u.createBranchName())
	}
}

// openCreateWorktree shows the dialog for a new worktree.
func (u *UI) openCreateWorktree() {
	if u.repo == nil || u.switchBlocked() || u.trees.busy {
		return
	}
	tr := &u.trees
	tr.createOpen, tr.createErr, tr.pathEdited, tr.newBranch = true, nil, false, ""
	if free := u.freeBranches(); len(free) > 0 && !slices.Contains(free, tr.branch) {
		tr.branch = free[0]
	}
	u.suggestPath()
}

// chooseWorktreeParent picks the folder the new worktree goes in, keeping
// the suggested directory name.
func (u *UI) chooseWorktreeParent() {
	if u.opts.ChooseDirectory == nil {
		return
	}
	title, name := u.t("worktreeChooseFolder"), filepath.Base(u.trees.path)
	u.opts.Go(func() {
		dir, err := u.opts.ChooseDirectory(title)
		u.opts.Post(func() {
			if err != nil {
				u.trees.createErr = noticeOf(err)
				return
			}
			if dir != "" {
				u.trees.path, u.trees.pathEdited = filepath.Join(dir, name), true
			}
		})
	})
}

// createWorktree creates the worktree the dialog describes and opens it in
// this window.
func (u *UI) createWorktree() {
	repo, tr := u.repo, &u.trees
	if repo == nil || tr.busy || u.switchBlocked() {
		return
	}
	spec := workflow.NewWorktree{Branch: strings.TrimSpace(u.createBranchName()), Create: tr.createMode == 0, Path: strings.TrimSpace(tr.path)}
	tr.busy, tr.createErr = true, nil
	tr.seq++
	seq := tr.seq
	u.background(func() func() {
		list, branches, root, err := repo.CreateWorktree(context.Background(), spec)
		return func() {
			if repo != u.repo {
				return
			}
			u.trees.busy = false
			if seq == u.trees.seq && list != nil {
				u.trees.list, u.trees.loaded = list, true
			}
			if branches.Local != nil {
				u.br.data, u.br.loaded = branches, true
			}
			if err != nil {
				u.trees.createErr = noticeOf(err)
				return
			}
			u.trees.createOpen = false
			// Shown, with a way to open it, until the new worktree is open.
			u.createdWorktree = root
			u.OpenPath(root)
		}
	})
}

// loadWorktrees reads the repository's worktrees in the background.
func (u *UI) loadWorktrees() {
	repo := u.repo
	if repo == nil {
		return
	}
	u.trees.seq++
	seq := u.trees.seq
	u.background(func() func() {
		list, err := repo.Worktrees(context.Background())
		return func() {
			if repo != u.repo || seq != u.trees.seq {
				return
			}
			if err != nil {
				u.report(err)
				return
			}
			u.trees.list, u.trees.loaded = list, true
		}
	})
}

// openWorktree opens w in this window, keeping the current worktree's words
// and position for a return.
func (u *UI) openWorktree(w workflow.Worktree) {
	if w.Current || u.switchBlocked() {
		return
	}
	u.OpenPath(w.Path)
}

// worktreeState names a worktree's branch, or detached HEAD.
func (u *UI) worktreeState(w workflow.Worktree) string {
	switch {
	case w.Detached && w.Head != "":
		return u.t("worktreeDetached", "hash", shortHash(w.Head))
	case w.Detached:
		return u.t("detachedHead")
	}
	return w.Branch
}

// worktreeTip is a worktree's full path and what protects it.
func (u *UI) worktreeTip(w workflow.Worktree) string {
	s := w.Path
	if w.Main {
		s += "\n" + u.t("worktreeMainTip")
	}
	if w.Locked {
		s += "\n" + u.t("worktreeLockedTip", "reason", orText(w.LockReason, "—"))
	}
	if w.Prunable {
		s += "\n" + u.t("worktreePrunableTip", "reason", orText(w.PrunableReason, "—"))
	}
	return s
}

// removeWorktree removes the worktree being confirmed. Git and GitOwl check
// again that it is safe; the list is read again either way.
func (u *UI) removeWorktree() {
	repo, tr := u.repo, &u.trees
	if repo == nil || tr.busy || u.switchBlocked() {
		return
	}
	path := tr.removing.Path
	tr.busy, tr.removeErr = true, nil
	tr.seq++
	seq := tr.seq
	u.background(func() func() {
		list, err := repo.RemoveWorktree(context.Background(), path)
		return func() {
			if repo != u.repo {
				return
			}
			u.trees.busy = false
			if seq == u.trees.seq && list != nil {
				u.trees.list = list
			}
			if err != nil {
				u.trees.removeErr = noticeOf(err)
				return
			}
			u.trees.removeOpen = false
			delete(u.sessions, path)
			u.loadBranches()
		}
	})
}

// openPrune reads what prune would remove and shows it.
func (u *UI) openPrune() {
	repo, tr := u.repo, &u.trees
	if repo == nil || tr.busy {
		return
	}
	tr.pruneOpen, tr.pruneRead, tr.pruneErr, tr.candidates, tr.pruned = true, false, nil, nil, -1
	tr.seq++
	seq := tr.seq
	u.background(func() func() {
		cands, err := repo.PruneCandidates(context.Background())
		return func() {
			if repo != u.repo || seq != u.trees.seq {
				return
			}
			u.trees.pruneRead, u.trees.candidates, u.trees.pruneErr = true, cands, noticeOf(err)
		}
	})
}

// prune removes the stale records Git finds now, which may differ from
// those shown if something changed meanwhile.
func (u *UI) prune() {
	repo, tr := u.repo, &u.trees
	if repo == nil || tr.busy || u.switchBlocked() {
		return
	}
	tr.busy, tr.pruneErr = true, nil
	tr.seq++
	seq := tr.seq
	u.background(func() func() {
		pruned, list, err := repo.Prune(context.Background())
		return func() {
			if repo != u.repo {
				return
			}
			u.trees.busy = false
			if seq == u.trees.seq && list != nil {
				u.trees.list = list
			}
			u.trees.pruned, u.trees.candidates, u.trees.pruneErr = len(pruned), nil, noticeOf(err)
			for _, w := range pruned {
				delete(u.sessions, w.Path)
			}
		}
	})
}

// worktreeMenu holds the actions on a worktree that is not current.
func (u *UI) worktreeMenu(m *ui.Menu, w workflow.Worktree) {
	if m.Item(u.t("openWorktree", "name", w.Name())).Disabled(w.Prunable || u.switchBlocked()).Chosen() {
		u.openWorktree(w)
	}
	m.Separator()
	if m.Item(u.t("removeWorktree", "name", w.Name())).Disabled(w.Main || w.Locked || w.Prunable || u.switchBlocked()).Chosen() {
		u.trees.removeOpen, u.trees.removing, u.trees.removeErr = true, w, nil
	}
}
