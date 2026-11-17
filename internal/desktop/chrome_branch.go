package desktop

import (
	"context"
	"net/url"
	"os/exec"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

func (u *UI) openBranchForm(kind, from string) {
	name := ""
	if kind == "rename" {
		name = from
	}
	u.br.confirm = branchConfirm{}
	u.br.form = branchForm{kind: kind, open: true, from: from, name: name}
}

func (u *UI) openBranchConfirm(kind, name, ours, theirs string) {
	u.br.form = branchForm{}
	u.br.confirm = branchConfirm{kind: kind, open: true, name: name, ours: ours, theirs: theirs}
}

func (u *UI) chromeBranchDialogs(c *ui.Context) {
	u.chromeBranchFormDialog(c)
	u.chromeBranchConfirmDialog(c)
}

func (u *UI) chromeBranchFormDialog(c *ui.Context) {
	f := &u.br.form
	if !f.open {
		return
	}
	title, source, field, primary := u.t("newBranch"), "", u.t("branchName"), u.t("createBranch")
	switch f.kind {
	case "createFrom":
		source = u.t("newBranchFromHereFrom", "branch", f.from)
	case "commitBranch":
		source = u.t("newBranchFromCommit", "hash", shortHash(f.from), "subject", f.label)
	case "tag":
		source, field, primary = u.t("newTagFromBranch", "branch", f.from), u.t("tagName"), u.t("createTag")
	case "commitTag":
		source, field, primary = u.t("newTagFromCommit", "hash", shortHash(f.from), "subject", f.label), u.t("tagName"), u.t("createTag")
	case "rename":
		source, primary = u.t("renameBranchFrom", "branch", f.from), u.t("renameBranch")
	case "stashBranch":
		source = u.t("newBranchFromStashFrom", "name", f.label)
	case "stashRename":
		title, source, field, primary = u.t("stashRename"), u.t("stashRenameFrom"), u.t("stashMessage"), u.t("renameStash")
	}
	u.chromeFormDialog(c, title, func(tk kitTokens) {
		ui.Text(c, title).FontWeight(600).FontSize(15).TextColor(tk.text)
		if source != "" {
			ui.Text(c, source).Wrap().FontSize(13).TextColor(tk.muted)
		}
		ui.Text(c, field).FontSize(12).TextColor(tk.muted)
		box := ui.TextInput(c, &f.name).Height(kitFieldH).FillWidth().Label(field).AutoFocus()
		if box.Changed() {
			f.err = nil
		}
		submit := box.Submitted()
		if f.kind == "stashBranch" {
			on := f.switchAfter
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				cb := kitCheckbox(c, &on, u.t("switchAfterCreate"), tk, false)
				if cb.Clicked() {
					f.switchAfter = !f.switchAfter
				}
				ui.Text(c, u.t("switchAfterCreate")).FontSize(12).TextColor(tk.muted).PassThrough()
			})
		}
		if e := f.err; e != nil {
			ui.Text(c, u.t("error"+string(e.code))).Wrap().TextColor(tk.del)
			if e.detail != "" {
				ui.Text(c, e.detail).Wrap().FontSize(12).TextColor(tk.muted)
			}
		}
		u.chromeFormButtons(c, tk, primary, func() {
			u.br.form = branchForm{}
		}, u.submitBranchForm, submit)
	}, func() {
		u.br.form = branchForm{}
	})
}

