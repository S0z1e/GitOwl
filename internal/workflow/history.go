package workflow

import (
	"context"
	"strings"
	"time"

	"gitowl/internal/git"
)

// InCommit is the area of a file changed by a commit in history.
const InCommit Area = 3

// CommitInfo is one commit of the history list.
type CommitInfo struct {
	Hash    string
	Parents []string
	Author  string
	Email   string
	When    time.Time
	Subject string
}

// Short returns the abbreviated hash.
func (c CommitInfo) Short() string { return short(c.Hash) }

func short(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// Base is what a commit is compared with.
type Base uint8

const (
	// BaseParent is the only parent.
	BaseParent Base = iota
	// BaseEmptyTree is the empty tree, for a commit without parents.
	BaseEmptyTree
	// BaseFirstParent is the first parent of a merge commit.
	BaseFirstParent
)

// CommitDetail is a commit selected in history.
type CommitDetail struct {
	CommitInfo
	// Message is the full commit message.
	Message   string
	Committer string
	// Base and BaseHash name what Files and diffs compare the commit with.
	Base     Base
	BaseHash string
	Files    []Change
}

// History returns up to limit commits reachable from HEAD after skipping
// skip, newest first, and whether more follow. A repository without commits
// has none. Pages read from a moving HEAD may overlap; SearchHistory with a
// ResolvedQuery keeps them consistent.
func (r *Repository) History(ctx context.Context, skip, limit int) ([]CommitInfo, bool, error) {
	rq, err := r.ResolveQuery(ctx, HistoryQuery{})
	if err != nil {
		return nil, false, err
	}
	return r.SearchHistory(ctx, rq, skip, limit)
}

// emptyTree returns the repository's empty tree object name, which differs
// between SHA-1 and SHA-256 repositories.
func (r *Repository) emptyTree(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "hash-object", "-t", "tree", "/dev/null")
	return strings.TrimSpace(string(out)), err
}

// Detail reads a commit's message and the files it changed against its
// base: its parent, the empty tree for a root commit, or the first parent of
// a merge.
func (r *Repository) Detail(ctx context.Context, c CommitInfo) (CommitDetail, error) {
	d := CommitDetail{CommitInfo: c}
	out, err := r.run(ctx, "show", "-s", "--no-color", "--format=%B%x00%cn", c.Hash, "--")
	if err != nil {
		return d, err
	}
	msg, committer, _ := strings.Cut(string(out), "\x00")
	d.Message, d.Committer = strings.TrimRight(msg, "\n"), strings.TrimSpace(committer)
	switch len(c.Parents) {
	case 0:
		d.Base = BaseEmptyTree
		if d.BaseHash, err = r.emptyTree(ctx); err != nil {
			return d, err
		}
	case 1:
		d.Base, d.BaseHash = BaseParent, c.Parents[0]
	default:
		d.Base, d.BaseHash = BaseFirstParent, c.Parents[0]
	}
	raw, err := r.run(ctx, "diff", "--raw", "-z", "--no-abbrev", "--find-renames", "--no-ext-diff", d.BaseHash, c.Hash, "--")
	if err != nil {
		return d, err
	}
	changes, err := git.ParseRaw(raw)
	if err != nil {
		return d, fail(CodeGitFailed, err)
	}
	for _, rc := range changes {
		d.Files = append(d.Files, Change{Path: rc.Path, OrigPath: rc.OrigPath, Kind: rawKind(rc.Status), Area: InCommit, Submodule: rc.Submodule()})
	}
	args := append([]string{"diff"}, git.NumstatArgs...)
	args = append(args, d.BaseHash, c.Hash, "--")
	statOut, err := r.run(ctx, args...)
	if err != nil {
		return d, err
	}
	stats, err := git.ParseNumstat(statOut)
	if err != nil {
		return d, fail(CodeGitFailed, err)
	}
	byPath := make(map[string]git.Numstat, len(stats))
	for _, s := range stats {
		byPath[s.Path] = s
	}
	for i := range d.Files {
		if s, ok := byPath[d.Files[i].Path]; ok {
			d.Files[i].Binary = s.Binary
			if !s.Binary {
				d.Files[i].Added, d.Files[i].Removed = s.Added, s.Removed
			}
		}
	}
	return d, nil
}

func rawKind(s byte) git.Kind {
	switch s {
	case 'A':
		return git.Added
	case 'D':
		return git.Deleted
	case 'R':
		return git.Renamed
	case 'C':
		return git.Copied
	case 'T':
		return git.TypeChanged
	}
	return git.Modified
}

// CommitFileDiff returns f's diff in commit d against d's base.
func (r *Repository) CommitFileDiff(ctx context.Context, d CommitDetail, f Change) (Diff, error) {
	out := Diff{Change: f}
	if f.Submodule {
		out.Submodule = true
		return out, nil
	}
	args := append([]string{"--literal-pathspecs", "diff"}, diffOptions...)
	args = append(args, d.BaseHash, d.Hash, "--", f.Path)
	if f.OrigPath != "" {
		args = append(args, f.OrigPath)
	}
	patch, err := r.run(ctx, args...)
	if err != nil {
		return Diff{}, err
	}
	return classify(out, patch), nil
}
