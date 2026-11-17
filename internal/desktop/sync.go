package desktop

import (
	"context"
	"errors"
	"strconv"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// Remote operations, as syncState.op names them.
const (
	opFetch = "fetch"
	opPush  = "push"
	opPull  = "pull"
)

// syncState is what the title bar's remote actions show and run.
type syncState struct {
	info   workflow.RemoteInfo
	loaded bool
	// seq numbers reads of info, so only the latest applies.
	seq uint64
	// op is the remote operation running, "" when none; remote is the
	// remote it talks to and cancel stops it.
	op     string
	remote string
	cancel context.CancelFunc
	// opSeq numbers operations, so one left behind by a switch is dropped.
	opSeq uint64

	// err is the last remote operation's failure, shown in the sync strip
	// until dismissed or the next operation succeeds.
	err *notice

	// The publish dialog: the remote and branch the current branch goes to.
	publishOpen                  bool
	publishRemote, publishBranch string
	// publishForCommit routes the dialog's Publish button to a
	// commit-and-push (U11) instead of a plain Publish push, so a branch
	// with no upstream can commit and push in one action. It is set only
	// while the dialog is opened for that purpose.
	publishForCommit bool

	// pop is the chrome Pull/Push popover.
	pop syncPopover
	// popSeq numbers the popover's background reads, so only the latest applies.
	popSeq uint64
}

// syncPopover is the chrome toolbar's Pull or Push popover and the name
// and Add remote dialogs it opens.
type syncPopover struct {
	kind               string // "pull", "push", or ""
	pullOpen, pushOpen bool
	remote, branch     string
	newBranch          bool // selected name is created when Push runs
	strategy           workflow.PullStrategy
	autostash, prune   bool
	// incoming and outgoing are the commits the run would move: Pull's
	// incoming and Push's outgoing, loaded by loadPopoverCommits.
	incoming, outgoing []workflow.CommitInfo
	// counts holds each remote branch's ahead and behind commits against the
	// local branch of the same name, keyed by branch name.
	counts          map[string]popoverCount
	setUpstream     bool
	pushTags        bool
	forceLease      bool
	nameOpen        bool
	name            string
	branchFilter    string
	addOpen         bool
	addName, addURL string
	formErr         *notice
}

// resetSync forgets the previous repository's remotes and stops its remote
// operation, whose result no longer applies.
func (u *UI) resetSync() {
	if u.sync.cancel != nil {
		u.sync.cancel()
	}
	u.sync = syncState{seq: u.sync.seq + 1, opSeq: u.sync.opSeq + 1}
}

// loadRemotes reads the remotes and the current branch's upstream.
func (u *UI) loadRemotes() {
	repo := u.repo
	if repo == nil {
		return
	}
	u.sync.seq++
	seq := u.sync.seq
	u.background(func() func() {
		info, err := repo.Remotes(context.Background())
		return func() {
			if repo != u.repo || seq != u.sync.seq || err != nil {
				return
			}
			u.sync.info, u.sync.loaded = info, true
		}
	})
}

// toast shows a short confirmation in the next frame.
func (u *UI) toast(kind, message string) {
	u.toasts = append(u.toasts, ui.Toast{Title: message, Type: kind})
}

// showToasts hands the queued confirmations to the window.
func (u *UI) showToasts(c *ui.Context) {
	for _, t := range u.toasts {
		c.AddToast(t)
	}
	u.toasts = nil
}

// remoteOp runs a remote operation in the background. It can be cancelled
// while it runs; afterwards the repository is read again, whatever the
// result, and done reports it.
func (u *UI) remoteOp(op, remote string, run func(context.Context, *workflow.Repository) (workflow.Snapshot, error), done func(error)) {
	repo := u.repo
	if repo == nil || u.switchBlocked() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.sync.op, u.sync.remote, u.sync.cancel, u.sync.err = op, remote, cancel, nil
	u.sync.opSeq++
	seq := u.sync.opSeq
	u.background(func() func() {
		snap, err := run(ctx, repo)
		cancel()
		return func() {
			if repo != u.repo || seq != u.sync.opSeq {
				return
			}
			u.sync.op, u.sync.remote, u.sync.cancel = "", "", nil
			if snap.Root != "" {
				u.setSnapshot(snap)
			}
			// Remote branches, ahead and behind, and the worktrees' branches
			// may all have moved.
			u.loadRemotes()
			u.loadBranches()
			u.loadWorktrees()
			u.remoteRefsChanged()
			done(err)
		}
	})
}

// remoteRefsChanged is called after remote-tracking branches may have
// moved. History that shows one of them reads it again.
func (u *UI) remoteRefsChanged() { u.scopeRefsChanged() }

// cancelRemote stops the running remote operation. What it already changed
// stays; the repository is read again when it stops.
func (u *UI) cancelRemote() {
	if u.sync.cancel != nil {
		u.sync.cancel()
	}
}

// fetchBlock returns the message key explaining why Fetch cannot run now,
// "busy" while another operation runs, or "".
func (u *UI) fetchBlock() string {
	switch {
	case u.repo == nil:
		return "errorRepositoryClosed"
	case u.switchBlocked():
		return "busy"
	}
	return ""
}

// fetchDims reports a standing reason Fetch cannot run: no repository, no
// remote, or sync state not read yet. A busy state is not one, so the
// button does not dim while a Git change runs.
func (u *UI) fetchDims() bool {
	if block := u.fetchBlock(); block != "" && block != "busy" {
		return true
	}
	return !u.sync.loaded || len(u.sync.info.Remotes) == 0
}

// fetch fetches remote. An empty remote is the default one, or every remote
// when there is no default; chromeFetch decides.
func (u *UI) fetch(remote string) {
	if remote == "" {
		u.chromeFetch("")
		return
	}
	if u.fetchBlock() != "" {
		return
	}
	u.remoteOp(opFetch, remote, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		return r.Fetch(ctx, remote)
	}, func(err error) {
		switch {
		case err == nil:
			u.notice = nil
			u.toast("success", u.t("fetchedToast", "remote", remote))
		case workflow.CodeOf(err) == workflow.CodeCancelled:
			u.toast("", u.t("fetchCancelled"))
		default:
			u.sync.err = noticeOf(err)
		}
	})
}

