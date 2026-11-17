package workflow

import (
	"reflect"
	"testing"
)

// gc builds a GraphCommit; parents are variadic for brevity.
func gc(hash string, parents ...string) GraphCommit {
	return GraphCommit{Hash: hash, Parents: parents}
}

// nodes returns the node lane of every row, for compact comparison.
func nodes(rows []GraphRow) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.Node
	}
	return out
}

// edge finds the edge from->to in a row, or fails.
func edge(t *testing.T, r GraphRow, from, to int) GraphEdge {
	t.Helper()
	for _, e := range r.Edges {
		if e.From == from && e.To == to {
			return e
		}
	}
	t.Fatalf("no edge %d->%d in %+v", from, to, r.Edges)
	return GraphEdge{}
}

func TestLaneLinear(t *testing.T) {
	// c -> b -> a, single lane 0 throughout.
	commits := []GraphCommit{gc("c", "b"), gc("b", "a"), gc("a")}
	page := LayOutLanes(commits, LaneState{}, false)
	if len(page.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(page.Rows))
	}
	for i, r := range page.Rows {
		if r.Node != 0 {
			t.Fatalf("row %d node lane = %d, want 0", i, r.Node)
		}
	}
	// Rows 0 and 1 each have one edge down lane 0 to lane 0.
	for i := 0; i < 2; i++ {
		es := page.Rows[i].Edges
		if len(es) != 1 || es[0].From != 0 || es[0].To != 0 || es[0].Kind != EdgeNormal {
			t.Fatalf("row %d edges = %+v, want one normal 0->0", i, es)
		}
	}
	if len(page.Rows[2].Edges) != 0 {
		t.Fatalf("last row edges = %+v, want none", page.Rows[2].Edges)
	}
	if !page.End.Empty() {
		t.Fatalf("end state = %+v, want empty (all parents resolved)", page.End)
	}
}

func TestLaneForkMerge(t *testing.T) {
	// m merges mainline a-side and feature b-side:
	//   m -> (a1, b1); a1 -> base; b1 -> base; base (root)
	// Order children-first: m, a1, b1, base.
	commits := []GraphCommit{
		gc("m", "a1", "b1"),
		gc("a1", "base"),
		gc("b1", "base"),
		gc("base"),
	}
	page := LayOutLanes(commits, LaneState{}, false)
	// m in lane 0; a1 inherits lane 0; b1 took the merge lane 1.
	if got := nodes(page.Rows); !reflect.DeepEqual(got, []int{0, 0, 1, 0}) {
		t.Fatalf("node lanes = %v, want [0 0 1 0]", got)
	}
	// m row: first-parent edge 0->0 normal, merge edge 0->1 merge.
	if e := edge(t, page.Rows[0], 0, 0); e.Kind != EdgeNormal {
		t.Fatalf("m first-parent kind = %v, want normal", e.Kind)
	}
	if e := edge(t, page.Rows[0], 0, 1); e.Kind != EdgeMerge {
		t.Fatalf("m merge kind = %v, want merge", e.Kind)
	}
	// b1 row (lane 1) merges back to base in lane 0: edge 1->0.
	edge(t, page.Rows[2], 1, 0)
	if !page.End.Empty() {
		t.Fatalf("end = %+v, want empty", page.End)
	}
}

func TestLaneMergeKeepsMainlineLane0(t *testing.T) {
	// A merge M onto the mainline, where the side commit S1 is emitted
	// before the mainline parent P1 (topo-order often does). The mainline
	// must stay in lane 0 the whole way down: when P1 (lane 0) and the side
	// lane 1 both wait for the shared base, lane 0 is kept straight and lane
	// 1 bends into it.
	//   M -> (P1, S1); S1 -> base; P1 -> base; base (root)
	commits := []GraphCommit{
		gc("M", "P1", "S1"),
		gc("S1", "base"),
		gc("P1", "base"),
		gc("base"),
	}
	page := LayOutLanes(commits, LaneState{}, false)
	// Every mainline node (M, P1, base) is lane 0; only the side S1 is lane 1.
	if got := nodes(page.Rows); !reflect.DeepEqual(got, []int{0, 1, 0, 0}) {
		t.Fatalf("node lanes = %v, want [0 1 0 0] (mainline straight on lane 0)", got)
	}
	// The row that lays out P1 (row 2, lane 0) keeps lane 0 straight (0->0)
	// and bends the side lane 1 into it (1->0), not the other way round.
	if e := edge(t, page.Rows[2], 0, 0); e.Kind != EdgeNormal {
		t.Fatalf("P1 mainline kind = %v, want normal straight 0->0", e.Kind)
	}
	edge(t, page.Rows[2], 1, 0) // the side lane merges back into the mainline
	// base is the single remaining node on lane 0.
	if page.Rows[3].Node != 0 {
		t.Fatalf("base node lane = %d, want 0", page.Rows[3].Node)
	}
	if !page.End.Empty() {
		t.Fatalf("end = %+v, want empty", page.End)
	}
}

