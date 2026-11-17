package desktop

import (
	"context"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/git"
	"gitowl/internal/workflow"
)

// branchPanel is the state of the branch section in the left column.
type branchPanel struct {
	data   workflow.Branches
	loaded bool
	seq    uint64
	// busy names the branch an operation runs on; one runs at a time.
	busy string

	createOpen  bool
	newName     string
	switchAfter bool
	createErr   *notice

	deleteOpen bool
	deleteName string

	// selected is the row of the trees with the selection, by key ("" is
	// the current branch); closed are the folders the user closed.
	selected string
	closed   map[string]bool

	// bro is the branch filter and the branch being browsed (browse.go).
	bro browseState

	form    branchForm
	confirm branchConfirm
}

// branchForm is the chrome New Branch dialog reused for new branch from
// here, new tag, rename, and the commit-menu New branch / New tag.
type branchForm struct {
	kind        string // "createFrom", "commitBranch", "tag", "commitTag", "rename", "stashBranch", "stashRename"
	open        bool
	name, from  string
	label       string
	err         *notice
	switchAfter bool
}

// branchConfirm is the chrome Delete-style prompt for delete, merge,
// rebase, and the commit-menu checkout, cherry-pick, revert, and reset.
type branchConfirm struct {
	kind         string // "delete", "dropStash", "merge", "rebase", "checkout", "cherry", "revert", "reset"
	open         bool
	name         string
	ours, theirs string
}

func newBranchPanel() branchPanel { return branchPanel{switchAfter: true, closed: map[string]bool{}} }

// loadBranches reads the branches in the background.
func (u *UI) loadBranches() {
	repo := u.repo
	if repo == nil {
		return
	}
	u.br.seq++
	seq := u.br.seq
	u.background(func() func() {
		b, err := repo.Branches(context.Background())
		return func() {
			if repo != u.repo || seq != u.br.seq {
				return
			}
			if err != nil {
				u.report(err)
				return
			}
			u.br.data, u.br.loaded = b, true
			// The history list draws the branches read here, so a list read
			// before them is out of date: read it again when they change it,
			// or read it for the first time when it waited for them.
			if u.hist.loaded || u.hist.loading || u.hist.awaitBranches {
				u.applySearch()
			}
		}
	})
}

// branchOp runs a branch operation and applies the repository state and
// branches read after it, which may also follow a failure.
func (u *UI) branchOp(name string, op func(*workflow.Repository) (workflow.Snapshot, workflow.Branches, error), done func(error)) {
	repo := u.repo
	if repo == nil || u.writing() {
		return
	}
	u.br.busy = name
	u.br.seq++
	seq := u.br.seq
	u.background(func() func() {
		snap, b, err := op(repo)
		return func() {
			if repo != u.repo {
				return
			}
			u.br.busy = ""
			if snap.Root != "" {
				if seq == u.br.seq {
					u.br.data, u.br.loaded = b, true
				}
				u.setSnapshot(snap)
			}
			u.scopeRefsChanged()
			done(err)
		}
	})
}

func (u *UI) createBranch() {
	name, switchTo := u.br.newName, u.br.switchAfter
	u.br.createErr = nil
	u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.CreateBranch(context.Background(), name, switchTo)
	}, func(err error) {
		if err != nil {
			u.br.createErr = noticeOf(err)
			return
		}
		u.br.createOpen, u.br.newName = false, ""
	})
}

func (u *UI) switchBranch(name string) {
	if u.revertInProgress() {
		u.report(&workflow.Error{Code: workflow.CodeRevertInProgress})
		return
	}
	u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.SwitchBranch(context.Background(), name)
	}, u.report)
}

// checkoutRemote checks out a remote-tracking branch such as origin/feature
// as a local branch of the same short name.
func (u *UI) checkoutRemote(ref string) {
	if u.revertInProgress() {
		u.report(&workflow.Error{Code: workflow.CodeRevertInProgress})
		return
	}
	remote := remoteOf(ref, u.sync.info.Remotes)
	branch := strings.TrimPrefix(ref, remote+"/")
	u.branchOp(branch, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.CheckoutRemote(context.Background(), remote, branch)
	}, u.report)
}

func (u *UI) deleteBranch(name string) {
	u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		return r.DeleteBranch(context.Background(), name)
	}, u.report)
}

func noticeOf(err error) *notice {
	if err == nil {
		return nil
	}
	n := &notice{code: workflow.CodeOf(err)}
	if e, ok := err.(*workflow.Error); ok {
		n.detail = git.Redact(e.Detail)
	} else {
		n.detail = git.Redact(err.Error())
	}
	return n
}

func shortHash(h string) string { return h[:min(7, len(h))] }

