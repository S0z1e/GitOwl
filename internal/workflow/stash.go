package workflow

import (
	"context"
	"strings"

	"gitowl/internal/git"
)

const (
	// CodeNothingToStash: the worktree has no staged, unstaged, or (when
	// requested) untracked change, so a stash would be empty.
	CodeNothingToStash Code = "NothingToStash"
	// CodeStashNotFound: the named stash is not in the list.
	CodeStashNotFound Code = "StashNotFound"
)

// Stash is one stash@{n} entry.
type Stash struct {
	// Ref is the reflog name, such as stash@{0}.
	Ref string
	// Hash is the stash commit.
	Hash string
	// Message is the stash's subject.
	Message string
}

// Stashes lists stash@{n} entries, newest first.
func (r *Repository) Stashes(ctx context.Context) ([]Stash, error) {
	out, err := r.run(ctx, "stash", "list", "--format=%gd%x00%H%x00%gs")
	if err != nil {
		return nil, err
	}
	var list []Stash
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			continue
		}
		f := strings.SplitN(line, "\x00", 3)
		if len(f) != 3 {
			continue
		}
		list = append(list, Stash{Ref: f[0], Hash: f[1], Message: f[2]})
	}
	return list, nil
}

// Stash records the current worktree's changes as a new stash. When
// includeUntracked is set, untracked files are included. A clean worktree
// fails with CodeNothingToStash and does not create an empty stash.
func (r *Repository) Stash(ctx context.Context, includeUntracked bool) (Snapshot, Stash, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, Stash{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Stash{}, err
	}
	if nothingToStash(before, includeUntracked) {
		return before, Stash{}, &Error{Code: CodeNothingToStash}
	}
	args := []string{"stash", "push", "--quiet"}
	if includeUntracked {
		args = append(args, "--include-untracked")
	}
	if _, err := r.run(ctx, args...); err != nil {
		after, rerr := r.read(ctx)
		if rerr != nil {
			return before, Stash{}, err
		}
		return after, Stash{}, err
	}
	after, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, Stash{}, err
	}
	list, err := r.Stashes(ctx)
	if err != nil {
		return after, Stash{}, err
	}
	if len(list) == 0 {
		return after, Stash{}, &Error{Code: CodeGitFailed, Detail: "stash created nothing"}
	}
	return after, list[0], nil
}

func nothingToStash(s Snapshot, includeUntracked bool) bool {
	if len(s.Staged) > 0 || len(s.Unstaged) > 0 {
		return false
	}
	return !includeUntracked || len(s.Untracked) == 0
}

// StashDetail is a stash opened for review: the files it recorded.
type StashDetail struct {
	Stash
	Files []Change
}

// StashShow lists the files recorded in ref, including untracked files
// that were stashed.
func (r *Repository) StashShow(ctx context.Context, ref string) (StashDetail, error) {
	st, err := r.stashByRef(ctx, ref)
	if err != nil {
		return StashDetail{}, err
	}
	files, err := r.stashFiles(ctx, st.Ref)
	if err != nil {
		return StashDetail{}, err
	}
	return StashDetail{Stash: st, Files: files}, nil
}

// StashDiff returns path's unified diff as recorded in ref, using the
// same Diff type as working-tree and commit review.
func (r *Repository) StashDiff(ctx context.Context, ref, path string) (Diff, error) {
	st, err := r.stashByRef(ctx, ref)
	if err != nil {
		return Diff{}, err
	}
	files, err := r.stashFiles(ctx, st.Ref)
	if err != nil {
		return Diff{}, err
	}
	ch := Change{Path: path, Area: InCommit}
	for _, f := range files {
		if f.Path == path {
			ch = f
			break
		}
	}
	out, err := r.stashPathDiff(ctx, st.Ref, path)
	if err != nil {
		return Diff{}, err
	}
	return classify(Diff{Change: ch}, out), nil
}

func (r *Repository) stashByRef(ctx context.Context, ref string) (Stash, error) {
	list, err := r.Stashes(ctx)
	if err != nil {
		return Stash{}, err
	}
	for _, st := range list {
		if st.Ref == ref || st.Hash == ref {
			return st, nil
		}
	}
	return Stash{}, &Error{Code: CodeStashNotFound, Detail: ref}
}

