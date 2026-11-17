package desktop

import (
	"path/filepath"
	"testing"

	"gitowl/internal/gittest"
	"gitowl/internal/workflow"
)

// graphRows returns the Graph rows the History list drew, failing when the
// list is not in Graph mode (a search, or not yet read).
func (h *harness) graphRows() []workflow.GraphRow {
	h.T.Helper()
	if !h.u.hist.graphing {
		h.T.Fatalf("history is not graphing (query %+v)", h.u.hist.query)
	}
	return h.u.hist.rows
}

// wantLanes checks that a Graph laid out off the real commit parents of the
// listed commits (newest first) places each node in the lane the workflow's
// own layout does, so the UI draws the topology git reports, not fixed
// nodes. It returns the rows for further checks.
func (h *harness) wantLanes(t *testing.T) []workflow.GraphRow {
	t.Helper()
	rows := h.graphRows()
	commits := h.u.hist.commits
	if len(rows) != len(commits) {
		t.Fatalf("%d rows for %d commits", len(rows), len(commits))
	}
	gcs := make([]workflow.GraphCommit, len(commits))
	for i, c := range commits {
		gcs[i] = workflow.GraphCommit{Hash: c.Hash, Parents: c.Parents}
	}
	want := workflow.LayOutLanes(gcs, workflow.LaneState{}, false)
	for i := range rows {
		if rows[i].Node != want.Rows[i].Node {
			t.Fatalf("row %d node lane %d, want %d (commit %q)",
				i, rows[i].Node, want.Rows[i].Node, commits[i].Subject)
		}
	}
	return rows
}

func TestGraphLinearTopology(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	for _, s := range []string{"One", "Two", "Three"} {
		gittest.Write(t, filepath.Join(dir, "a.txt"), s+"\n")
		h.Commit(dir, s)
	}
	h.open(dir)
	h.click("All commits")
	rows := h.wantLanes(t)
	// A straight line: every node in lane 0, so every node x is 16.
	for i, r := range rows {
		if r.Node != 0 {
			t.Fatalf("row %d node lane %d, want 0 for a linear history", i, r.Node)
		}
		if got := laneX(r.Node); got != graphLaneX0 {
			t.Fatalf("row %d node x %v, want %v", i, got, float32(graphLaneX0))
		}
	}
}

func TestGraphForkAndMerge(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	gittest.Write(t, filepath.Join(dir, "a.txt"), "base\n")
	h.Commit(dir, "Base")
	h.Git(dir, "checkout", "-q", "-b", "side")
	gittest.Write(t, filepath.Join(dir, "s.txt"), "side\n")
	h.Commit(dir, "On side")
	h.Git(dir, "checkout", "-q", "main")
	gittest.Write(t, filepath.Join(dir, "m.txt"), "main\n")
	h.Commit(dir, "On main")
	h.Git(dir, "merge", "-q", "--no-ff", "-m", "Merge side", "side")
	h.open(dir)
	h.click("All commits")
	rows := h.wantLanes(t)

	// The merge commit is newest and has two parents, so its row leaves two
	// edges toward the next row: the mainline and the side lane.
	merge := h.u.hist.commits[0]
	if len(merge.Parents) != 2 {
		t.Fatalf("newest %q has %d parents, want a merge", merge.Subject, len(merge.Parents))
	}
	var bends int
	for _, e := range rows[0].Edges {
		if e.From == rows[0].Node && e.To != e.From {
			bends++
		}
	}
	if bends == 0 {
		t.Fatalf("merge row draws no lane-changing edge: %+v", rows[0].Edges)
	}
	// Some node sits in a side lane (x = 16 + 16·lane > 16), so the graph is
	// not a single column.
	side := false
	for _, r := range rows {
		if r.Node > 0 && laneX(r.Node) > graphLaneX0 {
			side = true
		}
	}
	if !side {
		t.Fatalf("no side lane in a fork/merge history: %+v", rows)
	}
}

