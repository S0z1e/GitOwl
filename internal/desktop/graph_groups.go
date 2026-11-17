package desktop

import (
	"hash/fnv"
	"maps"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// graphGroupCount is how many colours the palette holds. A branch group or an
// author takes one of them.
const graphGroupCount = 8

// groupIndex is the palette slot of a group key. The hash is stable, so a group
// keeps its colour from one read to the next.
func groupIndex(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() % graphGroupCount)
}

// authorIndex is the palette slot of a commit's author: one slot per email.
func authorIndex(c workflow.CommitInfo) int {
	key := strings.ToLower(c.Email)
	if key == "" {
		key = c.Author
	}
	return groupIndex("author:" + key)
}

// graphRowGroups are the palette groups a graph row draws with. A line takes
// the group of the commit it leads to, so a branch's line keeps its colour down
// to the commit it ends at. above[l] is the group of the line in lane l entering
// the row, below[l] of the line leaving it in lane l, and node the group of the
// row's commit. -1 means no group, and the lane's own colour applies.
type graphRowGroups struct {
	above []int
	below []int
	node  int
}

// lane is the colour of the line passing through or merging into lane l.
func (g graphRowGroups) lane(tk kitTokens, l int) ui.Color {
	if l < len(g.above) && g.above[l] >= 0 {
		return tk.groupColor(g.above[l])
	}
	return laneColor(tk, l)
}

// nodeColour is the colour of the row's commit, in lane.
func (g graphRowGroups) nodeColour(tk kitTokens, lane int) ui.Color {
	if g.node >= 0 {
		return tk.groupColor(g.node)
	}
	return laneColor(tk, lane)
}

// leaveColour is the colour of a line leaving the node downward toward lane to:
// the commit's group, or the destination lane's colour without a group.
func (g graphRowGroups) leaveColour(tk kitTokens, to int) ui.Color {
	if to < len(g.below) && g.below[to] >= 0 {
		return tk.groupColor(g.below[to])
	}
	if g.node >= 0 {
		return tk.groupColor(g.node)
	}
	return laneColor(tk, to)
}

// laneGroups works out each row's groups from the rows and each commit's group
// (groupOf, -1 for none). It reads the rows from the bottom up, so each lane
// carries the group of the next commit it leads to: a line runs in the colour
// of the branch it ends at, and a merging line in that of the commit it joins.
func laneGroups(rows []workflow.GraphRow, groupOf []int) []graphRowGroups {
	lanes := 0
	for _, r := range rows {
		lanes = max(lanes, r.Node+1)
		for _, l := range r.Through {
			lanes = max(lanes, l+1)
		}
		for _, e := range r.Edges {
			lanes = max(lanes, e.From+1, e.To+1)
		}
	}
	owner := make([]int, lanes)
	for i := range owner {
		owner[i] = -1
	}
	out := make([]graphRowGroups, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		g := -1
		if i < len(groupOf) {
			g = groupOf[i]
		}
		below := slices.Clone(owner)
		// Above this row, the node's lane and the lanes merging into the node
		// lead to this commit.
		owner[r.Node] = g
		for _, e := range r.Edges {
			if e.To == r.Node && e.From != r.Node {
				owner[e.From] = g
			}
		}
		out[i] = graphRowGroups{above: slices.Clone(owner), below: below, node: g}
	}
	return out
}

// branchKey is the group a ref belongs to. A local branch is its own group; a
// remote-tracking branch shares the group of the local branch it tracks, so
// origin/x and x are one group. HEAD is the checked-out branch.
func (u *UI) branchKey(ref string) string {
	if ref == "HEAD" {
		return u.snap.Branch
	}
	if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
		return name
	}
	if name, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
		return strings.TrimPrefix(name, remoteOf(name, u.sync.info.Remotes)+"/")
	}
	return ref
}

// labelColour is the palette colour of a branch label: its branch group's, the
// same as the lanes of that branch.
func (u *UI) labelColour(tk kitTokens, l commitLabel) ui.Color {
	ref := "refs/heads/" + l.name
	if l.remote {
		ref = "refs/remotes/" + l.name
	}
	return tk.groupColor(groupIndex(u.branchKey(ref)))
}

// graphGroups colours each row of a graph by the branch group its commit came
// from, as sources maps commits to refs.
func (u *UI) graphGroups(commits []workflow.CommitInfo, rows []workflow.GraphRow, sources map[string]string) []graphRowGroups {
	groupOf := make([]int, len(commits))
	byRef := map[string]int{}
	for i, c := range commits {
		groupOf[i] = -1
		ref, ok := sources[c.Hash]
		if !ok {
			continue
		}
		g, seen := byRef[ref]
		if !seen {
			g = groupIndex(u.branchKey(ref))
			byRef[ref] = g
		}
		groupOf[i] = g
	}
	return laneGroups(rows, groupOf)
}

// addSources records the refs of more commits, as a further page of a list
// adds them.
func (h *history) addSources(more map[string]string) {
	if h.sources == nil {
		h.sources = map[string]string{}
	}
	maps.Copy(h.sources, more)
}

// graphSources maps each commit of a graph page to the ref it came from.
func graphSources(g workflow.GraphResult) map[string]string {
	m := make(map[string]string, len(g.Commits))
	for _, e := range g.Commits {
		m[e.Hash] = e.Source
	}
	return m
}

// searchSources maps each result of a search page to the ref it came from.
func searchSources(g workflow.SearchGraphResult) map[string]string {
	m := make(map[string]string, len(g.Commits))
	for _, e := range g.Commits {
		m[e.Hash] = e.Source
	}
	return m
}