func (r *Repository) stashFiles(ctx context.Context, ref string) ([]Change, error) {
	raw, err := r.run(ctx, "diff", "--raw", "-z", "--no-abbrev", "--find-renames", "--no-ext-diff", ref+"^1", ref, "--")
	if err != nil {
		return nil, err
	}
	files, err := changesFromRaw(raw)
	if err != nil {
		return nil, err
	}
	third, ok := r.stashUntracked(ctx, ref)
	if !ok {
		return files, nil
	}
	empty, err := r.emptyTree(ctx)
	if err != nil {
		return nil, err
	}
	uraw, err := r.run(ctx, "diff", "--raw", "-z", "--no-abbrev", "--no-ext-diff", empty, third, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := changesFromRaw(uraw)
	if err != nil {
		return nil, err
	}
	return append(files, untracked...), nil
}

func changesFromRaw(raw []byte) ([]Change, error) {
	parsed, err := git.ParseRaw(raw)
	if err != nil {
		return nil, fail(CodeGitFailed, err)
	}
	var files []Change
	for _, rc := range parsed {
		files = append(files, Change{Path: rc.Path, OrigPath: rc.OrigPath, Kind: rawKind(rc.Status), Area: InCommit, Submodule: rc.Submodule()})
	}
	return files, nil
}

func (r *Repository) stashPathDiff(ctx context.Context, ref, path string) ([]byte, error) {
	args := append([]string{"--literal-pathspecs", "diff"}, diffOptions...)
	out, err := r.run(ctx, append(args, ref+"^1", ref, "--", path)...)
	if err != nil {
		return nil, err
	}
	if len(out) > 0 {
		return out, nil
	}
	third, ok := r.stashUntracked(ctx, ref)
	if !ok {
		return out, nil
	}
	empty, err := r.emptyTree(ctx)
	if err != nil {
		return nil, err
	}
	args = append([]string{"--literal-pathspecs", "diff"}, diffOptions...)
	return r.run(ctx, append(args, empty, third, "--", path)...)
}

func (r *Repository) stashUntracked(ctx context.Context, ref string) (string, bool) {
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, "rev-parse", "--verify", "--quiet", ref+"^3")
	hash := strings.TrimSpace(string(out))
	return hash, err == nil && hash != ""
}

// ApplyStash restores ref onto the worktree and keeps the stash.
func (r *Repository) ApplyStash(ctx context.Context, ref string) (Snapshot, error) {
	return r.restoreStash(ctx, ref, false)
}

// PopStash restores ref onto the worktree and removes the stash.
func (r *Repository) PopStash(ctx context.Context, ref string) (Snapshot, error) {
	return r.restoreStash(ctx, ref, true)
}

// DropStash removes ref without restoring it.
func (r *Repository) DropStash(ctx context.Context, ref string) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	st, err := r.stashByRef(ctx, ref)
	if err != nil {
		snap, _ := r.read(ctx)
		return snap, err
	}
	if _, err := r.run(ctx, "stash", "drop", "--quiet", st.Ref); err != nil {
		after, rerr := r.read(ctx)
		if rerr != nil {
			return Snapshot{}, err
		}
		return after, err
	}
	return r.read(ctx)
}

func (r *Repository) restoreStash(ctx context.Context, ref string, pop bool) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	st, err := r.stashByRef(ctx, ref)
	if err != nil {
		snap, _ := r.read(ctx)
		return snap, err
	}
	cmd := "apply"
	if pop {
		cmd = "pop"
	}
	_, _, err = r.app.git.RunStatus(ctx, r.root, nil, []int{1}, "stash", cmd, "--quiet", st.Ref)
	after, rerr := r.read(ctx)
	if rerr != nil {
		if err != nil {
			return Snapshot{}, gitFailure(err)
		}
		return Snapshot{}, rerr
	}
	if err != nil && !conflicted(after) {
		return after, gitFailure(err)
	}
	return after, nil
}

// RenameStash replaces ref's message. The stash commit is unchanged.
func (r *Repository) RenameStash(ctx context.Context, ref, message string) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	st, err := r.stashByRef(ctx, ref)
	if err != nil {
		snap, _ := r.read(ctx)
		return snap, err
	}
	if _, err := r.run(ctx, "stash", "drop", "--quiet", st.Ref); err != nil {
		after, rerr := r.read(ctx)
		if rerr != nil {
			return Snapshot{}, err
		}
		return after, err
	}
	if _, err := r.run(ctx, "stash", "store", "-m", message, st.Hash); err != nil {
		after, rerr := r.read(ctx)
		if rerr != nil {
			return Snapshot{}, err
		}
		return after, err
	}
	return r.read(ctx)
}

// BranchFromStash creates name from the commit ref was saved on. When
// switchTo is set, it checks the branch out, applies the stash, and drops
// it, as git stash branch does. Otherwise the stash and the current
// worktree stay as they are.
func (r *Repository) BranchFromStash(ctx context.Context, name, ref string, switchTo bool) (Snapshot, Branches, error) {
	snap, b, err := r.branchOp(ctx, func(snap Snapshot, b Branches) error {
		st, err := r.stashByRef(ctx, ref)
		if err != nil {
			return err
		}
		if err := r.validBranchName(ctx, name, b); err != nil {
			return err
		}
		if !switchTo {
			_, err = r.run(ctx, "branch", "--no-track", name, st.Ref+"^1")
			return err
		}
		_, err = r.run(ctx, "stash", "branch", name, st.Ref)
		if err != nil {
			return switchFailure(err)
		}
		return nil
	})
	if err != nil && conflicted(snap) {
		return snap, b, nil
	}
	return snap, b, err
}
