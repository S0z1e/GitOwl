package workflow

import (
	"context"
	"strconv"
	"strings"

	"gitowl/internal/git"
)

// unlimitedCommits is a commit count no history reaches. Oldest first reads
// the whole list at once, because its first row is the last commit git lists.
const unlimitedCommits = 1 << 30

// GraphClass marks how a commit of the graph relates to the branch and its
// upstream.
type GraphClass uint8

const (
	// Shared is reachable from both the branch and its upstream, or from
	// the single ref of a scope without an upstream.
	Shared GraphClass = iota
	// Unpushed is reachable only from the local branch: a commit the
	// upstream lacks. Never marked when the branch has no upstream.
	Unpushed
	// Incoming is reachable only from the upstream: a commit the local
	// branch lacks.
	Incoming
)

// SortKey is the date a history list orders its commits by.
type SortKey uint8

const (
	// SortCommitDate orders by commit date, children before parents.
	SortCommitDate SortKey = iota
	// SortAuthorDate orders by author date, children before parents.
	SortAuthorDate
	// SortTopo orders topologically, so one branch's commits are not
	// interleaved with another's by date.
	SortTopo
)

// HistoryOrder is how a history list is ordered: by Key, newest first, or
// oldest first when Oldest is set.
type HistoryOrder struct {
	Key    SortKey
	Oldest bool
}

// flag returns the git log option that orders commits by Key.
func (o HistoryOrder) flag() string {
	switch o.Key {
	case SortAuthorDate:
		return "--author-date-order"
	case SortTopo:
		return "--topo-order"
	}
	return "--date-order"
}

// GraphScope is the line of history a graph reads: a local branch (with its
// upstream, when one is set and last fetched) or a remote-tracking branch on
// its own.
type GraphScope struct {
	// Name is the short branch name: main, or origin/main for a remote
	// scope.
	Name string
	// Remote selects refs/remotes over refs/heads.
	Remote bool
}

// ref returns the full ref name the scope names.
func (s GraphScope) ref() string {
	if s.Remote {
		return "refs/remotes/" + s.Name
	}
	return "refs/heads/" + s.Name
}

// history returns the history scope of the branch the graph scope names, or
// HEAD for an empty name (a detached HEAD).
func (s GraphScope) history() HistoryScope {
	if s.Name == "" {
		return HistoryScope{}
	}
	return HistoryScope{Ref: s.ref(), Name: s.Name, Remote: s.Remote}
}

// ResolvedGraph fixes a graph view to the commits it pointed at, so pages stay
// consistent while refs move. Tips are the shown branches' commits the graph
// walks. Scope is the checked-out branch; Branch and Upstream are its tip and
// its upstream's tip ("" when there is none), the basis of the Unpushed and
// Incoming classes.
type ResolvedGraph struct {
	Scope    GraphScope
	Branch   string
	Upstream string
	Tips     []string
	// Refs are the names of the shown branches (HEAD for the checked-out
	// one), as the walk names the commits it reaches from each.
	Refs  []string
	Order HistoryOrder
	// unpushed and incoming hold the commit hashes only one side reaches,
	// computed once from the fixed tips so classification never moves.
	unpushed map[string]bool
	incoming map[string]bool
}

// ResolveGraph fixes the graph of one branch: the branch and, when it has
// one, its upstream, both newest first. A missing branch is CodeScopeMissing.
func (r *Repository) ResolveGraph(ctx context.Context, scope GraphScope) (ResolvedGraph, error) {
	shown := []HistoryScope{scope.history()}
	if !scope.Remote {
		b, err := r.Branches(ctx)
		if err != nil {
			return ResolvedGraph{}, err
		}
		if br, ok := b.local(scope.Name); ok && br.Upstream != "" && !br.Gone {
			shown = append(shown, HistoryScope{Ref: "refs/remotes/" + br.Upstream, Name: br.Upstream, Remote: true})
		}
	}
	return r.ResolveGraphView(ctx, scope, shown, HistoryOrder{})
}