func (u *UI) chromeBranchConfirmDialog(c *ui.Context) {
	p := &u.br.confirm
	if !p.open {
		return
	}
	title, body, action := "", "", u.t("deleteAction")
	theirs := p.name
	if theirs == "" {
		theirs = p.theirs
	}
	switch p.kind {
	case "delete":
		title, body = u.t("deleteBranchTitle", "branch", p.name), u.t("deleteBranchBody")
	case "deleteTag":
		title, body = u.t("deleteTagTitle", "tag", p.name), u.t("deleteTagBody")
	case "deleteTagRemote":
		title, body = u.t("deleteTagRemoteTitle", "tag", p.name, "remote", p.theirs), u.t("deleteTagRemoteBody")
	case "dropStash":
		title, body, action = u.t("dropStashTitle", "name", p.name), u.t("dropStashBody"), u.t("stashDrop")
	case "merge":
		title, body, action = u.t("mergeTitle", "theirs", theirs, "ours", p.ours), u.t("mergeBody"), u.t("mergeAction")
	case "rebase":
		title, body, action = u.t("rebaseTitle", "branch", p.ours, "onto", theirs), u.t("rebaseBody"), u.t("rebaseAction")
	case "checkout":
		title, body, action = u.t("checkoutTitle"), u.t("commitCheckoutBody"), u.t("checkoutConfirm")
	case "cherry":
		title, body, action = u.t("commitCherryTitle"), u.t("commitCherryBody"), u.t("commitMenuCherryPick")
	case "revert":
		title, body, action = u.t("revertTitle"), u.t("commitRevertBody"), u.t("revertConfirm")
	case "reset":
		title, body, action = u.t("commitResetTitle"), u.t("commitResetBody"), u.t("commitResetAction")
	}
	u.chromeFormDialog(c, title, func(tk kitTokens) {
		ui.Text(c, title).FontWeight(600).FontSize(15).TextColor(tk.text)
		ui.Text(c, body).Wrap().FontSize(13).TextColor(tk.muted)
		u.chromeFormButtons(c, tk, action, func() {
			u.br.confirm = branchConfirm{}
		}, u.confirmBranchPrompt, false)
	}, func() {
		u.br.confirm = branchConfirm{}
	})
}

func (u *UI) submitBranchForm() {
	f := u.br.form
	name := strings.TrimSpace(f.name)
	if name == "" || u.writing() {
		return
	}
	head, from, kind := u.snap.Head, f.from, f.kind
	switch kind {
	case "createFrom", "commitBranch":
		u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
			return r.CreateBranchAt(context.Background(), workflow.BranchAtRequest{Name: name, Start: from, Head: head})
		}, u.finishBranchForm)
	case "rename":
		u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
			return r.RenameBranch(context.Background(), from, name)
		}, u.finishBranchForm)
	case "tag", "commitTag":
		u.createTagFromForm(name, from)
	case "stashBranch":
		switchTo := f.switchAfter
		u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
			return r.BranchFromStash(context.Background(), name, from, switchTo)
		}, func(err error) {
			u.finishBranchForm(err)
			if err == nil {
				u.leaveStash()
				u.loadNavLists()
			}
		})
	case "stashRename":
		u.renameStashFromForm(from, name)
	}
}

func (u *UI) finishBranchForm(err error) {
	if err != nil {
		u.br.form.err = noticeOf(err)
		return
	}
	u.br.form = branchForm{}
}

func (u *UI) renameStashFromForm(ref, message string) {
	repo := u.repo
	if repo == nil || u.writing() {
		return
	}
	u.stv.busy = true
	u.background(func() func() {
		snap, err := repo.RenameStash(context.Background(), ref, message)
		return func() {
			if repo != u.repo {
				return
			}
			u.stv.busy = false
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.loadNavLists()
			if err == nil && u.stv.open && u.stv.item.Ref == ref {
				u.stv.item.Message = message
			}
			u.finishBranchForm(err)
		}
	})
}

func (u *UI) createTagFromForm(name, target string) {
	repo := u.repo
	if repo == nil || u.writing() {
		return
	}
	u.br.busy = name
	u.background(func() func() {
		snap, err := repo.CreateTag(context.Background(), name, target)
		return func() {
			if repo != u.repo {
				return
			}
			u.br.busy = ""
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			u.loadNavLists()
			u.finishBranchForm(err)
		}
	})
}