// TestGraphRowGeometryMatchesMeasurement checks the drawing constants the
// slot uses are the repository design's (the History board's graph SVG): a
// 54-wide slot, 30-tall rows, nodes at y = top + 15 and x = 14 + 16·lane,
// radius 4.5, line width 2.
func TestGraphRowGeometryMatchesMeasurement(t *testing.T) {
	if graphSlotWidth != 54 || commitRowHeight != 30 || graphNodeCy != 15 {
		t.Fatalf("slot %v row %v cy %v, want 54/30/15", graphSlotWidth, commitRowHeight, graphNodeCy)
	}
	if graphNodeR != 4.5 || graphLineWidth != 2 {
		t.Fatalf("node r %v width %v, want 4.5/2", graphNodeR, graphLineWidth)
	}
	for lane, want := range map[int]float32{0: 14, 1: 30, 2: 46} {
		if got := laneX(lane); got != want {
			t.Fatalf("lane %d x %v, want %v", lane, got, want)
		}
	}
}

// classOf returns the Graph class the list gave commit subject.
func (h *harness) classOf(subject string) workflow.GraphClass {
	h.T.Helper()
	i := h.commitIndex(subject)
	return h.u.hist.classes[h.u.hist.commits[i].Hash]
}

func TestGraphLocalAheadMarksUnpushed(t *testing.T) {
	h := newHarness(t, 1360, 820)
	_, mine, _ := clones(t, h.Env)
	gittest.Write(t, filepath.Join(mine, "x.txt"), "x\n")
	h.Commit(mine, "Local only")
	h.open(mine)
	h.click("All commits")
	h.wantLanes(t)
	// The local-only commit the upstream lacks is Unpushed; the shared base
	// (Initial) is Shared.
	if got := h.classOf("Local only"); got != workflow.Unpushed {
		t.Fatalf("local commit class %v, want Unpushed", got)
	}
	if got := h.classOf("Initial"); got != workflow.Shared {
		t.Fatalf("shared base class %v, want Shared", got)
	}
}

func TestGraphUpstreamAheadShowsIncoming(t *testing.T) {
	h := newHarness(t, 1360, 820)
	_, mine, theirs := clones(t, h.Env)
	theyPush(t, h.Env, theirs, "b.txt")
	h.open(mine)
	h.click("Fetch") // origin/main now leads my main by one commit.
	h.click("All commits")
	rows := h.wantLanes(t)
	if len(rows) != len(h.u.hist.commits) {
		t.Fatalf("%d rows for %d commits", len(rows), len(h.u.hist.commits))
	}
	// The upstream-only commit appears in the list and is Incoming.
	if got := h.classOf("Their b.txt"); got != workflow.Incoming {
		t.Fatalf("upstream commit class %v, want Incoming", got)
	}
}

func TestGraphDivergenceHasBothSides(t *testing.T) {
	h := newHarness(t, 1360, 820)
	_, mine, theirs := clones(t, h.Env)
	theyPush(t, h.Env, theirs, "b.txt") // one commit only on the upstream
	gittest.Write(t, filepath.Join(mine, "x.txt"), "x\n")
	h.Commit(mine, "Mine only") // one commit only on the local branch
	h.open(mine)
	h.click("Fetch")
	h.click("All commits")
	h.wantLanes(t)
	// The local-only commit is Unpushed and the upstream-only one Incoming,
	// both in the same list, so divergence shows both sides.
	if got := h.classOf("Mine only"); got != workflow.Unpushed {
		t.Fatalf("local commit class %v, want Unpushed", got)
	}
	if got := h.classOf("Their b.txt"); got != workflow.Incoming {
		t.Fatalf("upstream commit class %v, want Incoming", got)
	}
}

func TestGraphNoUpstreamNeverUnpushed(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo() // a local repository with no remote at all
	for _, s := range []string{"One", "Two"} {
		gittest.Write(t, filepath.Join(dir, "a.txt"), s+"\n")
		h.Commit(dir, s)
	}
	h.open(dir)
	h.click("All commits")
	h.wantLanes(t)
	// Without an upstream nothing is Unpushed: every commit is Shared.
	for _, c := range h.u.hist.commits {
		if cl := h.u.hist.classes[c.Hash]; cl != workflow.Shared {
			t.Fatalf("commit %q class %v without an upstream, want Shared", c.Subject, cl)
		}
	}
	if h.u.hist.rg.Upstream != "" {
		t.Fatalf("resolved an upstream %q for a branch without one", h.u.hist.rg.Upstream)
	}
}