// ResolveGraphView fixes a graph of the shown branches, HEAD when none is
// shown, ordered by order. primary is the checked-out branch: Unpushed and
// Incoming are judged against its upstream, and nothing is classified when it
// is detached or has no tip. A shown branch that no longer exists is
// CodeScopeMissing.
func (r *Repository) ResolveGraphView(ctx context.Context, primary GraphScope, shown []HistoryScope, order HistoryOrder) (ResolvedGraph, error) {
	rg := ResolvedGraph{Scope: primary, Order: order}
	tips, err := r.resolveScopes(ctx, shown)
	if err != nil {
		return rg, err
	}
	rg.Tips = tips
	for _, s := range shown {
		if s.Ref == "" {
			rg.Refs = append(rg.Refs, "HEAD")
		} else {
			rg.Refs = append(rg.Refs, s.Ref)
		}
	}
	if primary.Name == "" {
		return rg, nil
	}
	tip, err := r.verifyRef(ctx, primary.ref())
	if err != nil {
		return rg, err
	}
	rg.Branch = tip
	if primary.Remote || tip == "" {
		return rg, nil
	}
	if rg.Upstream, err = r.scopeUpstream(ctx, primary.Name); err != nil {
		return rg, err
	}
	if rg.Upstream != "" {
		if rg.unpushed, err = r.onlySide(ctx, rg.Branch, rg.Upstream); err != nil {
			return rg, err
		}
		if rg.incoming, err = r.onlySide(ctx, rg.Upstream, rg.Branch); err != nil {
			return rg, err
		}
	}
	return rg, nil
}

