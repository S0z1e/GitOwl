package desktop

import (
	"context"

	"gitowl/internal/workflow"
)

// OpenPath opens the repository at path. The current repository stays open
// until it succeeds; only the latest request applies.
func (u *UI) OpenPath(path string) {
	u.openSeq++
	seq := u.openSeq
	u.opening = true
	req := u.wf.RequestOpen()
	u.background(func() func() {
		repo, snap, err := u.wf.OpenRequested(context.Background(), req, path)
		return func() {
			if seq != u.openSeq {
				return
			}
			u.opening = false
			if err != nil {
				if workflow.CodeOf(err) != workflow.CodeRepositoryClosed {
					u.report(err)
				}
				return
			}
			u.switchTo(repo, snap)
		}
	})
}

// switchTo makes repo the one the window shows. The previous worktree's
// words and position are kept for a later return; its work in flight is
// dropped. What was kept for repo applies to its state as just read.
func (u *UI) switchTo(repo *workflow.Repository, snap workflow.Snapshot) {
	sameRepo := u.repo != nil && u.repo.Common() == repo.Common()
	u.saveSession()
	if u.createdWorktree == repo.Root() {
		u.createdWorktree = ""
	}
	if !sameRepo {
		u.resetChromeNav()
	}
	u.repo, u.notice, u.loading = repo, nil, 0
	u.resetRepositoryState()
	u.setSnapshot(snap)
	u.restoreSession(repo.Root())
	u.loadBranches()
	u.loadWorktrees()
	u.loadRemotes()
	u.loadIdentity(false)
	u.loadNavLists()
	u.loadRevertState()
	u.remember(repo.Root())
	u.titleChanged()
}

// resetRepositoryState clears per-repository UI state. Later features add
// their own state here.
func (u *UI) resetRepositoryState() {
	u.resetWorkingTree()
	u.cm = composer{}
	u.resetAI()
	if u.hist.cancel != nil {
		u.hist.cancel()
	}
	u.hist.seq++
	u.hist.detailSeq++
	u.hist.diffSeq++
	hseq, detailSeq, diffSeq := u.hist.seq, u.hist.detailSeq, u.hist.diffSeq
	u.hist = newHistory()
	u.hist.seq, u.hist.detailSeq, u.hist.diffSeq = hseq, detailSeq, diffSeq
	u.leaveCompare()
	u.leaveStash()
	seq := u.br.seq + 1
	u.br = newBranchPanel()
	u.br.seq = seq
	u.trees = worktreePanel{seq: u.trees.seq + 1}
	u.resetSync()
	u.discard = discardState{seq: u.discard.seq + 1}
	u.ident = identityPanel{loadSeq: u.ident.loadSeq + 1}
	u.rev = revertUIState{seq: u.rev.seq + 1}
}

// setSnapshot shows a newly read state of the current repository.
func (u *UI) setSnapshot(snap workflow.Snapshot) {
	if snap.Branch != u.snap.Branch || snap.Detached != u.snap.Detached {
		// The last commit note belongs to the branch it was made on.
		u.cm.last = nil
	}
	u.snap = snap
	u.updateWorkingTree()
	u.followHead()
	u.loadAmendTarget()
}

// Refresh reads the current repository's state again. Results arriving after
// the window switched repositories are dropped.
func (u *UI) Refresh() {
	repo := u.repo
	if repo == nil {
		return
	}
	u.loading++
	u.loadBranches()
	u.loadWorktrees()
	u.loadRemotes()
	u.loadIdentity(false)
	u.loadNavLists()
	u.loadRevertState()
	u.background(func() func() {
		snap, err := repo.Status(context.Background())
		return func() {
			if u.repo != repo {
				return
			}
			u.loading--
			if err != nil {
				u.report(err)
				return
			}
			u.setSnapshot(snap)
		}
	})
}

// Refreshing reports a status read in flight.
func (u *UI) Refreshing() bool { return u.loading > 0 }