// laneColumnContinuous reports whether the lane-0 column drawn between two
// commit rows has a coloured line pixel on every scan row, i.e. no background
// gap breaks the mainline between its nodes (issue 13 #28). It finds the
// lane's x from the upper node's own pixels, then scans the inter-node span.
func laneColumnContinuous(t *testing.T, h *harness, upper, lower string) {
	t.Helper()
	img := h.tt.Image()
	b := img.Bounds()
	scale := float32(b.Dx()) / 1360.0
	up, ok := h.tt.Find(upper)
	if !ok {
		t.Fatalf("no row for %q", upper)
	}
	lo, ok := h.tt.Find(lower)
	if !ok {
		t.Fatalf("no row for %q", lower)
	}
	bg := kitDesignTokens(false).bg
	colored := func(px, py int) bool {
		r0, g0, bl0, _ := img.At(px, py).RGBA()
		dr := absByte(uint8(r0>>8), bg.R)
		dg := absByte(uint8(g0>>8), bg.G)
		db := absByte(uint8(bl0>>8), bg.B)
		return int(dr)+int(dg)+int(db) > 24 // clearly not the background
	}
	// Node centres sit at each row's vertical middle. The lane column is to
	// the left of the subject text, within the 64-DIP Graph slot; find it by
	// scanning rightward from the list column's left for the node's line.
	y0 := int((up.Y + up.H/2) * scale)
	y1 := int((lo.Y + lo.H/2) * scale)
	// The Graph slot sits at the row's left, the subject text just inside it;
	// find the lane by scanning rightward across the slot for the node line.
	// The lane 0 line sits at laneX(0) from the row's left edge. Use that x
	// directly: a scan would catch the node's disc or the selected row's
	// background edge, which are not the lane.
	left := int(up.X * scale)
	laneCx := left + int(laneX(0)*scale)
	if !colored(laneCx, y0) {
		t.Fatalf("no lane 0 node at x=%d under %q", laneCx, upper)
	}
	// Scan strictly between the two node centres; a small x band tolerates the
	// line's width and antialiasing.
	for py := y0 + 4; py < y1-4; py++ {
		hit := false
		for dx := -2; dx <= 2 && !hit; dx++ {
			px := laneCx + dx
			if px >= b.Min.X && px < b.Max.X && py >= b.Min.Y && py < b.Max.Y {
				hit = colored(px, py)
			}
		}
		if !hit {
			t.Fatalf("gap in mainline at y=%d between %q and %q (lane x≈%d)",
				py, upper, lower, laneCx)
		}
	}
}

