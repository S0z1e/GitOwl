package workflow

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// searchGraph resolves q and a graph scope, then lays search results out as a
// graph page classified against the same fixed basis.
func searchGraph(t *testing.T, repo *Repository, q HistoryQuery, gscope GraphScope, skip, limit int) SearchGraphResult {
	t.Helper()
	rq, err := repo.ResolveQuery(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := repo.ResolveGraph(context.Background(), gscope)
	if err != nil {
		t.Fatal(err)
	}
	res, err := repo.SearchGraph(context.Background(), rq, rg, skip, limit)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func subjectsOfSearchGraph(cs []GraphEntry) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Subject)
	}
	return strings.Join(out, "|")
}

// TestSearchGraphElidesHiddenParents checks that a message search whose
// results skip intervening commits draws EdgeElided for the gap, never a
// false direct parent edge, while adjacent visible results keep EdgeNormal.
func TestSearchGraphElidesHiddenParents(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	// match 0 (oldest) ... match 2 (newest) with filler between, so the
	// search result hides each match's true parent.
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "match 0")
	for i := range 3 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "filler "+strconv.Itoa(i))
	}
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "match 1")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "match 2")
	repo, _ := e.open(dir)
	res := searchGraph(t, repo, HistoryQuery{Field: FieldMessage, Text: "match"}, GraphScope{Name: "main"}, 0, 200)
	if got := subjectsOfSearchGraph(res.Commits); got != "match 2|match 1|match 0" {
		t.Fatalf("results %q", got)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("%d rows for 3 results", len(res.Rows))
	}
	// Row 0 (match 2) -> match 1 is the real parent: normal.
	if e := findEdge(t, res.Rows[0]); e.Kind != EdgeNormal {
		t.Fatalf("match2->match1 kind %v, want normal", e.Kind)
	}
	// Row 1 (match 1) -> match 0 hides three fillers: elided.
	if e := findEdge(t, res.Rows[1]); e.Kind != EdgeElided {
		t.Fatalf("match1->match0 kind %v, want elided", e.Kind)
	}
}

// TestSearchGraphMainlineStaysOneLane checks that two matches on the same
// mainline, with hidden commits between them, lay out in a single lane: the
// older match is the newer one's next visible ancestor, so the elided edge
// joins them in lane 0 rather than splitting into two parallel branches
// (issue 14 #31).
func TestSearchGraphMainlineStaysOneLane(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	// B (oldest match), hidden x, y, then A (newest match): a straight
	// mainline, A's recorded parent is a hidden commit, not B.
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "B match")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "x")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "y")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "A match")
	repo, _ := e.open(dir)
	res := searchGraph(t, repo, HistoryQuery{Field: FieldMessage, Text: "match"}, GraphScope{Name: "main"}, 0, 200)
	if got := subjectsOfSearchGraph(res.Commits); got != "A match|B match" {
		t.Fatalf("results %q", got)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("%d rows, want 2", len(res.Rows))
	}
	// Both nodes share lane 0: A does not spawn a second branch for B.
	if res.Rows[0].Node != 0 || res.Rows[1].Node != 0 {
		t.Fatalf("nodes %d,%d, want both in lane 0 (not two branches)",
			res.Rows[0].Node, res.Rows[1].Node)
	}
	// A -> B is the next visible ancestor across hidden x, y: elided, down
	// lane 0, so the dashed segment runs straight from A to B.
	ed := findEdge(t, res.Rows[0])
	if ed.To != 0 || ed.Kind != EdgeElided {
		t.Fatalf("A->B edge %+v, want To 0 Kind elided", ed)
	}
}

// TestSearchGraphTwoBranchesStayApart checks that when each of two real
// branches contributes exactly one match and the merge that joins them is
// hidden, the two matches keep their own lanes and are not wrongly joined:
// neither is the other's ancestor, so no elided edge connects them (issue
// 14 #31, the other side of the same fix).
func TestSearchGraphTwoBranchesStayApart(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "base")
	// side branch with its own match
	e.git(dir, "checkout", "-q", "-b", "side")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "side match")
	// back on main, its own match, then a merge of side (the merge hides
	// both matches' parents from each other).
	e.git(dir, "checkout", "-q", "main")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "main match")
	e.git(dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	repo, _ := e.open(dir)
	res := searchGraph(t, repo, HistoryQuery{Field: FieldMessage, Text: "match"}, GraphScope{Name: "main"}, 0, 200)
	if len(res.Rows) != 2 {
		t.Fatalf("%d rows, want 2 (the two branch matches)", len(res.Rows))
	}
	// The two matches sit in different lanes; neither is the other's
	// ancestor, so there is no elided edge between the two nodes.
	if res.Rows[0].Node == res.Rows[1].Node {
		t.Fatalf("both matches in lane %d, want separate lanes", res.Rows[0].Node)
	}
	for _, r := range res.Rows {
		for _, ed := range r.Edges {
			if ed.Kind == EdgeElided && ed.From == r.Node && ed.To != r.Node {
				t.Fatalf("branch matches wrongly joined by an elided edge %+v", ed)
			}
		}
	}
}