// fetchedText says when GitOwl last fetched the repository in this run.
func (u *UI) fetchedText() string {
	if at := u.sync.info.FetchedAt; !at.IsZero() {
		return u.t("lastFetched", "time", at.Format("15:04"))
	}
	return u.t("notFetched")
}

// fetchTip describes what Fetch does and when it last did.
func (u *UI) fetchTip(remote string) string {
	when := u.fetchedText()
	if remote == "" {
		return u.t("fetchChoose") + " · " + when
	}
	return u.t("fetchFrom", "remote", remote) + " · " + when
}

// fetchMenu lists the remotes to fetch from, with their URLs.
func (u *UI) fetchMenu(m *ui.Menu) {
	for _, r := range u.sync.info.Remotes {
		name := r.Name
		label := name
		if r.URL != "" {
			label += "  " + r.URL
		}
		if m.Item(label).Chosen() {
			u.fetch(name)
		}
	}
}

// pushBlock returns the message key explaining why Push or Publish cannot
// run now, "busy" while another operation runs, or "".
func (u *UI) pushBlock() string {
	switch {
	case u.repo == nil:
		return "errorRepositoryClosed"
	case u.switchBlocked() || !u.sync.loaded:
		return "busy"
	case u.snap.Detached:
		return "errorPushDetached"
	case u.snap.Unborn():
		return "errorPushUnborn"
	case len(u.sync.info.Remotes) == 0:
		return "errorNoRemote"
	}
	return ""
}

// aheadCount is the number of commits the upstream lacks, as last fetched,
// when the push goes there.
func (u *UI) aheadCount() string {
	p, info := u.sync.info.Push, u.sync.info
	if p == nil || p.Remote != info.UpstreamRemote || p.Branch != info.UpstreamBranch || !u.snap.HasAheadBehind || u.snap.Ahead == 0 {
		return ""
	}
	return "↑" + strconv.Itoa(u.snap.Ahead)
}