func absByte(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

// TestGraphMainlineLineIsSeamless renders a linear history and checks the
// lane-0 line has no background gap between consecutive nodes: each row draws
// its segment top-to-bottom so rows join (history.md §4; issue 13 #28).
func TestGraphMainlineLineIsSeamless(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	for _, s := range []string{"First subject line", "Second subject line", "Third subject line"} {
		gittest.Write(t, filepath.Join(dir, "a.txt"), s+"\n")
		h.Commit(dir, s)
	}
	h.open(dir)
	h.click("All commits")
	h.wantLanes(t)
	h.tt.SetDark(false)
	h.tt.Frame()
	// Newest first: "Third" is the top row, "Second" the next; the line
	// between their nodes must be unbroken.
	laneColumnContinuous(t, h, "Third subject line", "Second subject line")
	laneColumnContinuous(t, h, "Second subject line", "First subject line")
}

// TestGraphUnpushedPillOnRowAndDetail checks a local-only commit shows the
// "Not pushed" pill on its list row and in its detail metadata, while the
// shared base shows neither pill (issue 13 #29; history.md §5.3, §6).
func TestGraphUnpushedPillOnRowAndDetail(t *testing.T) {
	h := newHarness(t, 1360, 820)
	_, mine, _ := clones(t, h.Env)
	gittest.Write(t, filepath.Join(mine, "x.txt"), "x\n")
	h.Commit(mine, "Local only commit")
	h.open(mine)
	h.click("All commits")
	h.wantLanes(t)
	if got := h.classOf("Local only commit"); got != workflow.Unpushed {
		t.Fatalf("local commit class %v, want Unpushed", got)
	}
	// The pill text is present once History is drawn (the list row), and the
	// newest commit is selected by default so the detail shows it too.
	if !h.tt.HasText("Not pushed") {
		t.Fatalf("no \"Not pushed\" pill in %q", h.tt.Texts())
	}
	// "Incoming" never appears here: nothing is only on the upstream.
	for _, s := range h.tt.Texts() {
		if s == "Incoming" {
			t.Fatalf("unexpected Incoming pill in an ahead-only history")
		}
	}
}

// TestGraphIncomingPillOnRow checks an upstream-only commit shows the
// "Incoming" pill (issue 13 #29; history.md §5.3).
func TestGraphIncomingPillOnRow(t *testing.T) {
	h := newHarness(t, 1360, 820)
	_, mine, theirs := clones(t, h.Env)
	theyPush(t, h.Env, theirs, "b.txt")
	h.open(mine)
	h.click("Fetch")
	h.click("All commits")
	h.wantLanes(t)
	if got := h.classOf("Their b.txt"); got != workflow.Incoming {
		t.Fatalf("upstream commit class %v, want Incoming", got)
	}
	if !h.tt.HasText("Incoming") {
		t.Fatalf("no \"Incoming\" pill in %q", h.tt.Texts())
	}
}

// TestGraphNoUpstreamNoPill checks a branch without an upstream shows no
// "Not pushed" pill: nothing is Unpushed without an upstream (issue 13 #29).
func TestGraphNoUpstreamNoPill(t *testing.T) {
	h := newHarness(t, 1360, 820)
	dir := h.Repo()
	for _, s := range []string{"One", "Two"} {
		gittest.Write(t, filepath.Join(dir, "a.txt"), s+"\n")
		h.Commit(dir, s)
	}
	h.open(dir)
	h.click("All commits")
	h.wantLanes(t)
	if h.u.hist.rg.Upstream != "" {
		t.Fatalf("resolved an upstream for a branch without one")
	}
	for _, s := range h.tt.Texts() {
		if s == "Not pushed" || s == "Incoming" {
			t.Fatalf("unexpected pill %q without an upstream", s)
		}
	}
}

// TestGraphDesignScreenshots renders the History Graph at 1360×820 in light
// and dark for the three topologies the ticket compares against the HTML
// design (linear, fork/merge, divergence) when GITOWL_SNAPSHOT_DIR is set. The assertions keep the states honest; the
// images are the visual evidence for the comparison.
func TestGraphDesignScreenshots(t *testing.T) {
	shot := func(name string, build func(h *harness) string) {
		h := newHarness(t, 1360, 820)
		dir := build(h)
		h.open(dir)
		h.click("All commits")
		h.wantLanes(t)
		h.tt.SetDark(false)
		h.tt.Frame()
		snapshot(t, h.tt, name+"-light")
		h.tt.SetDark(true)
		h.tt.Frame()
		snapshot(t, h.tt, name+"-dark")
	}
	shot("graph-linear", func(h *harness) string {
		dir := h.Repo()
		for _, s := range []string{"Persist recent repositories", "Add AI draft language option", "Show staged file count"} {
			gittest.Write(t, filepath.Join(dir, "a.txt"), s+"\n")
			h.Commit(dir, s)
		}
		return dir
	})
	shot("graph-merge", func(h *harness) string {
		dir := h.Repo()
		gittest.Write(t, filepath.Join(dir, "a.txt"), "base\n")
		h.Commit(dir, "Base")
		h.Git(dir, "checkout", "-q", "-b", "feature/ai-drafts")
		gittest.Write(t, filepath.Join(dir, "s.txt"), "s\n")
		h.Commit(dir, "Add AI draft language option")
		h.Git(dir, "checkout", "-q", "main")
		gittest.Write(t, filepath.Join(dir, "m.txt"), "m\n")
		h.Commit(dir, "Show staged file count in commit box")
		h.Git(dir, "merge", "-q", "--no-ff", "-m", "Merge branch feature/ai-drafts", "feature/ai-drafts")
		return dir
	})
	shot("graph-divergence", func(h *harness) string {
		_, mine, theirs := clones(t, h.Env)
		theyPush(t, h.Env, theirs, "b.txt")
		gittest.Write(t, filepath.Join(mine, "x.txt"), "x\n")
		h.Commit(mine, "Use controlLabel for composer tool labels")
		h.Git(mine, "fetch", "-q", "origin")
		return mine
	})
}
