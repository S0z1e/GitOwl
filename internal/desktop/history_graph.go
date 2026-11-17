package desktop

import (
	"context"
	"math"

	"github.com/egoist/mygo/ui"

	"gitowl/internal/workflow"
)

// The design's History list draws a Graph column at the left of every commit
// row (history.md §3.2, §4): 64 DIPs wide, the full height of the 56-DIP
// commit row, with lanes 16 DIPs apart, a node at the row's vertical middle
// and lines joining a commit to its parents. This ticket draws that Graph
// from the real commit topology the workflow lays out (ticket 13).

const (
	// graphSlotWidth is the Graph column's width, the 64 the design's commit
	// button reserves with its left padding (history.md §4).
	graphSlotWidth = 54
	// commitRowHeight is the fixed height of a commit row, the repository
	// design's 30 (history.md §5, the History board). A node sits at its
	// vertical middle, graphNodeCy.
	commitRowHeight = 30
	// graphNodeCy is a node's centre from the top of its row: 15, the row's
	// middle.
	graphNodeCy = 15
	// graphLaneX0 is the first lane's x and graphLaneStep the gap to the
	// next, so lane i sits at graphLaneX0 + graphLaneStep*i = 14 + 16·i
	// (history.md §4.1).
	graphLaneX0   = 14
	graphLaneStep = 16
	// graphNodeR is a node's radius (r=5, diameter 10) and graphLineWidth
	// the stroke width of lines and node outlines (2), per history.md §4.
	graphNodeR     = 4.5
	graphLineWidth = 2
	// graphCtrlDy is the control-point offset a fork or merge curve uses to
	// leave and rejoin a lane vertically before bending (history.md §4.2:
	// 196→214, 308→326, an 18 DIP offset).
	graphCtrlDy = 7
)

// laneX returns the centre x of lane i: 16 + 16·i (history.md §4.1).
//
// The Graph slot is 54 wide, so it holds graphMaxLanes lanes. A history with
// more open branches folds the lanes past the last one onto it, so no line
// runs over the commit text beside the slot.
const graphMaxLanes = 3

func laneX(i int) float32 {
	return graphLaneX0 + graphLaneStep*float32(min(i, graphMaxLanes-1))
}

// laneColor is a lane's colour, cycling the design's two: the accent (--l1)
// for even lanes, starting with the mainline at lane 0, and purple (--l2) for
// odd lanes.
func laneColor(tk kitTokens, lane int) ui.Color {
	if min(lane, graphMaxLanes-1)%2 == 1 {
		return tk.lane2
	}
	return tk.accent
}

// graphSlot draws the Graph for commit row i: the lines passing straight
// through it, the lines leaving its node toward the next row, and the node
// itself. It paints over the fixed 64-wide, 56-tall slot but draws its
// downward lines a row further so rows join seamlessly (history.md §4).
func (u *UI) graphSlot(c *ui.Context, i int) {
	h := &u.hist
	box := ui.Box(c).Width(graphSlotWidth).Height(commitRowHeight).Shrink(0)
	if !h.graphing || i < 0 || i >= len(h.rows) {
		return
	}
	row := h.rows[i]
	class := u.classAt(i)
	tk := kitDesignTokens(c.Theme().Dark)
	// A line enters the node from the row above only when that row carries
	// a line down this lane. A branch's first or last commit has none, so
	// nothing is drawn above its node.
	fromAbove := i > 0 && i-1 < len(h.rows) && entersFromAbove(h.rows[i-1], row.Node)
	groups := graphRowGroups{node: -1}
	if i < len(h.groups) {
		groups = h.groups[i]
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		u.drawGraphRow(p, r, row, class, tk, fromAbove, groups)
	})
}

// entersFromAbove reports whether the row above, prev, leaves a line down
// lane: a lane passing straight through it, or an edge from its node onto
// the lane. Those are the segments that reach the top of the row below.
func entersFromAbove(prev workflow.GraphRow, lane int) bool {
	for _, l := range prev.Through {
		if l == lane {
			return true
		}
	}
	for _, e := range prev.Edges {
		if e.From == prev.Node && e.To == lane {
			return true
		}
	}
	return false
}

// classAt returns commit i's class, Shared when its hash is unknown.
func (u *UI) classAt(i int) workflow.GraphClass {
	h := &u.hist
	if i < 0 || i >= len(h.commits) {
		return workflow.Shared
	}
	return h.classes[h.commits[i].Hash]
}