func TestLaneOctopus(t *testing.T) {
	// o has three parents p1,p2,p3; each is a root for brevity.
	commits := []GraphCommit{
		gc("o", "p1", "p2", "p3"),
		gc("p1"), gc("p2"), gc("p3"),
	}
	page := LayOutLanes(commits, LaneState{}, false)
	// o lane 0; p1 inherits 0; p2 lane 1; p3 lane 2.
	if got := nodes(page.Rows); !reflect.DeepEqual(got, []int{0, 0, 1, 2}) {
		t.Fatalf("node lanes = %v, want [0 0 1 2]", got)
	}
	// o row has three outgoing edges: 0->0 normal, 0->1 and 0->2 merge.
	r := page.Rows[0]
	if len(r.Edges) != 3 {
		t.Fatalf("o edges = %+v, want 3", r.Edges)
	}
	if edge(t, r, 0, 0).Kind != EdgeNormal {
		t.Fatalf("octopus first parent not normal")
	}
	if edge(t, r, 0, 1).Kind != EdgeMerge || edge(t, r, 0, 2).Kind != EdgeMerge {
		t.Fatalf("octopus extra parents not merge")
	}
}

func TestLaneOffPageParent(t *testing.T) {
	// Only c and b are on the page; b's parent "a" is off-page. The lane
	// must stay open (End not empty) and b is not treated as a root.
	commits := []GraphCommit{gc("c", "b"), gc("b", "a")}
	page := LayOutLanes(commits, LaneState{}, false)
	if page.End.Empty() {
		t.Fatalf("end = %+v, want a lane still waiting for a", page.End)
	}
	if page.End.Waiting[0] != "a" {
		t.Fatalf("end lane 0 = %q, want \"a\"", page.End.Waiting[0])
	}
	// b's last row still emits a down edge keeping lane 0 alive.
	edge(t, page.Rows[1], 0, 0)
}

func TestLaneContinuedFromState(t *testing.T) {
	// Previous page ended waiting for "a" in lane 0; this page delivers it.
	start := LaneState{Waiting: []string{"a"}}
	commits := []GraphCommit{gc("a", "z")}
	page := LayOutLanes(commits, start, false)
	if page.Rows[0].Node != 0 {
		t.Fatalf("a node lane = %d, want 0 (continued)", page.Rows[0].Node)
	}
	if page.End.Waiting[0] != "z" {
		t.Fatalf("end lane 0 = %q, want \"z\"", page.End.Waiting[0])
	}
}

func TestLaneElidedSearch(t *testing.T) {
	// Visible commits x then y, but y is not x's recorded parent (hidden
	// commits between). With elide, x's parent edge is EdgeElided.
	commits := []GraphCommit{gc("x", "hidden"), gc("y", "w")}
	page := LayOutLanes(commits, LaneState{}, true)
	if e := edge(t, page.Rows[0], 0, 0); e.Kind != EdgeElided {
		t.Fatalf("x->y kind = %v, want elided", e.Kind)
	}
	// When the next commit IS the parent, the edge stays normal.
	commits2 := []GraphCommit{gc("x", "y"), gc("y")}
	page2 := LayOutLanes(commits2, LaneState{}, true)
	if e := edge(t, page2.Rows[0], 0, 0); e.Kind != EdgeNormal {
		t.Fatalf("direct parent kind = %v, want normal", e.Kind)
	}
}

func TestLaneReuse(t *testing.T) {
	// m1 opens lane 1 (merge of f1, a root) which closes at row 2; then m2
	// opens a second side branch and should reuse the freed lane 1.
	//   m1 -> (base, f1); f1 (root); base -> m2side? keep simple:
	//   m1 -> base, f1 ; f1 root ; base -> g1 (first parent) + g2 (merge)
	commits := []GraphCommit{
		gc("m1", "base", "f1"),
		gc("f1"),
		gc("base", "tail", "g1"),
		gc("g1"),
		gc("tail"),
	}
	page := LayOutLanes(commits, LaneState{}, false)
	// f1 is in lane 1 and closes it (root). base's merge parent g1 should
	// reuse lane 1 rather than growing to lane 2.
	if got := page.Rows[3].Node; got != 1 {
		t.Fatalf("g1 node lane = %d, want reused lane 1", got)
	}
	// At no point should a lane index reach 2.
	for i, r := range page.Rows {
		if r.Node > 1 {
			t.Fatalf("row %d node lane %d exceeds reused max 1", i, r.Node)
		}
	}
}

func TestLaneTwoPageMatchesWhole(t *testing.T) {
	// A fork/merge spanning a page boundary must lay out identically whether
	// drawn whole or in two pages stitched by LaneState.
	commits := []GraphCommit{
		gc("m", "a1", "b1"),
		gc("a1", "a2"),
		gc("b1", "base"),
		gc("a2", "base"),
		gc("base"),
	}
	whole := LayOutLanes(commits, LaneState{}, false)

	p1 := LayOutLanes(commits[:2], LaneState{}, false)
	p2 := LayOutLanes(commits[2:], p1.End, false)
	stitched := append(append([]GraphRow{}, p1.Rows...), p2.Rows...)

	if !reflect.DeepEqual(nodes(stitched), nodes(whole.Rows)) {
		t.Fatalf("stitched node lanes %v != whole %v",
			nodes(stitched), nodes(whole.Rows))
	}
	for i := range stitched {
		if !reflect.DeepEqual(stitched[i].Edges, whole.Rows[i].Edges) {
			t.Fatalf("row %d edges differ: paged %+v whole %+v",
				i, stitched[i].Edges, whole.Rows[i].Edges)
		}
	}
	if !reflect.DeepEqual(p2.End, whole.End) {
		t.Fatalf("end differs: paged %+v whole %+v", p2.End, whole.End)
	}
}
