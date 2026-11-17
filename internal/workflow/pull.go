package workflow

import (
	"context"
	"os"
	"strings"
)

const (
	// CodePullNoUpstream: the branch has no upstream to pull from.
	CodePullNoUpstream Code = "PullNoUpstream"
	// CodePullDirty: the worktree has changes, untracked files included.
	CodePullDirty Code = "PullDirty"
	// CodePullConflicts: the worktree has unresolved conflicts.
	CodePullConflicts Code = "PullConflicts"
	// CodePullInProgress: a merge, rebase, cherry-pick or revert is in
	// progress.
	CodePullInProgress Code = "PullInProgress"
	// CodePullDiverged: the branch and its upstream both have commits the
	// other lacks; a fast-forward cannot join them.
	CodePullDiverged Code = "PullDiverged"
)

// PullResult is what a pull did.
type PullResult struct {
	// Upstream is the branch pulled from, as origin/main.
	Upstream string
	// Commits is how many commits the branch advanced by; zero when it was
	// up to date.
	Commits int
}

// PullStrategy is how PullWith joins fetched commits with the current
// branch. Only these three values are valid.
type PullStrategy int

const (
	// PullMerge records a merge commit when the branches have diverged.
	PullMerge PullStrategy = iota
	// PullRebase replays the current branch onto the fetched commits.
	PullRebase
	// PullFastForwardOnly moves the branch only along existing history.
	PullFastForwardOnly
)

// PullOptions is the remote, branch and strategy a PullWith runs.
// Autostash defaults to on in the interface; callers must set it.
type PullOptions struct {
	Remote, Branch string
	Strategy       PullStrategy
	Autostash      bool // default true in the pull popover
	Prune          bool // default false
}

// PullBlock returns why the current branch cannot be pulled in snap, or
// "" when it can as far as the snapshot shows. Pull checks again, with
// the repository's in-progress operations, before it changes anything.
func PullBlock(snap Snapshot, info RemoteInfo) Code {
	switch {
	case snap.Detached:
		return CodePushDetached
	case snap.Unborn():
		return CodePushUnborn
	case info.UpstreamRemote == "" || snap.Upstream == "":
		return CodePullNoUpstream
	}
	for _, ch := range snap.Unstaged {
		if ch.Conflicted {
			return CodePullConflicts
		}
	}
	if !snap.Clean() {
		return CodePullDirty
	}
	return ""
}

// inProgress reports a merge, rebase, cherry-pick or revert under way in
// this worktree.
func (r *Repository) inProgress(ctx context.Context) (bool, error) {
	for _, p := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		out, err := r.run(ctx, "rev-parse", "--path-format=absolute", "--git-path", p)
		if err != nil {
			return false, err
		}
		if _, err := os.Lstat(strings.TrimSpace(string(out))); err == nil {
			return true, nil
		}
	}
	return false, nil
}

// Pull fetches the current branch's upstream remote and moves the branch
// to the upstream only when that is a fast-forward: never a merge commit,
// a rebase or a stash, whatever pull.rebase or pull.ff say. The worktree
// must be clean, untracked files included. When the branches have
// diverged it reports CodePullDiverged; the fetch it made stays. The state
// read afterwards is returned whatever the result.
func (r *Repository) Pull(ctx context.Context, branch string) (Snapshot, PullResult, error) {
	var res PullResult
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, res, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, res, err
	}
	if snap.Branch != branch && !snap.Detached {
		return snap, res, &Error{Code: CodeBranchChanged, Detail: snap.Branch}
	}
	info, err := r.Remotes(ctx)
	if err == nil && ctx.Err() != nil {
		// Configuration reads stopped by the cancel look like unset values.
		err = fail(CodeCancelled, ctx.Err())
	}
	if err != nil {
		return snap, res, err
	}
	if code := PullBlock(snap, info); code != "" {
		return snap, res, &Error{Code: code}
	}
	if busy, err := r.inProgress(ctx); err != nil || busy {
		if err == nil {
			err = &Error{Code: CodePullInProgress}
		}
		return snap, res, err
	}
	res.Upstream = snap.Upstream
	head := snap.Head
	if _, err := r.network(ctx, "fetch", "--no-recurse-submodules", "--no-write-fetch-head", "--", info.UpstreamRemote); err != nil {
		after, _ := r.afterNetwork()
		return after, res, err
	}
	r.app.markFetched(r.common)
	// Read again: something outside GitOwl may have changed the worktree
	// while the fetch ran.
	snap, err = r.afterNetwork()
	if err != nil {
		return snap, res, err
	}
	if snap.Head != head || snap.Branch != branch {
		return snap, res, &Error{Code: CodeStateChanged}
	}
	if code := PullBlock(snap, info); code != "" {
		return snap, res, &Error{Code: code}
	}
	upstream := "refs/remotes/" + info.UpstreamRemote + "/" + info.UpstreamBranch
	target, err := r.run(ctx, "rev-parse", "--verify", "--quiet", upstream+"^{commit}")
	if err != nil {
		return snap, res, &Error{Code: CodePullNoUpstream, Err: err}
	}
	to := strings.TrimSpace(string(target))
	if to == head {
		return snap, res, nil
	}
	if !r.isAncestor(ctx, to, head) {
		if !r.isAncestor(ctx, head, to) {
			return snap, res, &Error{Code: CodePullDiverged}
		}
		count, err := r.run(ctx, "rev-list", "--count", head+".."+to)
		if err != nil {
			return snap, res, err
		}
		res.Commits = atoi(strings.TrimSpace(string(count)))
		if _, err := r.run(ctx, "-c", "merge.autoStash=false", "merge", "--ff-only", "--no-autostash", "--no-edit", "--quiet", to); err != nil {
			after, _ := r.afterNetwork()
			res.Commits = 0
			return after, res, err
		}
	}
	after, err := r.afterNetwork()
	return after, res, err
}

