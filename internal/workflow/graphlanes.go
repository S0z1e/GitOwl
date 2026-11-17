package workflow

// Graph lane layout for the history list.
//
// The algorithm assigns each commit a horizontal lane and reports the
// straight lines passing through each row plus the connections from one row
// to the next, so the UI can draw a graph beside the commit list without any
// git calls. It works a page at a time: LaneState carries the open lanes from
// the previous page so a long history drawn in pages matches one drawn whole.
//
// Model. A lane is "open" while it waits for a particular commit hash to
// appear (the hash of a parent some earlier commit referred to). Lanes are
// identified by their index; a released index is reused by a later branch so
// lanes stay as narrow as possible. The commits arrive in topological order,
// children before parents, as `git log` emits them.

// GraphCommit is one commit's identity for layout: its hash and the hashes of
// its parents, first parent first. Nothing else is needed to place it.
//
// Elided, when set, has one entry per parent: true marks a parent link that
// stands in for hidden commits between this commit and that parent (a search
// result whose true parent is not shown, so Parents already holds the next
// visible ancestor). A nil Elided (the common case, a continuous history)
// leaves LayOutLanes to decide elision from whether the parent is the very
// next commit on the page.
type GraphCommit struct {
	Hash    string
	Parents []string
	Elided  []bool
}

// EdgeKind distinguishes the connections drawn from a row to the next.
type EdgeKind uint8

const (
	// EdgeNormal is an ordinary parent link or a line passing straight
	// through a row without touching its node.
	EdgeNormal EdgeKind = iota
	// EdgeMerge is a link to a second-or-later parent of a merge commit.
	EdgeMerge
	// EdgeElided stands in for hidden commits between a visible commit and
	// its next visible ancestor in search results; no direct parent link is
	// drawn for such a pair.
	EdgeElided
)

// GraphEdge is a connection from a lane in this row to a lane in the next
// row. From == To is a vertical segment; From != To bends between lanes.
type GraphEdge struct {
	From int
	To   int
	Kind EdgeKind
}

// GraphRow is the layout of one commit row: the lane its node sits in, the
// lanes of lines passing straight through the row (not the node's own), and
// the edges leaving the row toward the next.
type GraphRow struct {
	Node    int
	Through []int
	Edges   []GraphEdge
}

// LaneState is the set of open lanes carried between pages. Each entry is the
// commit hash a lane is waiting for, or "" for a free (reusable) slot.
type LaneState struct {
	Waiting []string
}

// Empty reports whether no lane is open.
func (s LaneState) Empty() bool {
	for _, w := range s.Waiting {
		if w != "" {
			return false
		}
	}
	return true
}

// clone returns a deep copy so callers can keep a page's end state unchanged.
func (s LaneState) clone() LaneState {
	w := make([]string, len(s.Waiting))
	copy(w, s.Waiting)
	return LaneState{Waiting: w}
}

// GraphPage is the layout of one page plus the state to continue with.
type GraphPage struct {
	Rows []GraphRow
	End  LaneState
}

// LayOutLanes lays out commits (topological, children first) starting from
// the open lanes in start. When elide is true, a commit's parent edge is
// drawn as EdgeElided whenever that parent is not the very next commit in the
// page: in search results the intervening commits are hidden, so the link
// stands for "some ancestor", not a direct parent. With elide false, edges to
// parents outside the page keep their lane open to the page bottom (recorded
// in End) rather than being treated as a root.
func LayOutLanes(commits []GraphCommit, start LaneState, elide bool) GraphPage {
	st := start.clone()
	var next *string // when eliding, the hash of the following commit
	rows := make([]GraphRow, 0, len(commits))
	for i := range commits {
		c := commits[i]
		if elide && i+1 < len(commits) {
			h := commits[i+1].Hash
			next = &h
		} else {
			next = nil
		}
		row := layRow(&st, c, elide, next)
		rows = append(rows, row)
	}
	trimTail(&st)
	return GraphPage{Rows: rows, End: st}
}

// layRow places one commit, returning its row and advancing the lane state so
// it describes the lanes entering the next row. The returned edges describe
// how the current lanes connect down to those next lanes.
func layRow(st *LaneState, c GraphCommit, elide bool, next *string) GraphRow {
	// The node sits in the first lane already waiting for this hash; if none
	// waits (a tip, or the first page), it claims a fresh lane.
	node := findOrClaim(st, c.Hash)
	// Every other lane waiting for this same hash also collapses onto node;
	// free those slots so later branches can reuse them.
	collapseDuplicates(st, c.Hash, node)

	through := passThrough(st, node)
	edges := assignParents(st, c, node, elide, next)
	// A lane that bends into the node this row is released by assignParents.
	// It must not pass straight through: its line ends at the node, not at
	// the row's bottom edge, where it would stick out as a stub.
	open := through[:0]
	for _, l := range through {
		if st.Waiting[l] != "" {
			open = append(open, l)
		}
	}
	return GraphRow{Node: node, Through: open, Edges: edges}
}

// findOrClaim returns the index of the first lane waiting for hash, or claims
// a free slot (reusing a released one before growing) and returns it.
func findOrClaim(st *LaneState, hash string) int {
	for i, w := range st.Waiting {
		if w == hash {
			return i
		}
	}
	for i, w := range st.Waiting {
		if w == "" {
			return i
		}
	}
	st.Waiting = append(st.Waiting, "")
	return len(st.Waiting) - 1
}