// verifyRef returns the commit a ref points at, or "" when it does not
// exist.
func (r *Repository) verifyRef(ctx context.Context, ref string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		if CodeOf(err) == CodeGitMissing || CodeOf(err) == CodeCancelled {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// scopeUpstream returns the commit the branch's upstream points at, or "" for
// a branch without an upstream (or one gone locally).
func (r *Repository) scopeUpstream(ctx context.Context, name string) (string, error) {
	b, err := r.Branches(ctx)
	if err != nil {
		return "", err
	}
	br, ok := b.local(name)
	if !ok {
		return "", &Error{Code: CodeScopeMissing, Detail: name}
	}
	if br.Upstream == "" || br.Gone {
		return "", nil
	}
	return r.verifyRef(ctx, "refs/remotes/"+br.Upstream)
}

// onlySide returns the commits reachable from a but not from b, as
// git rev-list a ^b lists them.
func (r *Repository) onlySide(ctx context.Context, a, b string) (map[string]bool, error) {
	out, err := r.run(ctx, "rev-list", a, "^"+b, "--")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for line := range strings.Lines(string(out)) {
		if h := strings.TrimSpace(line); h != "" {
			set[h] = true
		}
	}
	return set, nil
}

// GraphEntry is one commit of a graph page: its details, how it relates to
// the branch and upstream, and the refs pointing straight at it.
type GraphEntry struct {
	CommitInfo
	Class GraphClass
	// Refs are labels on this commit: local branches, remote-tracking
	// branches and "HEAD".
	Refs []string
	// Source is the ref the walk reached this commit from, as git log
	// --source names it: the branch a commit belongs to in the graph.
	Source string
}

// GraphResult is one page of a graph: the commits, the lane layout rows for
// them (one per commit, in order) and the state to continue with. Cursor is
// the number of commits before the next page; More reports that it has any.
type GraphResult struct {
	Commits []GraphEntry
	Rows    []GraphRow
	Lanes   LaneState
	Cursor  int
	More    bool
}

// Graph reads up to limit commits of rg after skipping skip, in the order
// rg.Order gives, from the fixed tips. start carries the open lanes from the
// previous page (the zero value for the first). It classifies each commit and
// lays out its lane row so pages join seamlessly. Oldest first reads the
// whole graph in one page, so only skip zero yields commits.
func (r *Repository) Graph(ctx context.Context, rg ResolvedGraph, skip, limit int, start LaneState) (GraphResult, error) {
	if len(rg.Tips) == 0 {
		return GraphResult{}, nil
	}
	if rg.Order.Oldest {
		if skip > 0 {
			return GraphResult{}, nil
		}
		limit = unlimitedCommits
	}
	args := []string{"log", "-z", "--no-color", "--no-notes", "--parents", rg.Order.flag(),
		"--format=" + git.LogFormat, "--skip=" + strconv.Itoa(skip), "--max-count=" + strconv.Itoa(limit+1)}
	args = append(args, rg.Tips...)
	out, err := r.run(ctx, append(args, "--")...)
	if err != nil {
		return GraphResult{}, err
	}
	commits, more, err := pageOf(out, limit)
	if err != nil {
		return GraphResult{}, err
	}
	labels, err := r.refLabels(ctx)
	if err != nil {
		return GraphResult{}, err
	}
	sources, err := r.commitSources(ctx, rg.Refs)
	if err != nil {
		return GraphResult{}, err
	}
	res := GraphResult{More: more, Cursor: skip + len(commits)}
	gcs := make([]GraphCommit, len(commits))
	for i, c := range commits {
		res.Commits = append(res.Commits, GraphEntry{CommitInfo: c, Class: rg.classify(c.Hash), Refs: labels[c.Hash], Source: sources[c.Hash]})
		gcs[i] = GraphCommit{Hash: c.Hash, Parents: c.Parents}
	}
	if rg.Order.Oldest {
		reverseEntries(res.Commits)
		res.Rows = oldestFirst(gcs)
		return res, nil
	}
	page := LayOutLanes(gcs, start, false)
	res.Rows, res.Lanes = page.Rows, page.End
	return res, nil
}

// commitSources maps each commit reachable from refs to the first of them the
// walk reaches it from, as git log --source reports it. Refs are the names the
// walk starts from; "HEAD" is the checked-out branch.
func (r *Repository) commitSources(ctx context.Context, refs []string) (map[string]string, error) {
	sources := map[string]string{}
	if len(refs) == 0 {
		return sources, nil
	}
	args := append([]string{"log", "--source", "--no-color", "--no-notes", "--format=%H%x1f%S"}, refs...)
	out, err := r.run(ctx, append(args, "--")...)
	if err != nil {
		return nil, err
	}
	for line := range strings.Lines(string(out)) {
		if f := strings.SplitN(strings.TrimSuffix(line, "\n"), "\x1f", 2); len(f) == 2 {
			sources[f[0]] = f[1]
		}
	}
	return sources, nil
}

// reverseEntries flips a list in place.
func reverseEntries(es []GraphEntry) {
	for i, j := 0, len(es)-1; i < j; i, j = i+1, j-1 {
		es[i], es[j] = es[j], es[i]
	}
}

// classify marks a commit from the fixed only-one-side sets. Without an
// upstream nothing is Unpushed.
func (rg ResolvedGraph) classify(hash string) GraphClass {
	switch {
	case rg.unpushed[hash]:
		return Unpushed
	case rg.incoming[hash]:
		return Incoming
	default:
		return Shared
	}
}

// refLabels maps each commit to the branches, remote-tracking branches and
// HEAD that point straight at it.
func (r *Repository) refLabels(ctx context.Context) (map[string][]string, error) {
	out, err := r.run(ctx, "for-each-ref", "--format=%(objectname)%00%(refname:short)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	labels := map[string][]string{}
	for line := range strings.Lines(string(out)) {
		f := strings.Split(strings.TrimSuffix(line, "\n"), "\x00")
		if len(f) != 3 || f[2] != "" || f[0] == "" {
			continue // malformed or a symbolic ref such as origin/HEAD
		}
		labels[f[0]] = append(labels[f[0]], f[1])
	}
	if head, err := r.verifyRef(ctx, "HEAD"); err == nil && head != "" {
		labels[head] = append(labels[head], "HEAD")
	}
	return labels, nil
}