// drawGraphRow draws one row's lanes, node and edges into r, the slot's box
// in the window. Every segment is drawn inside the row's own bounds so the
// list's per-row clip never cuts it: through lanes run the full height; the
// node's lane carries a straight segment from the top (when a line enters
// from above) down to its node and on toward the next row; an edge that bends
// to another lane is a half cubic Bézier that meets the row boundary at the
// destination lane's x, where the next row's straight top segment continues
// it — so consecutive rows join seamlessly (history.md §4.2; issue 13 #28).
func (u *UI) drawGraphRow(p *ui.Painter, r ui.Rect, row workflow.GraphRow, class workflow.GraphClass, tk kitTokens, fromAbove bool, g graphRowGroups) {
	top := r.Y
	bottom := top + commitRowHeight
	nodeY := top + graphNodeCy
	node := g.nodeColour(tk, row.Node)
	// Lines passing straight through the row, drawn its full height so the
	// rows above and below join to them.
	for _, lane := range row.Through {
		x := r.X + laneX(lane)
		p.Line(x, top, x, bottom, graphLineWidth, g.lane(tk, lane))
	}
	nodeX := r.X + laneX(row.Node)
	// The node's own lane entering from above: a straight segment from the
	// row top down to the node, matching what the row above left at the
	// boundary (a straight drop or a bend that landed on this lane).
	if fromAbove {
		p.Line(nodeX, top, nodeX, nodeY, graphLineWidth, node)
	}
	for _, e := range row.Edges {
		elided := e.Kind == workflow.EdgeElided
		switch {
		case e.From == row.Node && e.To == row.Node:
			// The node's lane continues toward the next row: the lower half,
			// within this row's bounds. When elided, the next visible commit
			// is not this one's real parent, so the segment is drawn broken
			// with a small dot, not a solid parent line.
			if elided {
				u.drawElided(p, nodeX, nodeY, nodeX, bottom, g.leaveColour(tk, row.Node), tk)
			} else {
				p.Line(nodeX, nodeY, nodeX, bottom, graphLineWidth, g.leaveColour(tk, row.Node))
			}
		case e.From == row.Node:
			// The node bends toward another lane in the next row: a half
			// curve from the node to that lane's x at the row boundary.
			toX := r.X + laneX(e.To)
			leave := g.leaveColour(tk, e.To)
			if elided {
				u.drawElided(p, nodeX, nodeY, toX, bottom, leave, tk)
				break
			}
			var path ui.Path
			path.MoveTo(nodeX, nodeY)
			path.CubeTo(nodeX, nodeY+graphCtrlDy, toX, bottom-graphCtrlDy, toX, bottom)
			p.StrokePath(&path, graphLineWidth, leave)
		case e.To == row.Node:
			// Another lane merges into the node this row: a half curve from
			// that lane's x at the row top down into the node.
			fromX := r.X + laneX(e.From)
			incoming := g.lane(tk, e.From)
			if elided {
				u.drawElided(p, fromX, top, nodeX, nodeY, incoming, tk)
				break
			}
			var path ui.Path
			path.MoveTo(fromX, top)
			path.CubeTo(fromX, top+graphCtrlDy, nodeX, nodeY-graphCtrlDy, nodeX, nodeY)
			p.StrokePath(&path, graphLineWidth, incoming)
		}
		// Edges with neither end on the node (From == To of a through lane)
		// are already drawn above as full-height through lines.
	}
	u.drawGraphNode(p, nodeX, nodeY, node, class, tk)
}

// elidedDashLen and elidedGapLen size the dashes of an elided edge: the gap
// between a visible commit and its next visible ancestor in a search, where
// intervening commits are hidden. elidedDotR is the small dot drawn at the
// segment's middle to read as "some ancestor", not a direct parent.
const (
	elidedDashLen = 4
	elidedGapLen  = 4
	elidedDotR    = 2
)

// drawElided draws a broken (dashed) straight segment from (x0,y0) to (x1,y1)
// in c, with a small hollow dot at its middle. The design's continuous graph
// has no elided edge, so this broken-with-a-dot form is a recorded deviation
// (issue 14 #2): it stands for hidden commits between two search results and
// must never read as a solid direct parent line. The dashes keep lane colour
// so the branch a result belongs to is still legible.
func (u *UI) drawElided(p *ui.Painter, x0, y0, x1, y1 float32, c ui.Color, tk kitTokens) {
	dx, dy := x1-x0, y1-y0
	length := float32(math.Hypot(float64(dx), float64(dy)))
	if length == 0 {
		return
	}
	ux, uy := dx/length, dy/length
	step := float32(elidedDashLen + elidedGapLen)
	for d := float32(0); d < length; d += step {
		end := d + elidedDashLen
		if end > length {
			end = length
		}
		p.Line(x0+ux*d, y0+uy*d, x0+ux*end, y0+uy*end, graphLineWidth, c)
	}
	// A small hollow dot at the middle marks the break as an elision.
	mx, my := x0+dx/2, y0+dy/2
	var dot ui.Path
	dot.Circle(mx, my, elidedDotR)
	p.FillPath(&dot, tk.bg)
	var ring ui.Path
	ring.Circle(mx, my, elidedDotR)
	p.StrokePath(&ring, graphLineWidth, c)
}

