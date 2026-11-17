package workflow

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// classesOf joins a page's classifications for comparison, newest first.
func classesOf(cs []GraphEntry) string {
	letter := map[GraphClass]string{Shared: "S", Unpushed: "U", Incoming: "I"}
	var out []string
	for _, c := range cs {
		out = append(out, letter[c.Class]+":"+c.Subject)
	}
	return strings.Join(out, "|")
}

// revList returns the hashes of git rev-list args, as a set.
func (e *env) revList(dir string, args ...string) map[string]bool {
	e.t.Helper()
	out := e.git(dir, append([]string{"rev-list"}, args...)...)
	set := map[string]bool{}
	for _, h := range strings.Fields(out) {
		set[h] = true
	}
	return set
}

func graphPage(t *testing.T, r *Repository, rg ResolvedGraph, skip, limit int, start LaneState) GraphResult {
	t.Helper()
	res, err := r.Graph(context.Background(), rg, skip, limit, start)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGraphLinear(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	for i := range 4 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "c"+strconv.Itoa(i))
	}
	repo, _ := e.open(dir)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	if classesOf(res.Commits) != "S:c3|S:c2|S:c1|S:c0" {
		t.Fatalf("linear classes %q", classesOf(res.Commits))
	}
	if len(res.Rows) != 4 || res.More {
		t.Fatalf("rows %d more %v", len(res.Rows), res.More)
	}
	// HEAD and main label the tip.
	sort.Strings(res.Commits[0].Refs)
	if strings.Join(res.Commits[0].Refs, ",") != "HEAD,main" {
		t.Fatalf("tip refs %v", res.Commits[0].Refs)
	}
}

func TestGraphNoUpstream(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "base")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "tip")
	repo, _ := e.open(dir)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	// Without an upstream nothing is Unpushed.
	if classesOf(res.Commits) != "S:tip|S:base" {
		t.Fatalf("no-upstream classes %q", classesOf(res.Commits))
	}
}

func TestGraphMissingScope(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "c")
	repo, _ := e.open(dir)
	_, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "nope"})
	wantCode(t, err, CodeScopeMissing)
}

// checkClasses asserts every commit's class matches git rev-list of the
// fixed tips: Unpushed = branch ^upstream, Incoming = upstream ^branch.
func checkClasses(t *testing.T, e *env, dir string, rg ResolvedGraph, res GraphResult) {
	t.Helper()
	var up, in map[string]bool
	if rg.Upstream != "" {
		up = e.revList(dir, rg.Branch, "^"+rg.Upstream)
		in = e.revList(dir, rg.Upstream, "^"+rg.Branch)
	}
	for _, c := range res.Commits {
		want := Shared
		switch {
		case up[c.Hash]:
			want = Unpushed
		case in[c.Hash]:
			want = Incoming
		}
		if c.Class != want {
			t.Fatalf("%s: class %d want %d", c.Short(), c.Class, want)
		}
	}
}

func TestGraphLocalAhead(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.commitAs(rs.mine, "x.txt", "M", "m@x", "Mine 1")
	e.commitAs(rs.mine, "y.txt", "M", "m@x", "Mine 2")
	repo, _ := e.open(rs.mine)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	if classesOf(res.Commits) != "U:Mine 2|U:Mine 1|S:Initial" {
		t.Fatalf("local ahead %q", classesOf(res.Commits))
	}
	checkClasses(t, e, rs.mine, rg, res)
}

func TestGraphUpstreamAhead(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "t.txt", "t\n")
	e.git(rs.mine, "fetch", "-q", "origin")
	repo, _ := e.open(rs.mine)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	if classesOf(res.Commits) != "I:Their t.txt|S:Initial" {
		t.Fatalf("upstream ahead %q", classesOf(res.Commits))
	}
	checkClasses(t, e, rs.mine, rg, res)
}

func TestGraphDiverged(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "t.txt", "t\n")
	e.git(rs.mine, "fetch", "-q", "origin")
	e.commitAs(rs.mine, "m.txt", "M", "m@x", "Mine only")
	repo, _ := e.open(rs.mine)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	checkClasses(t, e, rs.mine, rg, res)
	// Both sides present; the shared base is Shared.
	var u, i, s int
	for _, c := range res.Commits {
		switch c.Class {
		case Unpushed:
			u++
		case Incoming:
			i++
		case Shared:
			s++
		}
	}
	if u != 1 || i != 1 || s != 1 {
		t.Fatalf("diverged counts u=%d i=%d s=%d", u, i, s)
	}
}