// openPublish asks where to publish the current branch, suggesting a
// branch of the same name on the preferred remote.
func (u *UI) openPublish() {
	if u.pushBlock() != "" {
		return
	}
	s := &u.sync
	s.publishOpen, s.publishForCommit = true, false
	s.publishRemote, s.publishBranch = s.info.PublishRemote(), u.snap.Branch
}

// openPublishForCommit opens the same Publish dialog, but its Publish
// button commits and pushes (U11): a branch with no upstream picks its
// target before any commit, and cancelling builds no commit.
func (u *UI) openPublishForCommit() {
	if u.pushBlock() != "" {
		u.cm.hint = &notice{code: workflow.CodeCommitPushNoTarget}
		return
	}
	s := &u.sync
	s.publishOpen, s.publishForCommit = true, true
	s.publishRemote, s.publishBranch = s.info.PublishRemote(), u.snap.Branch
}

// push pushes the current branch to target, making it the upstream when
// publishing.
func (u *UI) push(target workflow.PushTarget, publish bool) {
	if u.pushBlock() != "" {
		return
	}
	branch := u.snap.Branch
	u.remoteOp(opPush, target.String(), func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		snap, res, err := r.Push(ctx, branch, target, publish)
		if err == nil && res.UpToDate && !publish {
			err = errUpToDate
		}
		return snap, err
	}, func(err error) {
		switch {
		case err == errUpToDate:
			u.notice = nil
			u.toast("success", u.t("pushUpToDate", "target", target.String()))
		case err == nil && publish:
			u.notice = nil
			u.toast("success", u.t("publishedToast", "branch", branch, "target", target.String()))
		case err == nil:
			u.notice = nil
			u.toast("success", u.t("pushedToast", "branch", branch, "target", target.String()))
		default:
			u.sync.err = noticeOf(err)
		}
	})
}

// errUpToDate tells a push that changed nothing on the remote apart from
// one that did.
var errUpToDate = errors.New("up to date")

// diverged reports whether the current branch and its upstream, as last
// fetched, both have commits the other lacks. A fast-forward Pull cannot
// complete then, so the title bar must not present it as a completable
// action (U19 item 3); the divergence is known before Pull is attempted.
func (u *UI) diverged() bool {
	return u.snap.HasAheadBehind && u.snap.Ahead > 0 && u.snap.Behind > 0
}

// pullBlock returns the message key explaining why Pull cannot run now,
// "busy" while another operation runs, or "".
func (u *UI) pullBlock() string {
	switch {
	case u.repo == nil:
		return "errorRepositoryClosed"
	case u.switchBlocked() || !u.sync.loaded:
		return "busy"
	case u.revertInProgress():
		return "errorRevertInProgress"
	case len(u.sync.info.Remotes) == 0:
		return "errorNoRemote"
	}
	// A clean worktree discovers divergence only when Pull fetches; but
	// when the last fetch already shows both ahead and behind, say so now
	// rather than let Pull look completable.
	if workflow.PullBlock(u.snap, u.sync.info) == "" && u.diverged() {
		return "error" + string(workflow.CodePullDiverged)
	}
	if code := workflow.PullBlock(u.snap, u.sync.info); code != "" {
		return "error" + string(code)
	}
	return ""
}

// pull fast-forwards the current branch to its upstream.
func (u *UI) pull() {
	if u.pullBlock() != "" {
		return
	}
	branch, upstream := u.snap.Branch, u.snap.Upstream
	var res workflow.PullResult
	u.remoteOp(opPull, upstream, func(ctx context.Context, r *workflow.Repository) (workflow.Snapshot, error) {
		snap, rr, err := r.Pull(ctx, branch)
		res = rr
		return snap, err
	}, func(err error) {
		switch {
		case err == nil && res.Commits == 0:
			u.notice = nil
			u.toast("success", u.t("pullUpToDate", "branch", branch, "upstream", upstream))
		case err == nil:
			u.notice = nil
			u.toast("success", u.t("pulledToast", "count", res.Commits, "branch", branch, "upstream", upstream))
		case workflow.CodeOf(err) == workflow.CodeCancelled:
			u.toast("", u.t("pullCancelled"))
		default:
			u.sync.err = noticeOf(err)
		}
	})
}