// PullWith fetches opts.Remote and updates the current branch from
// opts.Remote/opts.Branch using opts.Strategy. Unlike Pull, the remote
// and branch are chosen by the caller, Autostash may keep a dirty
// worktree, and Prune may drop stale remote-tracking branches. The state
// read afterwards is returned whatever the result.
func (r *Repository) PullWith(ctx context.Context, opts PullOptions) (Snapshot, PullResult, error) {
	var res PullResult
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, res, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, res, err
	}
	if _, err := r.checkRemote(ctx, opts.Remote); err != nil {
		return snap, res, err
	}
	name := opts.Branch
	if strings.TrimSpace(name) != name || name == "" || strings.HasPrefix(name, "-") {
		return snap, res, &Error{Code: CodeInvalidBranchName, Detail: name}
	}
	if _, err := r.run(ctx, "check-ref-format", "--branch", name); err != nil {
		return snap, res, fail(CodeInvalidBranchName, err)
	}
	switch opts.Strategy {
	case PullMerge, PullRebase, PullFastForwardOnly:
	default:
		return snap, res, &Error{Code: CodeGitFailed, Detail: "unknown pull strategy"}
	}
	if code := pullWithBlock(snap, opts); code != "" {
		return snap, res, &Error{Code: code}
	}
	if busy, err := r.inProgress(ctx); err != nil || busy {
		if err == nil {
			err = &Error{Code: CodePullInProgress}
		}
		return snap, res, err
	}
	res.Upstream = opts.Remote + "/" + opts.Branch
	head := snap.Head
	branch := snap.Branch
	fetch := []string{"fetch", "--no-recurse-submodules", "--no-write-fetch-head"}
	if opts.Prune {
		fetch = append(fetch, "--prune")
	}
	fetch = append(fetch, "--", opts.Remote)
	if _, err := r.network(ctx, fetch...); err != nil {
		after, _ := r.afterNetwork()
		return after, res, err
	}
	r.app.markFetched(r.common)
	snap, err = r.afterNetwork()
	if err != nil {
		return snap, res, err
	}
	if snap.Head != head || snap.Branch != branch {
		return snap, res, &Error{Code: CodeStateChanged}
	}
	if code := pullWithBlock(snap, opts); code != "" {
		return snap, res, &Error{Code: code}
	}
	upstream := "refs/remotes/" + opts.Remote + "/" + opts.Branch
	target, err := r.run(ctx, "rev-parse", "--verify", "--quiet", upstream+"^{commit}")
	if err != nil {
		return snap, res, &Error{Code: CodeBranchNotFound, Detail: res.Upstream, Err: err}
	}
	to := strings.TrimSpace(string(target))
	if r.isAncestor(ctx, to, head) {
		return snap, res, nil
	}
	if opts.Strategy == PullFastForwardOnly && !r.isAncestor(ctx, head, to) {
		return snap, res, &Error{Code: CodePullDiverged}
	}
	count, err := r.run(ctx, "rev-list", "--count", head+".."+to)
	if err != nil {
		return snap, res, err
	}
	res.Commits = atoi(strings.TrimSpace(string(count)))
	if err := r.applyPull(ctx, opts, to); err != nil {
		after, _ := r.afterNetwork()
		res.Commits = 0
		return after, res, err
	}
	after, err := r.afterNetwork()
	return after, res, err
}

// pullWithBlock returns why opts cannot pull from snap, or "" when it can
// as far as the snapshot shows.
func pullWithBlock(snap Snapshot, opts PullOptions) Code {
	switch {
	case snap.Detached:
		return CodePushDetached
	case snap.Unborn():
		return CodePushUnborn
	}
	for _, ch := range snap.Unstaged {
		if ch.Conflicted {
			return CodePullConflicts
		}
	}
	if !opts.Autostash && !snap.Clean() {
		return CodePullDirty
	}
	return ""
}

// applyPull joins to into HEAD according to opts.Strategy.
func (r *Repository) applyPull(ctx context.Context, opts PullOptions, to string) error {
	autostash := "--no-autostash"
	mergeStash, rebaseStash := "merge.autoStash=false", "rebase.autoStash=false"
	if opts.Autostash {
		autostash = "--autostash"
		mergeStash, rebaseStash = "merge.autoStash=true", "rebase.autoStash=true"
	}
	switch opts.Strategy {
	case PullRebase:
		_, err := r.run(ctx, "-c", rebaseStash, "rebase", autostash, "--quiet", to)
		return err
	case PullMerge:
		_, err := r.run(ctx, "-c", mergeStash, "merge", autostash, "--no-edit", "--quiet", to)
		return err
	default:
		_, err := r.run(ctx, "-c", mergeStash, "merge", "--ff-only", autostash, "--no-edit", "--quiet", to)
		return err
	}
}

// isAncestor reports whether a is an ancestor of b, or b itself.
func (r *Repository) isAncestor(ctx context.Context, a, b string) bool {
	_, code, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, "merge-base", "--is-ancestor", a, b)
	return err == nil && code == 0
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