func (u *UI) trackText(br workflow.Branch) string {
	switch {
	case br.Upstream == "":
		return ""
	case br.Gone:
		return u.t("upstreamGone", "upstream", br.Upstream)
	case br.Ahead == 0 && br.Behind == 0:
		return u.t("upstreamEven", "upstream", br.Upstream)
	}
	s := ""
	if br.Ahead > 0 {
		s += "↑" + strconv.Itoa(br.Ahead)
	}
	if br.Behind > 0 {
		if s != "" {
			s += " "
		}
		s += "↓" + strconv.Itoa(br.Behind)
	}
	return s + " " + br.Upstream
}

// branchItems are the actions on a branch that is not current.
func (u *UI) branchItems(m *ui.Menu, name string) {
	if m.Item(u.t("switchToBranch", "branch", name)).Disabled(u.writing()).Chosen() {
		u.switchBranch(name)
	}
	m.Separator()
	if m.Item(u.t("deleteBranch", "branch", name)).Disabled(u.writing()).Chosen() {
		u.br.deleteOpen, u.br.deleteName = true, name
	}
}

// chromeBranchItems is the custom-shell branch context menu. The header
// names the branch; groups follow the design: Switch, Create, Integrate,
// Sync, Manage. Destructive items sit last in their group.
func (u *UI) chromeBranchItems(m *ui.Menu, name string) {
	ours := u.snap.Branch
	if ours == "" {
		ours = "HEAD"
	}
	busy := u.writing()
	current := !u.snap.Detached && name == u.snap.Branch
	m.Item(name).Disabled(true)
	m.Separator()
	if m.Item(u.t("branchMenuCheckout")).Disabled(busy || u.switchBlocked() || current).Chosen() {
		u.switchBranch(name)
	}
	m.Separator()
	if m.Item(u.t("branchMenuNewFromHere")).Disabled(busy).Chosen() {
		u.openBranchForm("createFrom", name)
	}
	if m.Item(u.t("branchMenuNewTag")).Disabled(busy).Chosen() {
		u.openBranchForm("tag", name)
	}
	prOff, _ := u.pullRequestDisabled(name)
	if m.Item(u.t("branchMenuOpenPR")).Disabled(prOff).Chosen() {
		u.openPullRequest(name)
	}
	m.Separator()
	mergeOff := busy || current || u.snap.Detached
	if m.Item(u.t("branchMenuMerge", "theirs", name, "ours", ours)).Disabled(mergeOff).Chosen() {
		u.openBranchConfirm("merge", name, ours, name)
	}
	if m.Item(u.t("branchMenuRebase", "branch", ours, "onto", name)).Disabled(mergeOff).Chosen() {
		u.openBranchConfirm("rebase", name, ours, name)
	}
	if m.Item(u.t("branchMenuCompare")).Chosen() {
		u.startCompare(ours, name)
	}
	m.Separator()
	pullWhy := u.branchPullReason(name)
	pushWhy := u.branchPushReason(name)
	if m.Item(u.t("push")).Disabled(busy || pushWhy != "").Chosen() {
		u.menuPush(name)
	}
	if m.Item(u.t("pull")).Disabled(busy || pullWhy != "").Chosen() {
		u.menuPull(name)
	}
	m.Separator()
	if m.Item(u.t("branchMenuRename")).Disabled(busy).Chosen() {
		u.openBranchForm("rename", name)
	}
	if m.Item(u.t("branchMenuDelete")).Disabled(busy || current).Chosen() {
		u.openBranchConfirm("delete", name, "", "")
	}
	if m.Item(u.t("branchMenuCopyName")).Chosen() {
		u.copyPending = name
	}
}

// chromeRemoteBranchItems is the context menu of a remote-tracking branch
// such as origin/feature. Its items use the branch's own name. Delete,
// Pull and Push are not offered: they act on a local branch.
func (u *UI) chromeRemoteBranchItems(m *ui.Menu, ref string) {
	ours := u.snap.Branch
	if ours == "" {
		ours = "HEAD"
	}
	busy := u.writing()
	head := strings.HasSuffix(ref, "/HEAD")
	m.Item(ref).Disabled(true)
	m.Separator()
	if m.Item(u.t("branchMenuCheckout")).Disabled(busy || u.switchBlocked() || head).Chosen() {
		u.checkoutRemote(ref)
	}
	if m.Item(u.t("branchMenuNewFromHere")).Disabled(busy).Chosen() {
		u.openBranchForm("createFrom", ref)
	}
	m.Separator()
	integrateOff := busy || u.snap.Detached || head
	if m.Item(u.t("branchMenuMerge", "theirs", ref, "ours", ours)).Disabled(integrateOff).Chosen() {
		u.openBranchConfirm("merge", ref, ours, ref)
	}
	if m.Item(u.t("branchMenuRebase", "branch", ours, "onto", ref)).Disabled(integrateOff).Chosen() {
		u.openBranchConfirm("rebase", ref, ours, ref)
	}
	if m.Item(u.t("branchMenuCompare")).Chosen() {
		u.startCompare(ours, ref)
	}
	m.Separator()
	if m.Item(u.t("branchMenuCopyName")).Chosen() {
		u.copyPending = ref
	}
}
