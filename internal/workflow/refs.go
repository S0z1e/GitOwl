package workflow

import (
	"context"
	"strings"
)

const (
	// CodeInvalidTagName: the tag name is empty, padded, or Git rejects it.
	CodeInvalidTagName Code = "InvalidTagName"
	// CodeTagExists: a tag with that name is already in the repository.
	CodeTagExists Code = "TagExists"
	// CodeTagTargetMissing: the tag's target names no commit.
	CodeTagTargetMissing Code = "TagTargetMissing"
)

// RenameBranch renames a local branch. The worktree stays on the same
// commit; when the renamed branch is current, Snapshot.Branch is the new
// name.
func (r *Repository) RenameBranch(ctx context.Context, old, new string) (Snapshot, Branches, error) {
	return r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		if _, ok := b.local(old); !ok {
			return &Error{Code: CodeBranchNotFound, Detail: old}
		}
		if err := r.validBranchName(ctx, new, b); err != nil {
			return err
		}
		_, err := r.run(ctx, "branch", "-m", "--", old, new)
		return err
	})
}

// Tag is a named tag in the repository.
type Tag struct {
	Name string
	Hash string
}

// Tags lists tag names as Git reports them, without peeling annotated tags.
func (r *Repository) Tags(ctx context.Context) ([]Tag, error) {
	out, err := r.run(ctx, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/tags")
	if err != nil {
		return nil, err
	}
	var tags []Tag
	for line := range strings.Lines(string(out)) {
		f := strings.Split(strings.TrimSuffix(line, "\n"), "\x00")
		if len(f) != 2 || f[0] == "" {
			continue
		}
		name := strings.TrimPrefix(f[0], "refs/tags/")
		if name == f[0] {
			continue
		}
		tags = append(tags, Tag{Name: name, Hash: f[1]})
	}
	return tags, nil
}

// CreateTag points a lightweight tag at target. It writes no annotation
// and no message.
func (r *Repository) CreateTag(ctx context.Context, name, target string) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if strings.TrimSpace(name) != name || name == "" {
		return snap, &Error{Code: CodeInvalidTagName, Detail: name}
	}
	if _, err := r.run(ctx, "check-ref-format", "refs/tags/"+name); err != nil {
		return snap, fail(CodeInvalidTagName, err)
	}
	if hash, err := r.verifyRef(ctx, "refs/tags/"+name); err != nil {
		return snap, err
	} else if hash != "" {
		return snap, &Error{Code: CodeTagExists, Detail: name}
	}
	hash, err := r.verifyRef(ctx, target)
	if err != nil {
		return snap, err
	}
	if hash == "" {
		return snap, &Error{Code: CodeTagTargetMissing, Detail: target}
	}
	if _, err := r.run(ctx, "tag", "--", name, hash); err != nil {
		after, _ := r.read(ctx)
		return after, err
	}
	after, err := r.read(ctx)
	if err != nil {
		return snap, err
	}
	return after, nil
}

// DeleteTag deletes a local tag. A copy of the tag on a remote is not
// touched.
func (r *Repository) DeleteTag(ctx context.Context, name string) error {
	defer r.lockRepository()()
	if !r.Current() {
		return &Error{Code: CodeRepositoryClosed}
	}
	_, err := r.run(ctx, "tag", "-d", "--", name)
	return err
}

// PushTag pushes the local tag name to remote under the same name. Git
// refuses a remote tag that points elsewhere, and that refusal is returned.
// The state read afterwards is returned whatever the result.
func (r *Repository) PushTag(ctx context.Context, remote, name string) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	if _, err := r.checkRemote(ctx, remote); err != nil {
		return Snapshot{}, err
	}
	_, err := r.network(ctx, "push", "--no-recurse-submodules", "--", remote, "refs/tags/"+name+":refs/tags/"+name)
	after, rerr := r.afterNetwork()
	if err == nil {
		err = rerr
	}
	return after, err
}

// DeleteRemoteTag deletes the tag name on remote, then the local tag of the
// same name when there is one. When the remote refuses, the local tag is
// kept. The state read afterwards is returned whatever the result.
func (r *Repository) DeleteRemoteTag(ctx context.Context, remote, name string) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	if _, err := r.checkRemote(ctx, remote); err != nil {
		return Snapshot{}, err
	}
	_, err := r.network(ctx, "push", "--no-recurse-submodules", "--delete", "--", remote, "refs/tags/"+name)
	if err == nil {
		if hash, verr := r.verifyRef(ctx, "refs/tags/"+name); verr == nil && hash != "" {
			_, err = r.run(ctx, "tag", "-d", "--", name)
		}
	}
	after, rerr := r.afterNetwork()
	if err == nil {
		err = rerr
	}
	return after, err
}
