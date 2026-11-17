package workflow

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gitowl/internal/git"
)

const (
	CodeInvalidBranchName  Code = "InvalidBranchName"
	CodeBranchExists       Code = "BranchExists"
	CodeBranchNotFound     Code = "BranchNotFound"
	CodeNoCommitsForBranch Code = "NoCommitsForBranch"
	CodeSwitchBlocked      Code = "SwitchBlocked"
	CodeDeleteCurrent      Code = "DeleteCurrent"
	CodeBranchInWorktree   Code = "BranchInWorktree"
	CodeBranchNotMerged    Code = "BranchNotMerged"
)

// Branch is a local branch.
type Branch struct {
	Name    string
	Head    string
	Current bool
	// Upstream is the configured upstream; Ahead and Behind compare with it
	// as the repository last recorded, and Gone marks an upstream that no
	// longer exists locally.
	Upstream      string
	Ahead, Behind int
	Gone          bool
}

// RemoteBranch is a remote-tracking branch as last fetched outside GitOwl.
type RemoteBranch struct {
	Name string
	Head string
}

// Branches lists a repository's branches.
type Branches struct {
	Local  []Branch
	Remote []RemoteBranch
}

// Branches reads the local and remote-tracking branches.
func (r *Repository) Branches(ctx context.Context) (Branches, error) {
	out, err := r.run(ctx, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(HEAD)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return Branches{}, err
	}
	var b Branches
	for line := range strings.Lines(string(out)) {
		f := strings.Split(strings.TrimSuffix(line, "\n"), "\x00")
		if len(f) != 6 || f[5] != "" {
			continue // malformed, or a symbolic ref such as origin/HEAD
		}
		ref, head := f[0], f[1]
		switch {
		case strings.HasPrefix(ref, "refs/heads/"):
			br := Branch{Name: strings.TrimPrefix(ref, "refs/heads/"), Head: head, Current: f[2] == "*", Upstream: f[3]}
			parseTrack(&br, f[4])
			b.Local = append(b.Local, br)
		case strings.HasPrefix(ref, "refs/remotes/"):
			b.Remote = append(b.Remote, RemoteBranch{Name: strings.TrimPrefix(ref, "refs/remotes/"), Head: head})
		}
	}
	return b, nil
}

func parseTrack(b *Branch, track string) {
	if track == "gone" {
		b.Gone = true
		return
	}
	for part := range strings.SplitSeq(track, ", ") {
		kind, n, ok := strings.Cut(part, " ")
		if !ok {
			continue
		}
		v, _ := strconv.Atoi(n)
		switch kind {
		case "ahead":
			b.Ahead = v
		case "behind":
			b.Behind = v
		}
	}
}

// branchOp runs op while the repository is current and returns the state
// and branches read after it, whether or not op failed.
func (r *Repository) branchOp(ctx context.Context, op func(Snapshot, Branches) error) (Snapshot, Branches, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, Branches{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Branches{}, err
	}
	branches, err := r.Branches(ctx)
	if err != nil {
		return snap, Branches{}, err
	}
	opErr := op(snap, branches)
	if opErr == nil || CodeOf(opErr) == CodeGitFailed || CodeOf(opErr) == CodeSwitchBlocked || CodeOf(opErr) == CodeBranchNotMerged || CodeOf(opErr) == CodeCheckoutNotSwitched {
		if s, err := r.read(ctx); err == nil {
			snap = s
		}
		if b, err := r.Branches(ctx); err == nil {
			branches = b
		}
	}
	return snap, branches, opErr
}

func (b Branches) local(name string) (Branch, bool) {
	for _, br := range b.Local {
		if br.Name == name {
			return br, true
		}
	}
	return Branch{}, false
}

// CreateBranch creates a local branch at the checked-out commit, and
// switches to it when switchTo is set.
func (r *Repository) CreateBranch(ctx context.Context, name string, switchTo bool) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		if snap.Unborn() {
			return &Error{Code: CodeNoCommitsForBranch}
		}
		if strings.TrimSpace(name) != name || name == "" {
			return &Error{Code: CodeInvalidBranchName, Detail: name}
		}
		if _, err := r.run(ctx, "check-ref-format", "--branch", name); err != nil {
			return fail(CodeInvalidBranchName, err)
		}
		if _, ok := b.local(name); ok {
			return &Error{Code: CodeBranchExists, Detail: name}
		}
		// check-ref-format --branch rejects names starting with "-".
		if switchTo {
			_, err := r.run(ctx, "switch", "--quiet", "--no-guess", "-c", name)
			return err
		}
		_, err := r.run(ctx, "branch", "--no-track", name)
		return err
	})
}