// drawGraphNode draws the node at (x, cy) in lane's colour: hollow (filled
// with the background) for an Unpushed commit, filled for a Shared one, and
// a hollow info node for an Incoming commit — the measurement defines no
// Incoming node, so this is a recorded deviation (history.md §4.3, §5.3).
func (u *UI) drawGraphNode(p *ui.Painter, x, cy float32, colour ui.Color, class workflow.GraphClass, tk kitTokens) {
	stroke, fill := colour, colour // Shared: filled.
	switch class {
	case workflow.Unpushed:
		fill = tk.bg // hollow.
	case workflow.Incoming:
		stroke, fill = tk.acx, tk.bg // hollow accent (deviation).
	}
	var disc ui.Path
	disc.Circle(x, cy, graphNodeR)
	p.FillPath(&disc, fill)
	var ring ui.Path
	ring.Circle(x, cy, graphNodeR)
	p.StrokePath(&ring, graphLineWidth, stroke)
}

// historyPageResult is one read of the History list: either a Graph page or
// a search page. The unused fields stay zero for the other kind.
type historyPageResult struct {
	commits []workflow.CommitInfo
	more    bool
	err     error
	// Search:
	rq workflow.ResolvedQuery
	// Graph:
	rg      workflow.ResolvedGraph
	rows    []workflow.GraphRow
	lanes   workflow.LaneState
	classes map[string]workflow.GraphClass
	sources map[string]string
}

// primaryScope is the checked-out branch: the basis of the Unpushed and
// Incoming classes. Its name is "" on a detached HEAD.
func (u *UI) primaryScope() workflow.GraphScope {
	return workflow.GraphScope{Name: u.snap.Branch}
}

// readHistoryPage reads the first page of the list, always as a graph: the
// Graph of the shown branches when not searching (its commits, lane rows and
// classes), or the search results laid out with elided edges, classified
// against the same fixed tips. It runs off the main goroutine; the caller
// applies it.
func (u *UI) readHistoryPage(ctx context.Context, repo *workflow.Repository, q workflow.HistoryQuery, primary workflow.GraphScope, graphing bool) historyPageResult {
	var res historyPageResult
	if !graphing {
		return u.readSearchPage(ctx, repo, q, primary)
	}
	res.rg, res.err = repo.ResolveGraphView(ctx, primary, q.Scopes, q.Order)
	if res.err != nil {
		return res
	}
	limit := historyPage
	if u.chrome.shell {
		limit = historyAllLimit
	}
	var g workflow.GraphResult
	g, res.err = repo.Graph(ctx, res.rg, 0, limit, workflow.LaneState{})
	if res.err != nil {
		return res
	}
	res.commits, res.rows, res.lanes, res.more = graphCommits(g), g.Rows, g.Lanes, g.More
	res.classes = graphClasses(g)
	res.sources = graphSources(g)
	return res
}

// graphCommits returns the plain CommitInfo of a Graph page, in order.
func graphCommits(g workflow.GraphResult) []workflow.CommitInfo {
	out := make([]workflow.CommitInfo, len(g.Commits))
	for i, e := range g.Commits {
		out[i] = e.CommitInfo
	}
	return out
}

// graphClasses maps each commit's hash to its class.
func graphClasses(g workflow.GraphResult) map[string]workflow.GraphClass {
	m := make(map[string]workflow.GraphClass, len(g.Commits))
	for _, e := range g.Commits {
		m[e.Hash] = e.Class
	}
	return m
}

// readSearchPage reads the first page of a search, laid out as a graph with
// elided edges for the commits hidden between results, classified against the
// same fixed tips a Graph of the scope would use. The query is fixed to a
// start commit (rq) and the classification to the scope's tips (rg), so both
// stay consistent while refs move during paging. When the scope has no graph
// basis (an unborn HEAD, say), classification falls back to an empty graph:
// the results still list, every one Shared.
func (u *UI) readSearchPage(ctx context.Context, repo *workflow.Repository, q workflow.HistoryQuery, primary workflow.GraphScope) historyPageResult {
	var res historyPageResult
	res.rq, res.err = repo.ResolveQuery(ctx, q)
	if res.err != nil {
		return res
	}
	res.rg, _ = repo.ResolveGraphView(ctx, primary, q.Scopes, q.Order) // a failed basis leaves rg empty: all Shared.
	limit := historyPage
	if u.chrome.shell {
		limit = historyAllLimit
	}
	var g workflow.SearchGraphResult
	g, res.err = repo.SearchGraph(ctx, res.rq, res.rg, 0, limit)
	if res.err != nil {
		return res
	}
	res.commits, res.rows, res.more = searchCommits(g), g.Rows, g.More
	res.classes = searchClasses(g)
	res.sources = searchSources(g)
	res.lanes = workflow.LaneState{} // a search does not carry lanes across pages.
	return res
}

// searchCommits returns the plain CommitInfo of a search graph page, in order.
func searchCommits(g workflow.SearchGraphResult) []workflow.CommitInfo {
	out := make([]workflow.CommitInfo, len(g.Commits))
	for i, e := range g.Commits {
		out[i] = e.CommitInfo
	}
	return out
}

// searchClasses maps each matching commit's hash to its class.
func searchClasses(g workflow.SearchGraphResult) map[string]workflow.GraphClass {
	m := make(map[string]workflow.GraphClass, len(g.Commits))
	for _, e := range g.Commits {
		m[e.Hash] = e.Class
	}
	return m
}
