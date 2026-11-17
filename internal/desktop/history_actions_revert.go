package desktop

import (
	"context"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// revertUIState is the Revert action's state: its confirmation dialog and
// the revert in progress GitOwl last read in this worktree. status is read
// on open, refresh and worktree switch, so a revert started before GitOwl
// opened or in Terminal is recognised; it is never assumed finished.
type revertUIState struct {
	// open shows the confirmation dialog; target is the commit it reverts.
	open   bool
	target workflow.CommitInfo
	// branch and head are the worktree the user reviewed, carried into the
	// request so the workflow refuses if the worktree moved meanwhile.
	branch, head string
	busy         bool
	err          *notice
	// status is the revert in progress; Active is false when none.
	status workflow.RevertStatus
	// aborting is set while AbortRevert runs.
	aborting bool
	// continuing is set while ContinueRevert runs; resolving is the path
	// MarkResolved is running for, "" when none. barErr explains the last
	// resolve or continue failure in the status strip.
	continuing bool
	resolving  string
	barErr     *notice
	// cursor is the focused conflict file in the strip, kept so the
	// keyboard can move through the list; list is its List state.
	cursor int
	list   ui.ListState
	seq    uint64
}

// reverting reports work the revert strip is doing: a Continue, an Abort or
// a MarkResolved in flight. The strip's buttons are disabled meanwhile.
func (u *UI) reverting() bool {
	return u.rev.continuing || u.rev.aborting || u.rev.resolving != ""
}

// revertInProgress reports a revert left in progress in this worktree. While
// one is in progress Commit, Pull, a branch or worktree switch and another
// Revert are unavailable, so the user cannot start a conflicting operation.
func (u *UI) revertInProgress() bool { return u.rev.status.Active }

// loadRevertState reads whether a revert is in progress and keeps it for the
// interface. It changes nothing in the repository.
func (u *UI) loadRevertState() {
	repo := u.repo
	if repo == nil {
		return
	}
	u.rev.seq++
	seq := u.rev.seq
	u.background(func() func() {
		st, err := repo.RevertState(context.Background())
		return func() {
			if repo != u.repo || seq != u.rev.seq || err != nil {
				return
			}
			u.rev.status = st
		}
	})
}

// revertDisabled returns the message key for why the selected commit cannot
// be reverted, or "" when it can. It mirrors the workflow's own refusals so
// the button never offers a revert the workflow would reject.
func (u *UI) revertDisabled() string {
	d := u.hist.detail
	switch {
	case d == nil:
		return "revertNoCommit"
	case u.snap.Detached:
		return "errorRevertDetached"
	case u.snap.Unborn():
		return "errorRevertUnborn"
	case u.revertInProgress():
		return "errorRevertInProgress"
	case !u.snap.Clean():
		return "errorRevertDirty"
	case d.Base == workflow.BaseFirstParent || len(d.Parents) > 1:
		return "errorRevertMerge"
	}
	return ""
}

// planRevert opens the confirmation for the selected commit, recording the
// branch and HEAD the user reviewed so a later move is caught.
func (u *UI) planRevert() {
	d := u.hist.detail
	if d == nil || u.revertDisabled() != "" {
		return
	}
	u.rev.open, u.rev.err = true, nil
	u.rev.target = d.CommitInfo
	u.rev.branch, u.rev.head = u.snap.Branch, u.snap.Head
}

// confirmRevert runs the confirmed revert. It refreshes Changes, History
// and the sync state whatever the result. A clean revert closes the dialog
// and shows the new commit; a conflict closes the dialog and leaves the
// status strip to show the revert in progress with its conflicted files.
func (u *UI) confirmRevert() {
	repo := u.repo
	if repo == nil || u.rev.busy || u.writing() {
		return
	}
	u.rev.busy, u.rev.err = true, nil
	u.rev.seq++
	seq := u.rev.seq
	req := workflow.RevertRequest{Target: u.rev.target.Hash, Branch: u.rev.branch, Head: u.rev.head}
	u.background(func() func() {
		snap, status, err := repo.Revert(context.Background(), req)
		return func() {
			if repo != u.repo || seq != u.rev.seq {
				return
			}
			u.rev.busy = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.rev.status = status
			u.refreshAfterRevert()
			if err != nil {
				u.rev.err = noticeOf(err)
				return
			}
			u.rev.open = false
			if status.Active {
				u.toast("warning", u.t("revertConflictToast", "hash", u.rev.target.Short()))
			} else {
				u.toast("success", u.t("revertedToast", "hash", u.rev.target.Short()))
			}
		}
	})
}

// abortRevert aborts the revert in progress, restoring HEAD, the index and
// the working tree to before it began. It refreshes the views afterwards.
func (u *UI) abortRevert() {
	repo := u.repo
	if repo == nil || u.reverting() || u.writing() {
		return
	}
	u.rev.aborting = true
	u.rev.seq++
	seq := u.rev.seq
	u.background(func() func() {
		snap, err := repo.AbortRevert(context.Background())
		var st workflow.RevertStatus
		if err == nil {
			st, _ = repo.RevertState(context.Background())
		}
		return func() {
			if repo != u.repo || seq != u.rev.seq {
				return
			}
			u.rev.aborting = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			if err != nil {
				u.report(err)
				u.loadRevertState()
				return
			}
			u.rev.status = st
			u.refreshAfterRevert()
			u.toast("success", u.t("revertAbortedToast"))
		}
	})
}

// markResolved stages path as the user has resolved it (git add, or git rm
// for a delete conflict). A file still holding conflict markers is refused,
// with the reason shown in the strip; nothing is staged then. The revert
// status is re-read afterwards so the progress and Continue update.
func (u *UI) markResolved(path string) {
	repo := u.repo
	if repo == nil || u.reverting() || u.writing() {
		return
	}
	u.rev.resolving, u.rev.barErr = path, nil
	u.rev.seq++
	seq := u.rev.seq
	u.background(func() func() {
		snap, err := repo.MarkResolved(context.Background(), path)
		var st workflow.RevertStatus
		if err == nil {
			st, _ = repo.RevertState(context.Background())
		}
		return func() {
			if repo != u.repo || seq != u.rev.seq {
				return
			}
			u.rev.resolving = ""
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			if err != nil {
				u.rev.barErr = noticeOf(err)
				u.loadRevertState()
				return
			}
			u.rev.status = st
		}
	})
}

// continueRevert finishes the revert once every conflict is resolved,
// producing the reverse commit. It carries the branch and HEAD the user
// reviewed so the workflow refuses if the worktree moved. Success clears the
// strip and refreshes History; a failure keeps the Git state and the user's
// edits, explaining why in the strip.
func (u *UI) continueRevert() {
	repo := u.repo
	if repo == nil || u.reverting() || u.writing() || !u.rev.status.AllResolved() {
		return
	}
	u.rev.continuing, u.rev.barErr = true, nil
	u.rev.seq++
	seq := u.rev.seq
	req := workflow.RevertRequest{Target: u.rev.status.Target, Branch: u.snap.Branch, Head: u.snap.Head}
	u.background(func() func() {
		snap, status, err := repo.ContinueRevert(context.Background(), req)
		return func() {
			if repo != u.repo || seq != u.rev.seq {
				return
			}
			u.rev.continuing = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.rev.status = status
			if err != nil {
				u.rev.barErr = noticeOf(err)
				u.loadRevertState()
				return
			}
			u.refreshAfterRevert()
			u.toast("success", u.t("revertContinuedToast"))
		}
	})
}

// refreshAfterRevert reads History, branches and remotes again after a
// revert or abort moved HEAD.
func (u *UI) refreshAfterRevert() {
	u.historyChanged()
	u.loadBranches()
	u.loadRemotes()
}
