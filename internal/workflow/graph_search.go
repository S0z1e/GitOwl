package workflow

import (
	"context"
	"strings"
)

// SearchGraphResult is one page of a history search drawn as a graph: the
// matching commits with their classes and refs, the lane rows laid out for
// them with elided edges standing in for commits hidden between results, and
// the state to continue paging. Cursor is the number of commits before the
// next page; More reports that it has any.
//
// Unlike Graph, which reads a continuous slice of history, a search hides the
// commits that do not match, so a visible commit's next visible ancestor is
// rarely its real parent. LayOutLanes is called with elide true so each such
// gap is an EdgeElided, never a false direct parent link.
type SearchGraphResult struct {
	Commits []GraphEntry
	Rows    []GraphRow
	Cursor  int
	More    bool
}

// SearchGraph returns up to limit matching commits of rq after skipping skip,
// laid out as a graph and classified against rg's fixed tips. rq fixes the
// search to a start commit so its pages stay consistent while refs move; rg
// fixes the Unpushed/Incoming basis to the same tips the Graph uses, so a
// commit's class is the same whether it is seen in the Graph or in a search.
//
// A search hides every commit that does not match, so a visible commit's real
// parent is rarely another result. Each result's parents are rewritten to the
// nearest visible ancestors on each parent chain (read once from the scope's
// full topology), with those rewritten links marked elided when commits were
// hidden between them. Laid out this way, two matches on one mainline keep a
// single lane joined by one dashed edge instead of splitting into two false
// branches (issue 14 #31); a parent chain with no matching ancestor keeps its
// lane open to the page bottom as an elided continuation.
func (r *Repository) SearchGraph(ctx context.Context, rq ResolvedQuery, rg ResolvedGraph, skip, limit int) (SearchGraphResult, error) {
	var commits []CommitInfo
	var more bool
	var err error
	if rq.Order.Oldest {
		// Oldest first reads the whole result in one page, as Graph does.
		if skip > 0 {
			return SearchGraphResult{}, nil
		}
		commits, more, err = r.SearchHistory(ctx, rq, 0, unlimitedCommits)
	} else {
		commits, more, err = r.SearchHistory(ctx, rq, skip, limit)
	}
	if err != nil {
		return SearchGraphResult{}, err
	}
	labels, err := r.refLabels(ctx)
	if err != nil {
		return SearchGraphResult{}, err
	}
	sources, err := r.commitSources(ctx, rq.Refs)
	if err != nil {
		return SearchGraphResult{}, err
	}
	res := SearchGraphResult{More: more, Cursor: skip + len(commits)}
	rows, err := r.layoutSearch(ctx, rq, commits)
	if err != nil {
		return SearchGraphResult{}, err
	}
	for _, c := range commits {
		res.Commits = append(res.Commits, GraphEntry{CommitInfo: c, Class: rg.classify(c.Hash), Refs: labels[c.Hash], Source: sources[c.Hash]})
	}
	if rq.Order.Oldest {
		reverseEntries(res.Commits)
	}
	res.Rows = rows
	return res, nil
}

// SearchGraphRows lays the given run of search results out as a graph page,
// resolving each result's parents to the nearest visible ancestors over the
// whole run. Load-more passes every result loaded so far so lanes and elided
// edges join across the page boundary (a result on an earlier page reaches
// one on a later page), which a page-at-a-time layout cannot see.
func (r *Repository) SearchGraphRows(ctx context.Context, rq ResolvedQuery, commits []CommitInfo) ([]GraphRow, error) {
	return r.layoutSearch(ctx, rq, commits)
}

// layoutSearch resolves commits' parents to their nearest visible ancestors
// (reading the scope topology from rq's starts once) and lays the run out with
// elide true, so hidden commits between two results become dashed edges and
// two results on one mainline keep a single joined lane (issue 14 #31). Oldest
// first returns the rows of the reversed run.
func (r *Repository) layoutSearch(ctx context.Context, rq ResolvedQuery, commits []CommitInfo) ([]GraphRow, error) {
	visible := map[string]bool{}
	for _, c := range commits {
		visible[c.Hash] = true
	}
	parents, err := r.scopeParents(ctx, rq.Starts)
	if err != nil {
		return nil, err
	}
	gcs := make([]GraphCommit, len(commits))
	for i, c := range commits {
		gcs[i] = resolveVisibleParents(c.Hash, c.Parents, visible, parents)
	}
	if rq.Order.Oldest {
		return oldestFirst(gcs), nil
	}
	return LayOutLanes(gcs, LaneState{}, true).Rows, nil
}

// scopeParents reads the whole history reachable from starts once and returns
// each commit's parent hashes, first parent first. It is used to walk past
// the hidden (non-matching) commits of a search to each result's nearest
// visible ancestor. No starts yield an empty map.
func (r *Repository) scopeParents(ctx context.Context, starts []string) (map[string][]string, error) {
	parents := map[string][]string{}
	if len(starts) == 0 {
		return parents, nil
	}
	out, err := r.run(ctx, append(append([]string{"rev-list", "--parents"}, starts...), "--")...)
	if err != nil {
		return nil, err
	}
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		parents[f[0]] = f[1:]
	}
	return parents, nil
}

// resolveVisibleParents rewrites one result's parents to the nearest visible
// ancestors on each parent chain, so the lane layout joins results through the
// commits a search hides. For each real parent it finds the closest ancestors
// that are themselves results (following the parent DAG past hidden commits);
// a direct result parent stays a normal edge, one reached across hidden
// commits an elided one. A parent chain with no visible ancestor keeps the
// real parent, marked elided, so its lane runs on to the page bottom as a
// dashed continuation. Duplicate ancestors (two chains meeting at one result)
// collapse to a single edge, first-parent order preserved so the mainline
// keeps its lane.
func resolveVisibleParents(hash string, realParents []string, visible map[string]bool, parents map[string][]string) GraphCommit {
	gc := GraphCommit{Hash: hash}
	seen := map[string]bool{}
	add := func(h string, elided bool) {
		if seen[h] {
			return
		}
		seen[h] = true
		gc.Parents = append(gc.Parents, h)
		gc.Elided = append(gc.Elided, elided)
	}
	for _, p := range realParents {
		if visible[p] {
			add(p, false)
			continue
		}
		anc := nearestVisible(p, visible, parents)
		if len(anc) == 0 {
			// No visible ancestor on this chain: keep its lane open to the
			// page bottom as a dashed continuation. A unique sentinel (the
			// child's hash joined to the parent's) keeps two results that
			// share only a hidden ancestor in separate lanes, not merged.
			add(hash+"\x00"+p, true)
			continue
		}
		for _, a := range anc {
			add(a, true)
		}
	}
	return gc
}

// nearestVisible walks the parent DAG from start (a hidden commit) breadth
// first and returns the closest visible commits: once a visible commit is
// reached on a path, that path stops, so only the nearest ancestors are
// returned, never their own ancestors. Order follows first-parent links first.
func nearestVisible(start string, visible map[string]bool, parents map[string][]string) []string {
	var out []string
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, p := range parents[cur] {
			if seen[p] {
				continue
			}
			seen[p] = true
			if visible[p] {
				out = append(out, p)
				continue // stop descending past a visible ancestor
			}
			queue = append(queue, p)
		}
	}
	return out
}
