package desktop

import (
	"context"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// chromeCommitItems is the All commits context menu. The header names the
// commit; groups follow the design: Switch, Create, Integrate, Undo,
// Export, Copy. Interactive rebase stays unavailable. Reset is last in Undo.
func (u *UI) chromeCommitItems(m *ui.Menu, cm workflow.CommitInfo) {
	ours := u.snap.Branch
	if ours == "" {
		ours = "HEAD"
	}
	short := cm.Short()
	busy := u.writing()
	m.Item(cm.Subject).Disabled(true)
	m.Separator()
	if m.Item(u.t("commitMenuCheckout")).Disabled(busy || u.switchBlocked()).Chosen() {
		u.openBranchConfirm("checkout", cm.Subject, "", cm.Hash)
	}
	m.Separator()
	if m.Item(u.t("commitMenuNewBranch")).Disabled(busy).Chosen() {
		u.openCommitForm("commitBranch", cm)
	}
	if m.Item(u.t("branchMenuNewTag")).Disabled(busy).Chosen() {
		u.openCommitForm("commitTag", cm)
	}
	m.Separator()
	mergeOff := busy || u.snap.Detached
	if m.Item(u.t("branchMenuMerge", "theirs", short, "ours", ours)).Disabled(mergeOff).Chosen() {
		u.openBranchConfirm("merge", short, ours, cm.Hash)
	}
	if m.Item(u.t("branchMenuRebase", "branch", ours, "onto", short)).Disabled(mergeOff).Chosen() {
		u.openBranchConfirm("rebase", short, ours, cm.Hash)
	}
	m.Item(u.t("commitMenuInteractiveRebase")).Disabled(true)
	if m.Item(u.t("commitMenuCherryPick")).Disabled(busy || u.snap.Detached).Chosen() {
		u.openBranchConfirm("cherry", cm.Subject, ours, cm.Hash)
	}
	m.Separator()
	if m.Item(u.t("commitMenuRevert")).Disabled(busy || u.commitRevertDisabled(cm) != "").Chosen() {
		u.openBranchConfirm("revert", cm.Subject, ours, cm.Hash)
	}
	if m.Item(u.t("commitMenuReset")).Disabled(busy || u.snap.Detached).Chosen() {
		u.openBranchConfirm("reset", cm.Subject, ours, cm.Hash)
	}
	m.Separator()
	if m.Item(u.t("stashSavePatch")).Chosen() {
		u.saveCommitPatch(cm)
	}
	if m.Item(u.t("stashCompareWorking")).Chosen() {
		u.startCompareAs(cm.Hash, "", cm.Subject, u.t("workingDirectory"))
	}
	m.Separator()
	if m.Item(u.t("historyCopyHash")).Chosen() {
		u.copyPending = cm.Hash
	}
	if m.Item(u.t("commitMenuCopyInfo")).Chosen() {
		u.copyPending = commitCopyInfo(cm, u.t("commitWhen", "when", cm.When))
	}
}

func commitCopyInfo(cm workflow.CommitInfo, when string) string {
	return cm.Subject + "\n" + cm.Author + "\n" + cm.Hash + "\n" + when
}

func (u *UI) openCommitForm(kind string, cm workflow.CommitInfo) {
	u.br.confirm = branchConfirm{}
	u.br.form = branchForm{kind: kind, open: true, from: cm.Hash, label: cm.Subject}
}

func (u *UI) commitRevertDisabled(cm workflow.CommitInfo) string {
	switch {
	case u.snap.Detached:
		return "errorRevertDetached"
	case u.snap.Unborn():
		return "errorRevertUnborn"
	case u.revertInProgress():
		return "errorRevertInProgress"
	case !u.snap.Clean():
		return "errorRevertDirty"
	case len(cm.Parents) > 1:
		return "errorRevertMerge"
	}
	return ""
}

func (u *UI) chromeCheckoutCommit(hash string) {
	head := u.snap.Head
	u.branchOp(hash, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.CheckoutCommit(context.Background(), workflow.CheckoutRequest{Hash: hash, Head: head})
	}, func(err error) {
		u.loadWorktrees()
		u.report(err)
	})
}

func (u *UI) chromeRevertCommit(hash string) {
	repo := u.repo
	if repo == nil || u.writing() {
		return
	}
	req := workflow.RevertRequest{Target: hash, Branch: u.snap.Branch, Head: u.snap.Head}
	u.leaveCompare()
	u.leaveStash()
	u.middleTab = 0
	var status workflow.RevertStatus
	u.branchOp(hash, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		snap, st, err := r.Revert(context.Background(), req)
		status = st
		if snap.Root == "" {
			return snap, workflow.Branches{}, err
		}
		b, berr := r.Branches(context.Background())
		if err != nil {
			return snap, b, err
		}
		return snap, b, berr
	}, func(err error) {
		u.rev.status = status
		u.report(err)
	})
}

func (u *UI) saveCommitPatch(cm workflow.CommitInfo) {
	fn := u.opts.ChooseSaveFile
	if fn == nil || u.repo == nil {
		return
	}
	repo := u.repo
	suggested := strings.TrimSpace(cm.Short()) + ".patch"
	u.background(func() func() {
		path, err := fn(u.t("stashSavePatch"), suggested)
		if err != nil || path == "" {
			return func() {
				if err != nil {
					u.report(err)
				}
			}
		}
		d, err := repo.Detail(context.Background(), cm)
		if err != nil {
			return func() { u.report(err) }
		}
		var b strings.Builder
		for _, f := range d.Files {
			diff, derr := repo.CommitFileDiff(context.Background(), d, f)
			if derr != nil {
				return func() { u.report(derr) }
			}
			b.WriteString(diff.Patch)
			if diff.Patch != "" && !strings.HasSuffix(diff.Patch, "\n") {
				b.WriteByte('\n')
			}
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			return func() { u.report(err) }
		}
		return nil
	})
}
