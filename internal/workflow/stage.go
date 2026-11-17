package workflow

import (
	"bytes"
	"context"
	"strings"
)

const (
	// CodeNothingToStage: no unstaged or untracked change can be staged.
	CodeNothingToStage Code = "NothingToStage"
	// CodeUnstageAllConflicts: the index holds unresolved conflicts, which
	// unstaging everything would discard.
	CodeUnstageAllConflicts Code = "UnstageAllConflicts"
)

// Stageable reports whether ch can be staged or unstaged as a whole file.
// Conflicts are not resolved by staging, and a repository nested inside the
// worktree is not added as a submodule.
func (c Change) Stageable() bool {
	return !c.Conflicted && !(c.Area == Untracked && (c.Submodule || strings.HasSuffix(c.Path, "/")))
}

// BulkResult is what StageAll did besides the new state.
type BulkResult struct {
	// Staged counts the paths given to Git.
	Staged int
	// Skipped are changes left alone: conflicts and nested repositories.
	Skipped []Change
}

// StageAll stages every unstaged and untracked change of this worktree that
// can be staged, as whole files, as Git's ignore rules allow. Conflicts and
// nested repositories are skipped and reported. The working tree files are
// not changed, and nothing is committed.
func (r *Repository) StageAll(ctx context.Context) (Snapshot, BulkResult, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, BulkResult{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, BulkResult{}, err
	}
	var res BulkResult
	var list bytes.Buffer
	for _, ch := range append(append([]Change{}, before.Unstaged...), before.Untracked...) {
		if !ch.Stageable() {
			res.Skipped = append(res.Skipped, ch)
			continue
		}
		list.WriteString(ch.Path)
		list.WriteByte(0)
		res.Staged++
	}
	if res.Staged == 0 {
		return before, res, &Error{Code: CodeNothingToStage}
	}
	_, opErr := r.app.git.Run(ctx, r.root, &list, "--literal-pathspecs", "add", "-A", "--pathspec-from-file=-", "--pathspec-file-nul")
	return r.after(ctx, before, gitFailure(opErr), res)
}

// UnstageAll removes every staged change from the index, keeping the
// working tree files as they are; before the first commit it empties the
// index. With unresolved conflicts it does nothing.
func (r *Repository) UnstageAll(ctx context.Context) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	for _, ch := range before.Unstaged {
		if ch.Conflicted {
			return before, &Error{Code: CodeUnstageAllConflicts, Detail: ch.Path}
		}
	}
	if len(before.Staged) == 0 {
		return before, &Error{Code: CodeNothingStaged}
	}
	if before.Unborn() {
		_, err = r.run(ctx, "rm", "--cached", "-r", "-q", "--ignore-unmatch", "--", ".")
	} else {
		_, err = r.run(ctx, "reset", "-q", "--no-recurse-submodules")
	}
	snap, _, err := r.after(ctx, before, err, BulkResult{})
	return snap, err
}

// after reads the state that follows an operation, successful or not, so
// a failure shows what Git actually left.
func (r *Repository) after(ctx context.Context, before Snapshot, opErr error, res BulkResult) (Snapshot, BulkResult, error) {
	after, err := r.read(ctx)
	if err != nil {
		if opErr != nil {
			return before, res, opErr
		}
		return before, res, err
	}
	return after, res, opErr
}

// Stage adds the whole of ch, an unstaged or untracked change, to the index
// and returns the new state. When ch is no longer in the area it was read
// in, nothing runs and the error is CodeStateChanged, with the fresh state.
func (r *Repository) Stage(ctx context.Context, ch Change) (Snapshot, error) {
	if ch.Area == Staged {
		return Snapshot{}, &Error{Code: CodeStateChanged}
	}
	return r.change(ctx, ch, func() error {
		_, err := r.run(ctx, "--literal-pathspecs", "add", "-A", "--", ch.Path)
		return err
	})
}

// Unstage removes ch, a staged change, from the index and keeps the working
// tree file as it is. Before the first commit it removes the index entry.
func (r *Repository) Unstage(ctx context.Context, ch Change) (Snapshot, error) {
	if ch.Area != Staged {
		return Snapshot{}, &Error{Code: CodeStateChanged}
	}
	return r.change(ctx, ch, func() error {
		paths := []string{ch.Path}
		if ch.OrigPath != "" {
			paths = append(paths, ch.OrigPath)
		}
		head, err := r.run(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil || len(head) == 0 {
			// No commits yet: there is nothing to reset to.
			_, err = r.run(ctx, append([]string{"--literal-pathspecs", "rm", "--cached", "-q", "-r", "--ignore-unmatch", "--"}, paths...)...)
			return err
		}
		_, err = r.run(ctx, append([]string{"--literal-pathspecs", "reset", "-q", "HEAD", "--"}, paths...)...)
		return err
	})
}

// change runs op on ch after checking that the repository is still open and
// ch is still what Git reports, then reads the result.
func (r *Repository) change(ctx context.Context, ch Change, op func() error) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	before, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if !before.has(ch) {
		return before, &Error{Code: CodeStateChanged, Detail: ch.Path}
	}
	opErr := op()
	after, err := r.read(ctx)
	if opErr != nil {
		if err != nil {
			return before, opErr
		}
		return after, opErr
	}
	return after, err
}

// has reports whether s holds ch in the same area with the same kind.
func (s Snapshot) has(ch Change) bool {
	for _, c := range areaOf(s, ch.Area) {
		if c.Path == ch.Path && c.OrigPath == ch.OrigPath && c.Kind == ch.Kind && c.indexHash == ch.indexHash {
			return true
		}
	}
	return false
}

func areaOf(s Snapshot, a Area) []Change {
	switch a {
	case Staged:
		return s.Staged
	case Unstaged:
		return s.Unstaged
	}
	return s.Untracked
}
