package desktop

import (
	"context"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// commit_actions.go holds the panel's commit button and its menu (the
// design's .off disabled button and the enabled split button). This slice's
// menu holds only Commit; issues 09/10/11 add Commit and push, amend and AI
// items beside it, all under the same operation protection commitShortcut
// and commit already enforce.

// commitHeight is the design's main-button height (commit-b.md: both the
// disabled .off and the enabled split button are height 40).
const commitHeight = 40

// commitMenu is the split button's dropdown. Plain Commit commits locally;
// Commit and push commits and then pushes the branch, publishing it first
// when it has no upstream.
func (u *UI) commitMenu(m *ui.Menu) {
	if m.Item(u.t("commitAction")).Chosen() {
		u.commit()
	}
	if m.Item(u.t("commitPushAction")).Chosen() {
		u.commitAndPush()
	}
}

// commitPush is a commit-and-push that committed but whose push failed or
// was refused. The local commit stands; Retry push runs again with the same
// request, which RetryPush rechecks against the current HEAD and branch.
type commitPush struct {
	committed workflow.Commit
	req       workflow.CommitPushRequest
	// err is the push failure shown beside the kept commit.
	err *notice
}

// commitAndPush commits the staged changes and then pushes the branch. With
// an upstream it runs at once; without one it opens Publish first and the
// dialog's Publish button starts the commit-and-push, so cancelling it
// builds no commit.
func (u *UI) commitAndPush() {
	if u.commitBlock() != "" {
		return
	}
	info := u.sync.info
	if !u.sync.loaded || info.Branch == "" || info.Branch != u.snap.Branch {
		u.cm.hint = &notice{code: workflow.CodeCommitPushNoTarget}
		return
	}
	if info.Push == nil {
		// No upstream: choose a Publish target first; its Publish button
		// calls runCommitPush with Publish set.
		u.openPublishForCommit()
		return
	}
	u.runCommitPush(*info.Push, false)
}

// runCommitPush commits and pushes to target, publishing the branch when
// publish is set. The commit is made first; a push failure keeps the commit
// and shows it with a Retry push button.
func (u *UI) runCommitPush(target workflow.PushTarget, publish bool) {
	if u.repo == nil || u.cm.committing || u.indexBusy() {
		return
	}
	u.cm.committing, u.cm.hint, u.cm.push = true, nil, nil
	repo := u.repo
	req := workflow.CommitPushRequest{
		Message: u.cm.message, Staged: u.snap.StagedFingerprint,
		Branch: u.snap.Branch, Target: target, Publish: publish,
	}
	u.background(func() func() {
		snap, res, err := repo.CommitAndPush(context.Background(), req)
		return func() { u.appliedCommitPush(repo, req, snap, res, err) }
	})
}

// appliedCommitPush applies a commit-and-push result on the UI thread. A
// result from a repository that is no longer open is dropped, so switching
// session never overwrites another draft. A failed commit keeps the draft
// and shows why; a successful commit clears the draft, and a failed push
// keeps the commit with a Retry push button.
func (u *UI) appliedCommitPush(repo *workflow.Repository, req workflow.CommitPushRequest, snap workflow.Snapshot, res workflow.CommitPushResult, err error) {
	if repo != u.repo {
		return
	}
	u.cm.committing = false
	if err != nil {
		// The commit itself was refused: nothing was pushed, the draft stays.
		u.cm.hint = noticeOf(err)
		if snap.Root != "" {
			u.setSnapshot(snap)
		}
		return
	}
	// The commit succeeded: clear the draft as a plain commit does.
	if u.cm.message == req.Message {
		u.cm.message, u.cm.summary, u.cm.description, u.cm.composed = "", "", "", ""
	}
	u.cm.last = &res.Committed
	u.ai.applied, u.ai.pending, u.ai.last = "", nil, nil
	if res.PushErr != nil {
		// Commit stands, push failed: keep it for Retry push.
		u.cm.push = &commitPush{committed: res.Committed, req: req, err: noticeOf(res.PushErr)}
	}
	u.setSnapshot(snap)
	u.committed()
	u.loadRemotes()
}

// retryPush pushes the commit a failed commit-and-push left local, without
// committing again. RetryPush refuses if HEAD or the branch moved, so a
// stale target is caught before any write.
func (u *UI) retryPush() {
	cp := u.cm.push
	if cp == nil || u.repo == nil || u.cm.committing || u.indexBusy() {
		return
	}
	u.cm.committing, u.cm.push.err = true, nil
	repo, req, committed := u.repo, cp.req, cp.committed
	u.background(func() func() {
		snap, _, err := repo.RetryPush(context.Background(), req, committed)
		return func() {
			if repo != u.repo {
				return
			}
			u.cm.committing = false
			if err != nil {
				if u.cm.push != nil {
					u.cm.push.err = noticeOf(err)
				}
				if snap.Root != "" {
					u.setSnapshot(snap)
				}
				return
			}
			u.cm.push = nil
			u.setSnapshot(snap)
			u.loadRemotes()
		}
	})
}
