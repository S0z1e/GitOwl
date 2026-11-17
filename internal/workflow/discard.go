package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gitowl/internal/git"
)

const (
	// CodeDiscardStale: the file, the staging area or HEAD changed since
	// the discard was confirmed; nothing was discarded.
	CodeDiscardStale Code = "DiscardStale"
	// Reasons a change cannot be discarded.
	CodeDiscardUntracked Code = "DiscardUntracked"
	CodeDiscardConflict  Code = "DiscardConflict"
	CodeDiscardSubmodule Code = "DiscardSubmodule"
	CodeDiscardSymlink   Code = "DiscardSymlink"
	CodeDiscardRenamed   Code = "DiscardRenamed"
	CodeDiscardNothing   Code = "DiscardNothing"
)

// DiscardPlan is a discard of one file's unstaged changes, as confirmed:
// the working tree file goes back to its staged version. It holds what the
// file, the index and HEAD were, so Discard refuses when any changed.
type DiscardPlan struct {
	Change Change
	// Path is the file's absolute path.
	Path string
	// Deleted is set when the file is missing from the working tree, so
	// discarding restores it.
	Deleted bool
	// Staged is set when the file has staged changes too; they stay.
	Staged bool
	// Binary and TooLarge say that the diff cannot be shown as text; the
	// whole file is restored all the same.
	Binary, TooLarge bool

	head, indexEntry, worktree string
}

// PlanDiscard checks that ch, an unstaged change, can be discarded and
// returns what the confirmation shows and Discard checks again. Only
// regular tracked files in the index are supported, never untracked files,
// conflicts, submodules, symbolic links, type changes or a staged rename.
func (r *Repository) PlanDiscard(ctx context.Context, ch Change) (DiscardPlan, error) {
	snap, err := r.read(ctx)
	if err != nil {
		return DiscardPlan{}, err
	}
	return r.plan(ctx, snap, ch)
}

func (r *Repository) plan(ctx context.Context, snap Snapshot, ch Change) (DiscardPlan, error) {
	p := DiscardPlan{Change: ch, head: snap.Head}
	switch {
	case ch.Area == Untracked:
		return p, &Error{Code: CodeDiscardUntracked}
	case ch.Area != Unstaged:
		return p, &Error{Code: CodeDiscardNothing}
	case ch.Conflicted:
		return p, &Error{Code: CodeDiscardConflict}
	case ch.Submodule:
		return p, &Error{Code: CodeDiscardSubmodule}
	case ch.Kind == git.TypeChanged:
		return p, &Error{Code: CodeDiscardSymlink}
	}
	listed := false
	for _, c := range snap.Unstaged {
		if c.Path == ch.Path {
			listed = true
		}
	}
	if !listed {
		return p, &Error{Code: CodeDiscardNothing}
	}
	for _, c := range snap.Staged {
		if c.Path == ch.Path {
			p.Staged = true
			if c.OrigPath != "" {
				return p, &Error{Code: CodeDiscardRenamed}
			}
		}
	}
	abs, err := r.insidePath(ch.Path)
	if err != nil {
		return p, err
	}
	p.Path = abs
	out, err := r.run(ctx, "--literal-pathspecs", "ls-files", "-s", "-z", "--", ch.Path)
	if err != nil {
		return p, err
	}
	entry := strings.TrimSuffix(string(out), "\x00")
	mode, _, _ := strings.Cut(entry, " ")
	switch {
	case entry == "" || strings.Contains(entry, "\x00"):
		return p, &Error{Code: CodeDiscardNothing}
	case mode == "120000":
		return p, &Error{Code: CodeDiscardSymlink}
	case mode == "160000":
		return p, &Error{Code: CodeDiscardSubmodule}
	case mode != "100644" && mode != "100755":
		return p, &Error{Code: CodeDiscardSymlink}
	}
	p.indexEntry = entry
	p.worktree, p.Deleted, err = fileIdentity(abs)
	if err != nil {
		return p, err
	}
	if !p.Deleted {
		if d, err := r.FileDiff(ctx, ch); err == nil {
			p.Binary, p.TooLarge = d.Binary, d.TooLarge
		}
	}
	return p, nil
}

