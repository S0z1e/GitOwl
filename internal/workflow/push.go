package workflow

import (
	"context"
	"strings"
)

const (
	// CodePushDetached: HEAD is not on a branch, so there is nothing named
	// to push.
	CodePushDetached Code = "PushDetached"
	// CodePushUnborn: the branch has no commits yet.
	CodePushUnborn Code = "PushUnborn"
	// CodePushRejected: the remote branch has commits the local one does
	// not, and a normal push would lose them.
	CodePushRejected Code = "PushRejected"
	// CodePushRemoteRejected: the remote refused the update, as a
	// protected branch or a server hook does.
	CodePushRemoteRejected Code = "PushRemoteRejected"
	// CodePushUncertain: the push stopped before Git reported its result;
	// the remote may or may not have been updated.
	CodePushUncertain Code = "PushUncertain"
	// CodeBranchChanged: the current branch is not the one the push was
	// asked for.
	CodeBranchChanged Code = "BranchChanged"
)

// PushTarget is where the current branch goes when pushed.
type PushTarget struct {
	Remote, Branch string
}

// String is the target as Git shows it, as origin/main.
func (p PushTarget) String() string { return p.Remote + "/" + p.Branch }

// pushTarget resolves where the current branch is pushed: to
// branch.<name>.pushRemote, remote.pushDefault or its upstream's remote,
// into its upstream branch on that remote and into a branch of the same
// name on another. Without an upstream on a configured remote there is no
// target: the branch must be published first, to a remote and branch the
// user confirms.
func (r *Repository) pushTarget(ctx context.Context, info *RemoteInfo) {
	b := info.Branch
	if b == "" || info.UpstreamRemote == "" {
		return
	}
	remote := r.config(ctx, "branch."+b+".pushRemote")
	if remote == "" {
		remote = r.config(ctx, "remote.pushDefault")
	}
	if remote == "" || !info.Has(remote) {
		remote = info.UpstreamRemote
	}
	target := b
	if remote == info.UpstreamRemote {
		target = info.UpstreamBranch
	}
	info.Push = &PushTarget{Remote: remote, Branch: target}
}

// PublishRemote suggests the remote to publish the current branch to:
// remote.pushDefault, the only remote or origin; "" when the user must
// choose.
func (ri RemoteInfo) PublishRemote() string {
	if ri.PushDefault != "" && ri.Has(ri.PushDefault) {
		return ri.PushDefault
	}
	return ri.DefaultFetch()
}

// PushResult is what a push did.
type PushResult struct {
	Target PushTarget
	// UpToDate is set when the remote already had the commits.
	UpToDate bool
	// Tracking is set when the push made the target the branch's upstream.
	Tracking bool
}

// PushOptions is the remote, branch and flags a PushWith runs. Branch is
// the remote branch name and may not exist yet: PushWith creates it.
type PushOptions struct {
	Remote, Branch string
	SetUpstream    bool
	PushTags       bool
	ForceWithLease bool
}

// Push pushes the current branch, which must be branch, to target: only
// that one branch, as a fast-forward, without tags, whatever push.default,
// push.followTags or the remote's push refspecs say. With setUpstream the
// target becomes the branch's upstream once the remote has its commits.
// The local branch, the index and the files never change. The state read
// afterwards is returned whatever the result.
func (r *Repository) Push(ctx context.Context, branch string, target PushTarget, setUpstream bool) (Snapshot, PushResult, error) {
	return r.push(ctx, branch, PushOptions{
		Remote: target.Remote, Branch: target.Branch, SetUpstream: setUpstream,
	}, true)
}

// PushWith pushes the current branch to opts.Remote/opts.Branch. A remote
// branch that does not exist yet is created when the push runs. Force with
// lease and pushing tags are opt-in; a regular Push still fast-forwards
// without tags. The state read afterwards is returned whatever the result.
func (r *Repository) PushWith(ctx context.Context, opts PushOptions) (Snapshot, PushResult, error) {
	return r.push(ctx, "", opts, false)
}

func (r *Repository) push(ctx context.Context, branch string, opts PushOptions, checkBranch bool) (Snapshot, PushResult, error) {
	res := PushResult{Target: PushTarget{Remote: opts.Remote, Branch: opts.Branch}}
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, res, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, res, err
	}
	switch {
	case snap.Detached:
		return snap, res, &Error{Code: CodePushDetached}
	case checkBranch && snap.Branch != branch:
		return snap, res, &Error{Code: CodeBranchChanged, Detail: snap.Branch}
	case snap.Unborn():
		return snap, res, &Error{Code: CodePushUnborn}
	}
	local := snap.Branch
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
	refspec := "refs/heads/" + local + ":refs/heads/" + name
	args := []string{"push", "--porcelain"}
	if opts.PushTags {
		args = append(args, "--tags")
	} else {
		args = append(args, "--no-follow-tags")
	}
	if opts.ForceWithLease {
		args = append(args, "--force-with-lease")
	}
	args = append(args, "--recurse-submodules=no", opts.Remote, refspec)
	out, err := r.network(ctx, args...)
	status, summary := pushStatus(out, "refs/heads/"+name)
	switch {
	case err == nil:
		res.UpToDate = status == '='
		if opts.SetUpstream {
			if err = r.track(ctx, local, res.Target); err == nil {
				res.Tracking = true
			}
		}
	case status == '!' && strings.Contains(summary, "remote rejected"):
		err = &Error{Code: CodePushRemoteRejected, Detail: summary, Err: err}
	case status == '!':
		err = &Error{Code: CodePushRejected, Detail: summary, Err: err}
	case CodeOf(err) == CodeCancelled || CodeOf(err) == CodeTimedOut:
		err = &Error{Code: CodePushUncertain, Err: err}
	}
	after, rerr := r.afterNetwork()
	if err == nil {
		err = rerr
	}
	return after, res, err
}

// track makes target the upstream of branch.
func (r *Repository) track(ctx context.Context, branch string, target PushTarget) error {
	if _, err := r.run(ctx, "config", "branch."+branch+".remote", target.Remote); err != nil {
		return err
	}
	_, err := r.run(ctx, "config", "branch."+branch+".merge", "refs/heads/"+target.Branch)
	return err
}

// pushStatus finds the line of ref in git push --porcelain output and
// returns its flag and summary: ' ' fast-forward, '*' new, '=' up to date,
// '!' rejected; 0 when the ref was not reported.
func pushStatus(out []byte, ref string) (byte, string) {
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSuffix(line, "\n")
		if len(line) < 2 || line[1] != '\t' {
			continue
		}
		fields := strings.SplitN(line[2:], "\t", 2)
		if len(fields) < 1 {
			continue
		}
		if _, to, ok := strings.Cut(fields[0], ":"); ok && to == ref {
			summary := ""
			if len(fields) == 2 {
				summary = fields[1]
			}
			return line[0], summary
		}
	}
	return 0, ""
}