// findEdge returns the first outgoing edge from a row's node.
func findEdge(t *testing.T, r GraphRow) GraphEdge {
	t.Helper()
	for _, e := range r.Edges {
		if e.From == r.Node {
			return e
		}
	}
	t.Fatalf("no outgoing edge in %+v", r)
	return GraphEdge{}
}

// TestSearchGraphClassifiesFromFixedBasis checks search results carry the
// same Unpushed/Incoming classes the Graph assigns, from the fixed tips, not
// from the search scope alone. A local-scope search finds the local-only
// commit and marks it Unpushed; a remote-scope search finds the
// upstream-only commit but, classified against the same local basis, marks
// it Incoming, so a commit's class does not depend on which scope found it.
func TestSearchGraphClassifiesFromFixedBasis(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "t.txt", "their change\n") // message "Their t.txt"
	e.git(rs.mine, "fetch", "-q", "origin")
	e.commitAs(rs.mine, "m.txt", "M", "m@x", "Mine change") // message "Mine change"
	repo, _ := e.open(rs.mine)
	// The local-only commit is reachable from HEAD and Unpushed.
	local := searchGraph(t, repo, HistoryQuery{Field: FieldMessage, Text: "Mine"}, GraphScope{Name: "main"}, 0, 200)
	classes := map[string]GraphClass{}
	for _, c := range local.Commits {
		classes[c.Subject] = c.Class
	}
	if classes["Mine change"] != Unpushed {
		t.Fatalf("Mine change class %v, want Unpushed", classes["Mine change"])
	}
	// The upstream-only commit is only reachable through the remote ref;
	// searching it while classifying against the local main basis still
	// marks it Incoming.
	rq, err := repo.ResolveQuery(context.Background(), HistoryQuery{
		Scopes: []HistoryScope{{Ref: "refs/remotes/origin/main", Name: "origin/main", Remote: true}},
		Field:  FieldMessage, Text: "Their"})
	if err != nil {
		t.Fatal(err)
	}
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := repo.SearchGraph(context.Background(), rq, rg, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	var incoming GraphClass = Shared
	for _, c := range remote.Commits {
		if c.Subject == "Their t.txt" {
			incoming = c.Class
		}
	}
	if incoming != Incoming {
		t.Fatalf("Their t.txt class %v, want Incoming", incoming)
	}
}

// TestSearchGraphStablePages checks two stitched pages of a search graph
// match one read whole, and that a commit landing mid-paging does not shift
// the fixed query's results.
func TestSearchGraphStablePages(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	for i := range 9 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "match "+strconv.Itoa(i))
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "other "+strconv.Itoa(i))
	}
	repo, _ := e.open(dir)
	q := HistoryQuery{Field: FieldMessage, Text: "match"}
	whole := searchGraph(t, repo, q, GraphScope{Name: "main"}, 0, 200)
	rq, err := repo.ResolveQuery(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.SearchGraph(context.Background(), rq, rg, 0, 4)
	if err != nil || !first.More {
		t.Fatalf("first page more=%v err=%v", first.More, err)
	}
	// A commit lands mid-paging; the fixed query keeps the walk.
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "match new")
	second, err := repo.SearchGraph(context.Background(), rq, rg, first.Cursor, 200)
	if err != nil {
		t.Fatal(err)
	}
	joined := subjectsOfSearchGraph(first.Commits) + "|" + subjectsOfSearchGraph(second.Commits)
	if joined != subjectsOfSearchGraph(whole.Commits) {
		t.Fatalf("stitched %q != whole %q", joined, subjectsOfSearchGraph(whole.Commits))
	}
}

// TestSearchGraphEmptyQueryIsLinear checks an empty query (listing the whole
// scope) lays out with real parents and no elided edges, since nothing is
// hidden.
func TestSearchGraphEmptyQueryIsLinear(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	for i := range 3 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "c"+strconv.Itoa(i))
	}
	repo, _ := e.open(dir)
	res := searchGraph(t, repo, HistoryQuery{Field: FieldMessage, Text: ""}, GraphScope{Name: "main"}, 0, 200)
	for _, r := range res.Rows {
		for _, ed := range r.Edges {
			if ed.Kind == EdgeElided {
				t.Fatalf("empty query produced an elided edge: %+v", r)
			}
		}
	}
}