// insidePath returns rel's absolute path in the worktree after checking
// that no folder on the way is a symbolic link, so the file restored is
// the one inside this worktree.
func (r *Repository) insidePath(rel string) (string, error) {
	abs := filepath.Join(r.root, filepath.FromSlash(rel))
	if !strings.HasPrefix(abs, r.root+string(filepath.Separator)) {
		return "", &Error{Code: CodeDiscardSymlink, Detail: rel}
	}
	dir := r.root
	parts := strings.Split(filepath.FromSlash(rel), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		dir = filepath.Join(dir, part)
		st, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			break // a deleted folder: checkout makes it again
		}
		if err != nil {
			return "", fail(CodeNoAccess, err)
		}
		if st.Mode()&fs.ModeSymlink != 0 || !st.IsDir() {
			return "", &Error{Code: CodeDiscardSymlink, Detail: rel}
		}
	}
	return abs, nil
}

// fileIdentity hashes a regular file's mode and bytes; a missing file is
// "deleted". A symbolic link or another kind of file is refused.
func fileIdentity(path string) (string, bool, error) {
	st, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "deleted", true, nil
	}
	if err != nil {
		return "", false, fail(CodeNoAccess, err)
	}
	if !st.Mode().IsRegular() {
		return "", false, &Error{Code: CodeDiscardSymlink}
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false, fail(CodeNoAccess, err)
	}
	defer f.Close()
	h := sha256.New()
	h.Write([]byte(st.Mode().Perm().String()))
	if _, err := io.Copy(h, f); err != nil {
		return "", false, fail(CodeNoAccess, err)
	}
	return hex.EncodeToString(h.Sum(nil)), false, nil
}

// Discard restores the working tree file of p to its staged version. The
// index, HEAD and every other file stay as they are. When the file, its
// staged version or HEAD changed since p was made, it changes nothing and
// reports CodeDiscardStale with the state as it is now.
func (r *Repository) Discard(ctx context.Context, p DiscardPlan) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	now, err := r.plan(ctx, snap, p.Change)
	if err != nil {
		if CodeOf(err) == CodeDiscardNothing {
			err = &Error{Code: CodeDiscardStale, Detail: p.Change.Path}
		}
		return snap, err
	}
	if now.head != p.head || now.indexEntry != p.indexEntry || now.worktree != p.worktree {
		return snap, &Error{Code: CodeDiscardStale, Detail: p.Change.Path}
	}
	_, cerr := r.run(ctx, "--literal-pathspecs", "checkout", "-q", "--", p.Change.Path)
	after, err := r.read(ctx)
	if cerr != nil {
		return after, cerr
	}
	return after, err
}

// DiscardUnstaged restores every tracked unstaged file's working tree to
// its staged version. The index, HEAD, untracked files and other worktrees
// stay as they are. There is no confirmation; files PlanDiscard would
// refuse are skipped. With nothing to restore it reports CodeDiscardNothing.
func (r *Repository) DiscardUnstaged(ctx context.Context) (Snapshot, error) {
	defer r.lockWorktree()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	snap, err := r.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	var paths []string
	for _, ch := range snap.Unstaged {
		if _, err := r.plan(ctx, snap, ch); err != nil {
			continue
		}
		paths = append(paths, ch.Path)
	}
	if len(paths) == 0 {
		return snap, &Error{Code: CodeDiscardNothing}
	}
	args := append([]string{"--literal-pathspecs", "checkout", "-q", "--"}, paths...)
	_, cerr := r.run(ctx, args...)
	after, rerr := r.read(ctx)
	if cerr != nil {
		if after.Root != "" {
			return after, cerr
		}
		return snap, cerr
	}
	return after, rerr
}