func (u *UI) confirmBranchPrompt() {
	p := u.br.confirm
	u.br.confirm = branchConfirm{}
	switch p.kind {
	case "delete":
		u.deleteBranch(p.name)
	case "deleteTag":
		u.deleteTag(p.name)
	case "deleteTagRemote":
		u.deleteRemoteTag(p.name, p.theirs)
	case "dropStash":
		u.dropStash(p.theirs)
	case "merge":
		u.integrateOp(p.theirs, func(r *workflow.Repository) (workflow.Snapshot, error) {
			return r.Merge(context.Background(), p.ours, p.theirs)
		})
	case "rebase":
		u.integrateOp(p.ours, func(r *workflow.Repository) (workflow.Snapshot, error) {
			return r.Rebase(context.Background(), p.ours, p.theirs)
		})
	case "checkout":
		u.chromeCheckoutCommit(p.theirs)
	case "cherry":
		u.integrateOp(p.theirs, func(r *workflow.Repository) (workflow.Snapshot, error) {
			return r.CherryPick(context.Background(), p.theirs)
		})
	case "revert":
		u.chromeRevertCommit(p.theirs)
	case "reset":
		u.integrateOp(p.theirs, func(r *workflow.Repository) (workflow.Snapshot, error) {
			return r.MixedReset(context.Background(), p.theirs)
		})
	}
}

func (u *UI) integrateOp(name string, op func(*workflow.Repository) (workflow.Snapshot, error)) {
	u.leaveCompare()
	u.middleTab = 0
	u.branchOp(name, func(r *workflow.Repository) (workflow.Snapshot, workflow.Branches, error) {
		snap, err := op(r)
		if snap.Root == "" {
			return snap, workflow.Branches{}, err
		}
		b, berr := r.Branches(context.Background())
		if err != nil {
			return snap, b, err
		}
		return snap, b, berr
	}, u.report)
}

func (u *UI) localBranch(name string) (workflow.Branch, bool) {
	for _, br := range u.br.data.Local {
		if br.Name == name {
			return br, true
		}
	}
	return workflow.Branch{}, false
}

func (u *UI) branchPullReason(name string) string {
	if why := u.branchSyncReason(name); why != "" {
		return why
	}
	br, ok := u.localBranch(name)
	if !ok || br.Upstream == "" {
		return u.t("errorPullNoUpstream")
	}
	if br.Behind == 0 {
		return u.t("pullUpToDate", "branch", name, "upstream", br.Upstream)
	}
	return ""
}

func (u *UI) branchPushReason(name string) string {
	if why := u.branchSyncReason(name); why != "" {
		return why
	}
	br, ok := u.localBranch(name)
	if !ok || br.Upstream == "" {
		return u.t("errorPullNoUpstream")
	}
	if br.Ahead == 0 {
		return u.t("pushUpToDate", "target", br.Upstream)
	}
	return ""
}

func (u *UI) branchSyncReason(name string) string {
	switch {
	case u.snap.Detached:
		return u.t("errorPushDetached")
	case u.snap.Unborn():
		return u.t("errorPushUnborn")
	case name != u.snap.Branch:
		return u.t("branchMenuSyncOther")
	}
	return ""
}

func (u *UI) menuPull(name string) {
	if u.branchPullReason(name) != "" || u.writing() {
		return
	}
	remote, branch := u.sync.info.UpstreamRemote, u.sync.info.UpstreamBranch
	if remote == "" || branch == "" {
		return
	}
	u.sync.pop.pullOpen, u.sync.pop.pushOpen, u.sync.pop.kind = false, false, ""
	opts := workflow.PullOptions{Remote: remote, Branch: branch, Strategy: workflow.PullMerge, Autostash: true}
	target := remote + "/" + branch
	var res workflow.PullResult
	u.remoteOp(opPull, target, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		snap, rr, err := r.PullWith(ctx, opts)
		res = rr
		return snap, err
	}, func(err error) {
		u.chromeRemoteDone(err, res.Commits == 0, "pulledToast", "pullUpToDate", "pullCancelled",
			"count", res.Commits, "branch", u.snap.Branch, "upstream", target)
	})
}

