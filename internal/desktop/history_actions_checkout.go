package desktop

import (
	"context"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// checkoutState is the state of History's Checkout and Create branch…
// actions (issue 15). Both read the selected commit when the user confirms,
// so a plain browse or selection never writes to Git.
type checkoutState struct {
	// The Checkout confirmation: the commit it will detach onto.
	confirmOpen    bool
	confirmHash    string
	confirmSubject string

	// The Create branch dialog: its name, the start commit it shows and
	// the last failure to report inside it.
	createOpen   bool
	newName      string
	switchAfter  bool
	startHash    string
	startSubject string
	createErr    *notice
}

func newCheckoutState() checkoutState { return checkoutState{switchAfter: true} }

// selectedCommit returns the commit the detail shows, or false when none is
// open, so an action confirms against a live selection.
func (u *UI) selectedCommit() (workflow.CommitInfo, bool) {
	h := &u.hist
	if h.detail == nil || h.selected == "" {
		return workflow.CommitInfo{}, false
	}
	return h.detail.CommitInfo, true
}

// checkoutCommit runs CheckoutCommit for the confirmed commit. HEAD at the
// confirmation is passed as the reviewed head, so the workflow refuses a
// stale write when HEAD moved since. On success it closes the dialog; on
// failure branchOp still applies the refreshed state and report explains.
func (u *UI) checkoutCommit() {
	hash, head := u.co.confirmHash, u.snap.Head
	u.branchOp(hash, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.CheckoutCommit(context.Background(), workflow.CheckoutRequest{Hash: hash, Head: head})
	}, func(err error) {
		u.co.confirmOpen = false
		u.report(err)
	})
}

// createBranchAt runs CreateBranchAt for the dialog's name and start. A
// refusal stays in the dialog with its reason; success closes it and, when
// the branch was created but not switched, reports that outside it.
func (u *UI) createBranchAt() {
	name, start, head := u.co.newName, u.co.startHash, u.snap.Head
	sw := u.co.switchAfter
	u.co.createErr = nil
	u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.CreateBranchAt(context.Background(), workflow.BranchAtRequest{
			Name: name, Start: start, Switch: sw, Head: head,
		})
	}, func(err error) {
		if err == nil {
			u.co.createOpen = false
			u.toast("success", u.t("checkoutCreated", "branch", name))
			return
		}
		if workflow.CodeOf(err) == workflow.CodeCheckoutNotSwitched {
			// The branch was created; leave the dialog and explain outside.
			u.co.createOpen = false
			u.report(err)
			return
		}
		u.co.createErr = noticeOf(err)
	})
}

// returnToBranch switches from a detached HEAD back to a local branch,
// through the existing switch (with its worktree and dirty protection).
func (u *UI) returnToBranch(name string) {
	if !u.writing() {
		u.switchBranch(name)
	}
}

// returnToBranchItems lists the local branches a detached HEAD can return
// to, through the existing switch protection.
func (u *UI) returnToBranchItems(m *ui.Menu) {
	locals := u.br.data.Local
	if len(locals) == 0 {
		m.Item(u.t("checkoutReturnNoBranches")).Disabled(true)
		return
	}
	for _, br := range locals {
		name := br.Name
		if m.Item(name).Disabled(u.writing()).Chosen() {
			u.returnToBranch(name)
		}
	}
}