// SwitchBranch checks out a local branch. Git refuses when local changes
// would be overwritten; they are kept, never stashed or discarded.
func (r *Repository) SwitchBranch(ctx context.Context, name string) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		br, ok := b.local(name)
		if !ok {
			return &Error{Code: CodeBranchNotFound, Detail: name}
		}
		if br.Current {
			return nil
		}
		if err := r.notInOtherWorktree(ctx, name); err != nil {
			return err
		}
		_, err := r.run(ctx, "switch", "--quiet", "--no-guess", name)
		return switchFailure(err)
	})
}

// CheckoutRemote checks out the remote-tracking branch remote/branch. When a
// local branch of the same name exists, it is switched to, as Git does for
// a branch name. Otherwise a local branch that tracks the remote branch is
// created and checked out. A remote's HEAD is refused. Local changes are
// kept, as SwitchBranch keeps them.
func (r *Repository) CheckoutRemote(ctx context.Context, remote, branch string) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		ref := remote + "/" + branch
		found := false
		for _, rb := range b.Remote {
			if rb.Name == ref {
				found = true
				break
			}
		}
		if !found || branch == "HEAD" || branch == "" {
			return &Error{Code: CodeBranchNotFound, Detail: ref}
		}
		if local, ok := b.local(branch); ok {
			if local.Current {
				return nil
			}
			if err := r.notInOtherWorktree(ctx, branch); err != nil {
				return err
			}
			_, err := r.run(ctx, "switch", "--quiet", "--no-guess", branch)
			return switchFailure(err)
		}
		_, err := r.run(ctx, "switch", "--quiet", "--no-guess", "-c", branch, "--track", "refs/remotes/"+ref)
		return switchFailure(err)
	})
}

// notInOtherWorktree refuses a branch that another worktree has checked
// out; Git keeps a branch in one worktree at a time. The error's detail is
// that worktree's path.
func (r *Repository) notInOtherWorktree(ctx context.Context, name string) error {
	list, err := r.Worktrees(ctx)
	if err != nil {
		return err
	}
	for _, w := range list {
		if !w.Current && !w.Detached && w.Branch == name {
			return &Error{Code: CodeBranchInWorktree, Detail: w.Path}
		}
	}
	return nil
}

func switchFailure(err error) error {
	var e *Error
	if errors.As(err, &e) {
		var g *git.Error
		if errors.As(e.Err, &g) {
			msg := g.Message()
			if strings.Contains(msg, "is already used by worktree") || strings.Contains(msg, "already checked out at") {
				return fail(CodeBranchInWorktree, g)
			}
			if strings.Contains(msg, "would be overwritten") || strings.Contains(msg, "you need to resolve your current index first") || strings.Contains(msg, "Please commit your changes or stash them") {
				return fail(CodeSwitchBlocked, g)
			}
		}
	}
	return err
}

// DeleteBranch deletes a local branch that is merged, as git branch -d
// decides. It refuses the current branch and never forces.
func (r *Repository) DeleteBranch(ctx context.Context, name string) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		br, ok := b.local(name)
		if !ok {
			return &Error{Code: CodeBranchNotFound, Detail: name}
		}
		if br.Current {
			return &Error{Code: CodeDeleteCurrent, Detail: name}
		}
		if err := r.notInOtherWorktree(ctx, name); err != nil {
			return err
		}
		_, err := r.run(ctx, "branch", "-d", name)
		var e *Error
		if errors.As(err, &e) && strings.Contains(e.Detail, "not fully merged") {
			return fail(CodeBranchNotMerged, e.Err)
		}
		if errors.As(err, &e) && strings.Contains(e.Detail, "used by worktree") {
			return fail(CodeBranchInWorktree, e.Err)
		}
		return err
	})
}