func (u *UI) menuPush(name string) {
	if u.branchPushReason(name) != "" || u.writing() {
		return
	}
	remote, branch := u.sync.info.UpstreamRemote, u.sync.info.UpstreamBranch
	if remote == "" || branch == "" {
		return
	}
	u.sync.pop.pullOpen, u.sync.pop.pushOpen, u.sync.pop.kind = false, false, ""
	opts := workflow.PushOptions{Remote: remote, Branch: branch}
	target := remote + "/" + branch
	var res workflow.PushResult
	u.remoteOp(opPush, target, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		snap, rr, err := r.PushWith(ctx, opts)
		res = rr
		return snap, err
	}, func(err error) {
		u.chromeRemoteDone(err, res.UpToDate, "pushedToast", "pushUpToDate", "pushCancelled",
			"branch", u.snap.Branch, "target", target)
	})
}

func (u *UI) pullRequestBase() (remote, branch string) {
	if u.sync.pop.remote != "" && u.sync.pop.branch != "" {
		return u.sync.pop.remote, u.sync.pop.branch
	}
	return u.sync.info.UpstreamRemote, u.sync.info.UpstreamBranch
}

func (u *UI) pullRequestDisabled(name string) (bool, string) {
	if u.writing() {
		return true, u.t("errorGitFailed")
	}
	remote, branch := u.pullRequestBase()
	if remote == "" || branch == "" {
		return true, u.t("errorNoRemote")
	}
	if pullRequestURL(u.remoteURL(remote), branch, name) == "" {
		return true, u.t("pullRequestUnavailable")
	}
	return false, ""
}

func (u *UI) remoteURL(name string) string {
	for _, r := range u.sync.info.Remotes {
		if r.Name == name {
			return r.URL
		}
	}
	return ""
}

func (u *UI) openPullRequest(name string) {
	off, _ := u.pullRequestDisabled(name)
	if off {
		return
	}
	remote, branch := u.pullRequestBase()
	u.openURL(pullRequestURL(u.remoteURL(remote), branch, name))
}

func (u *UI) openURL(raw string) {
	if raw == "" {
		return
	}
	fn := u.opts.OpenURL
	if fn == nil {
		fn = func(s string) error { return exec.Command("/usr/bin/open", s).Start() }
	}
	u.background(func() func() {
		err := fn(raw)
		return func() {
			if err != nil {
				u.report(err)
			}
		}
	})
}

func (u *UI) flushCopy(c *ui.Context) {
	if u.copyPending == "" {
		return
	}
	c.WriteClipboard(u.copyPending)
	u.copyPending = ""
}

func webRepoURL(remote string) string {
	s := strings.TrimSpace(remote)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")
	switch {
	case strings.HasPrefix(s, "git@"):
		rest := strings.TrimPrefix(s, "git@")
		host, path, ok := strings.Cut(rest, ":")
		if !ok || host == "" || path == "" {
			return ""
		}
		return "https://" + host + "/" + strings.TrimPrefix(path, "/")
	case strings.HasPrefix(s, "ssh://"):
		rest := strings.TrimPrefix(s, "ssh://")
		rest = strings.TrimPrefix(rest, "git@")
		if rest == "" {
			return ""
		}
		return "https://" + rest
	case strings.HasPrefix(s, "git://"):
		return "https://" + strings.TrimPrefix(s, "git://")
	case strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://"):
		return s
	default:
		return ""
	}
}

func pullRequestURL(remoteURL, base, head string) string {
	repo := webRepoURL(remoteURL)
	if repo == "" || base == "" || head == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(repo), "gitlab.") {
		return repo + "/-/merge_requests/new?merge_request[source_branch]=" + url.QueryEscape(head) +
			"&merge_request[target_branch]=" + url.QueryEscape(base)
	}
	return repo + "/compare/" + url.PathEscape(base) + "..." + url.PathEscape(head)
}