// collapseDuplicates frees any lane other than keep that was also waiting for
// hash, so duplicate waits merge onto the single node lane.
func collapseDuplicates(st *LaneState, hash string, keep int) {
	for i, w := range st.Waiting {
		if i != keep && w == hash {
			st.Waiting[i] = ""
		}
	}
}

// passThrough lists lanes, other than node, that currently carry an open line
// (they pass straight through this row without touching the node).
func passThrough(st *LaneState, node int) []int {
	var out []int
	for i, w := range st.Waiting {
		if i != node && w != "" {
			out = append(out, i)
		}
	}
	return out
}

// assignParents updates the lane state to carry c's parents and returns the
// edges from this row to the next. The first parent continues in the lower of
// the node's lane and any other open lane already waiting for that same
// parent: the smaller-numbered lane is kept straight and the larger one bends
// into it and is released, so the mainline (lane 0) stays a straight column
// and a side branch merges back into it. Each further parent merges into an
// existing waiting lane or a freshly claimed one. A parentless commit (a
// root, or a tip whose history is cut off) releases its lane. Lanes that pass
// straight through emit a vertical EdgeNormal.
func assignParents(st *LaneState, c GraphCommit, node int, elide bool, next *string) []GraphEdge {
	// kind reports a parent edge's kind. When the commit carries explicit
	// Elided markers (a search that already resolved each parent to its next
	// visible ancestor), they decide; otherwise, while eliding, a parent that
	// is not the very next commit on the page stands in for hidden commits.
	kind := func(pi int, parent string) EdgeKind {
		if pi < len(c.Elided) {
			if c.Elided[pi] {
				return EdgeElided
			}
			return EdgeNormal
		}
		if elide && next != nil && parent != *next {
			return EdgeElided
		}
		return EdgeNormal
	}
	if len(c.Parents) == 0 {
		st.Waiting[node] = "" // release; nothing continues this lane
	}
	// bends records other open lanes that must bend into the node's lane this
	// row (the node kept a lower-numbered lane than one already waiting for
	// the same first parent), by lane index to its edge kind.
	bends := map[int]EdgeKind{}
	var edges []GraphEdge
	for pi, p := range c.Parents {
		existing := laneWaiting(st, p, node)
		var to int
		switch {
		case pi == 0 && existing < 0:
			st.Waiting[node] = p // first parent keeps the node's lane
			to = node
		case pi == 0 && existing < node:
			to = existing         // a lower lane already carries this parent
			st.Waiting[node] = "" // bend the node's lane into it and release
		case pi == 0:
			// The node's lane is lower than the one that was waiting: keep
			// the node's lane straight and bend the higher lane into it.
			st.Waiting[node] = p
			st.Waiting[existing] = ""
			bends[existing] = kind(pi, p)
			to = node
		case existing >= 0:
			to = existing // later parent already has an open lane
		default:
			to = findOrClaim(st, p) // new side lane for this parent
			st.Waiting[to] = p
		}
		k := kind(pi, p)
		if pi > 0 && k == EdgeNormal {
			k = EdgeMerge
		}
		edges = append(edges, GraphEdge{From: node, To: to, Kind: k})
	}
	// Lines in other open lanes pass straight down; a lane recorded in bends
	// instead bends into the node's lane. Skip any lane an edge already lands
	// on this row.
	for i, w := range st.Waiting {
		if i == node || touched(edges, i) {
			continue
		}
		if k, ok := bends[i]; ok {
			edges = append(edges, GraphEdge{From: i, To: node, Kind: k})
			continue
		}
		if w == "" {
			continue
		}
		edges = append(edges, GraphEdge{From: i, To: i, Kind: EdgeNormal})
	}
	return edges
}

// laneWaiting returns the lowest lane index (other than skip) already waiting
// for hash, or -1 if none.
func laneWaiting(st *LaneState, hash string, skip int) int {
	for i, w := range st.Waiting {
		if i != skip && w == hash {
			return i
		}
	}
	return -1
}

// touched reports whether any edge already lands on lane to.
func touched(es []GraphEdge, to int) bool {
	for _, e := range es {
		if e.To == to {
			return true
		}
	}
	return false
}

// trimTail drops trailing free slots so the end state is minimal; interior
// free slots stay, holding their index for reuse within a page continuation.
func trimTail(st *LaneState) {
	n := len(st.Waiting)
	for n > 0 && st.Waiting[n-1] == "" {
		n--
	}
	st.Waiting = st.Waiting[:n]
}

// oldestFirst lays out a run of commits, given newest first (children before
// parents), for listing oldest first. Read from the other end, a commit's
// children are its parents: the reversed run lists each parent before its
// child, so LayOutLanes, given the children as the parents, draws the same
// lines from the other direction. An elided marker stays with its edge. The
// rows come back in the reversed order, one per commit.
func oldestFirst(gcs []GraphCommit) []GraphRow {
	kids := map[string][]string{}
	kidElided := map[string][]bool{}
	for _, g := range gcs {
		for i, p := range g.Parents {
			kids[p] = append(kids[p], g.Hash)
			kidElided[p] = append(kidElided[p], i < len(g.Elided) && g.Elided[i])
		}
	}
	rev := make([]GraphCommit, len(gcs))
	for i, g := range gcs {
		rev[len(gcs)-1-i] = GraphCommit{Hash: g.Hash, Parents: kids[g.Hash], Elided: kidElided[g.Hash]}
	}
	return LayOutLanes(rev, LaneState{}, false).Rows
}