func TestGraphRemoteScope(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	rs.theirCommit(t, e, "t.txt", "t\n")
	e.git(rs.mine, "fetch", "-q", "origin")
	e.commitAs(rs.mine, "m.txt", "M", "m@x", "Mine only")
	repo, _ := e.open(rs.mine)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "origin/main", Remote: true})
	if err != nil {
		t.Fatal(err)
	}
	if rg.Upstream != "" {
		t.Fatalf("remote scope has upstream %q", rg.Upstream)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	// A remote scope reads only its own ref; the local commit is absent and
	// nothing is Unpushed or Incoming.
	if classesOf(res.Commits) != "S:Their t.txt|S:Initial" {
		t.Fatalf("remote scope %q", classesOf(res.Commits))
	}
	checkClasses(t, e, rs.mine, rg, res)
}

func TestGraphForkAndMerge(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "base")
	e.git(dir, "switch", "-q", "-c", "side")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "side work")
	e.git(dir, "switch", "-q", "main")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "main work")
	e.git(dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	repo, _ := e.open(dir)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	res := graphPage(t, repo, rg, 0, 200, LaneState{})
	if len(res.Commits) != 4 || len(res.Rows) != 4 {
		t.Fatalf("fork/merge commits %d rows %d", len(res.Commits), len(res.Rows))
	}
	// The merge commit (first) has two parents, so its row carries a merge
	// edge to a second lane.
	if len(res.Commits[0].Parents) != 2 {
		t.Fatalf("merge parents %d", len(res.Commits[0].Parents))
	}
	var merge bool
	for _, ed := range res.Rows[0].Edges {
		if ed.Kind == EdgeMerge {
			merge = true
		}
	}
	if !merge {
		t.Fatal("no merge edge from the merge commit")
	}
}

// subjectsOfGraph joins a page's subjects, newest first.
func subjectsOfGraph(cs []GraphEntry) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Subject)
	}
	return strings.Join(out, "|")
}

func TestGraphPagesStitch(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "base")
	e.git(dir, "switch", "-q", "-c", "side")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "side work")
	e.git(dir, "switch", "-q", "main")
	for i := range 3 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "main "+strconv.Itoa(i))
	}
	e.git(dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	repo, _ := e.open(dir)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	whole := graphPage(t, repo, rg, 0, 200, LaneState{})
	// Read the same history in two pages and join them.
	first := graphPage(t, repo, rg, 0, 3, LaneState{})
	if !first.More {
		t.Fatal("first page claims no more")
	}
	second := graphPage(t, repo, rg, first.Cursor, 200, first.Lanes)
	joinedSubjects := subjectsOfGraph(first.Commits) + "|" + subjectsOfGraph(second.Commits)
	if joinedSubjects != subjectsOfGraph(whole.Commits) {
		t.Fatalf("stitched subjects %q != %q", joinedSubjects, subjectsOfGraph(whole.Commits))
	}
	joinedRows := append(append([]GraphRow{}, first.Rows...), second.Rows...)
	if !sameRows(joinedRows, whole.Rows) {
		t.Fatalf("stitched rows differ from whole")
	}
}

// sameRows reports whether two row slices lay out identically.
func sameRows(a, b []GraphRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Node != b[i].Node || !eqInts(a[i].Through, b[i].Through) || !eqEdges(a[i].Edges, b[i].Edges) {
			return false
		}
	}
	return true
}

func eqInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func eqEdges(a, b []GraphEdge) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGraphStableWhileHeadMoves(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	for i := range 9 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "c"+strconv.Itoa(i))
	}
	repo, _ := e.open(dir)
	rg, err := repo.ResolveGraph(context.Background(), GraphScope{Name: "main"})
	if err != nil {
		t.Fatal(err)
	}
	first := graphPage(t, repo, rg, 0, 4, LaneState{})
	if subjectsOfGraph(first.Commits) != "c8|c7|c6|c5" || !first.More {
		t.Fatalf("first %q", subjectsOfGraph(first.Commits))
	}
	// New commits land mid-paging; the fixed tip keeps the walk the same.
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "late")
	second := graphPage(t, repo, rg, first.Cursor, 4, first.Lanes)
	third := graphPage(t, repo, rg, second.Cursor, 4, second.Lanes)
	if subjectsOfGraph(second.Commits) != "c4|c3|c2|c1" || !second.More {
		t.Fatalf("second %q", subjectsOfGraph(second.Commits))
	}
	if subjectsOfGraph(third.Commits) != "c0" || third.More {
		t.Fatalf("third %q more=%v", subjectsOfGraph(third.Commits), third.More)
	}
}
