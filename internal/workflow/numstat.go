package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitowl/internal/git"
)

// FileStat is one path's line counts in one area of the change list.
type FileStat struct {
	Path, OrigPath string
	Area           Area
	// Added and Removed are line counts; both are 0 when Binary is set,
	// which marks a change Git does not count in lines.
	Added, Removed int
	Binary         bool
}

// Numstat is the added and removed line counts of a whole change list:
// staged, unstaged and untracked files, with the staged totals.
type Numstat struct {
	Files                      []FileStat
	StagedAdded, StagedRemoved int
	// Basis identifies the repository state the counts were read from, so a
	// result can be dropped once the state has moved (Stale). The Snapshot
	// carries only StagedFingerprint, which covers the staged side alone;
	// numstat also depends on the index and HEAD, so Basis combines the
	// staged fingerprint with HEAD's object id and the index file's
	// modification time, as the task's fallback prescribes for a state the
	// Snapshot has no single identifier for.
	Basis string
}

// Stale reports whether the repository has moved away from the state the
// counts were read from. It recomputes the basis from snap, read now, and
// the index file as it stands, and compares.
func (n Numstat) Stale(snap Snapshot) bool {
	return n.Basis != basis(snap)
}

// basis combines the staged fingerprint, HEAD object id and the index
// file's modification time into one identifier of the whole change list's
// state. The index mtime moves whenever Git rewrites the index (staging or
// unstaging), which StagedFingerprint alone would miss for the unstaged
// side; a missing index (an unborn repository before any add) contributes
// an empty time.
func basis(snap Snapshot) string {
	return fmt.Sprintf("%s\x00%s\x00%s", snap.StagedFingerprint, snap.Head, indexMtime(snap.Root))
}

// indexMtime returns the repository index file's modification time as text,
// or "" when it cannot be read. It consults the worktree's own index via
// its .git directory, falling back to the conventional path.
func indexMtime(root string) string {
	if root == "" {
		return ""
	}
	st, err := os.Stat(filepath.Join(root, ".git", "index"))
	if err != nil {
		return ""
	}
	return st.ModTime().UTC().Format("20060102T150405.000000000")
}

// Numstat counts the added and removed lines of every file in the change
// list snap describes: staged and unstaged changes with two batched
// git diff --numstat calls, and untracked text files by their line count.
// Binary files are marked, not counted as zero. Renames are reported under
// the new path. The result carries the basis snap was read from, so a
// caller can drop it once the repository has moved (Stale).
func (r *Repository) Numstat(ctx context.Context, snap Snapshot) (Numstat, error) {
	staged, err := r.areaNumstat(ctx, true)
	if err != nil {
		return Numstat{}, err
	}
	unstaged, err := r.areaNumstat(ctx, false)
	if err != nil {
		return Numstat{}, err
	}
	n := Numstat{Basis: basis(snap)}
	for _, ch := range snap.Staged {
		f := FileStat{Path: ch.Path, OrigPath: ch.OrigPath, Area: Staged}
		apply(&f, staged[ch.Path])
		n.Files = append(n.Files, f)
		if !f.Binary {
			n.StagedAdded += f.Added
			n.StagedRemoved += f.Removed
		}
	}
	for _, ch := range snap.Unstaged {
		f := FileStat{Path: ch.Path, OrigPath: ch.OrigPath, Area: Unstaged}
		apply(&f, unstaged[ch.Path])
		n.Files = append(n.Files, f)
	}
	for _, ch := range snap.Untracked {
		f, err := r.untrackedStat(ch.Path)
		if err != nil {
			return Numstat{}, err
		}
		n.Files = append(n.Files, f)
	}
	return n, nil
}

// areaNumstat runs one git diff --numstat for the staged (--cached) or the
// unstaged side and returns the counts keyed by current path.
func (r *Repository) areaNumstat(ctx context.Context, cached bool) (map[string]git.Numstat, error) {
	args := []string{"--literal-pathspecs", "diff"}
	if cached {
		args = append(args, "--cached")
	}
	args = append(args, git.NumstatArgs...)
	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	stats, err := git.ParseNumstat(out)
	if err != nil {
		return nil, fail(CodeGitFailed, err)
	}
	byPath := make(map[string]git.Numstat, len(stats))
	for _, s := range stats {
		byPath[s.Path] = s
	}
	return byPath, nil
}

// apply copies a Git numstat onto a FileStat, leaving zero counts when the
// path produced no diff line (as a pure mode change does).
func apply(f *FileStat, s git.Numstat) {
	f.Binary = s.Binary
	if !s.Binary {
		f.Added, f.Removed = s.Added, s.Removed
	}
}

// untrackedStat counts an untracked file: a text file's whole line count as
// added, a binary one marked Binary. A directory entry (a nested, untracked
// repository) is reported as Binary, with no lines to count.
func (r *Repository) untrackedStat(path string) (FileStat, error) {
	f := FileStat{Path: path, Area: Untracked}
	full := filepath.Join(r.root, path)
	data, err := os.ReadFile(full)
	if err != nil {
		if st, serr := os.Stat(full); serr == nil && st.IsDir() {
			f.Binary = true
			return f, nil
		}
		return FileStat{}, fail(CodeGitFailed, err)
	}
	if isBinary(data) {
		f.Binary = true
		return f, nil
	}
	f.Added = countLines(data)
	return f, nil
}

// isBinary reports content Git would treat as binary: a NUL byte in the
// first 8000 bytes, as git diff's own heuristic does.
func isBinary(data []byte) bool {
	n := min(len(data), 8000)
	return bytes.IndexByte(data[:n], 0) >= 0
}

// countLines counts the lines a diff would add for new content: one per
// line separator, plus one for a final line without a trailing newline.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}
