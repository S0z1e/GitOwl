package workflow

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// commitAs commits a change to file by author with message.
func (e *env) commitAs(dir, file, author, email, message string) {
	e.t.Helper()
	write(e.t, filepath.Join(dir, file), message+"\n")
	e.git(dir, "add", "-A")
	e.git(dir, "-c", "user.name="+author, "-c", "user.email="+email, "commit", "-q", "-m", message)
}

func subjectsOf(cs []CommitInfo) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Subject)
	}
	return strings.Join(out, "|")
}

func search(t *testing.T, repo *Repository, q HistoryQuery, skip, limit int) ([]CommitInfo, bool) {
	t.Helper()
	rq, err := repo.ResolveQuery(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	cs, more, err := repo.SearchHistory(context.Background(), rq, skip, limit)
	if err != nil {
		t.Fatal(err)
	}
	return cs, more
}

func TestSearchMessageBeyondFirstPage(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Fix the parser\n\nThe needle is in the body.")
	for i := range 250 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "Filler "+strconv.Itoa(i))
	}
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Another NEEDLE in the subject")
	repo, _ := e.open(dir)
	cs, more := search(t, repo, HistoryQuery{Field: FieldMessage, Text: "needle"}, 0, 200)
	if more || subjectsOf(cs) != "Another NEEDLE in the subject|Fix the parser" {
		t.Fatalf("found %q", subjectsOf(cs))
	}
	// Literal: regular expression characters match themselves.
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Use a.*b literally [x]")
	cs, _ = search(t, repo, HistoryQuery{Field: FieldMessage, Text: "a.*b literally [x]"}, 0, 200)
	if subjectsOf(cs) != "Use a.*b literally [x]" {
		t.Fatalf("literal %q", subjectsOf(cs))
	}
	cs, _ = search(t, repo, HistoryQuery{Field: FieldMessage, Text: "a.b"}, 0, 200)
	if len(cs) != 0 {
		t.Fatalf("a.b matched %q", subjectsOf(cs))
	}
	// Option-like text is text.
	cs, _ = search(t, repo, HistoryQuery{Field: FieldMessage, Text: "--all"}, 0, 200)
	if len(cs) != 0 {
		t.Fatalf("--all matched %q", subjectsOf(cs))
	}
}

func TestSearchAuthorNameAndEmail(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.commitAs(dir, "a.txt", "Ada Lovelace", "ada@example.com", "By Ada")
	e.commitAs(dir, "b.txt", "Grace Hopper", "grace@navy.example", "By Grace")
	e.commitAs(dir, "c.txt", "Ada Lovelace", "ada@example.com", "Ada again")
	repo, _ := e.open(dir)
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldAuthor, Text: "ada"}, 0, 200); subjectsOf(cs) != "Ada again|By Ada" {
		t.Fatalf("by name %q", subjectsOf(cs))
	}
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldAuthor, Text: "NAVY.example"}, 0, 200); subjectsOf(cs) != "By Grace" {
		t.Fatalf("by email %q", subjectsOf(cs))
	}
	// The author field does not search messages, nor the message field
	// authors.
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldMessage, Text: "Hopper"}, 0, 200); len(cs) != 0 {
		t.Fatalf("message matched an author: %q", subjectsOf(cs))
	}
}

func TestSearchStablePagesWhileHeadMoves(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	for i := range 9 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "match "+strconv.Itoa(i))
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "other "+strconv.Itoa(i))
	}
	repo, _ := e.open(dir)
	rq, err := repo.ResolveQuery(context.Background(), HistoryQuery{Field: FieldMessage, Text: "match"})
	if err != nil {
		t.Fatal(err)
	}
	first, more, _ := repo.SearchHistory(context.Background(), rq, 0, 4)
	if !more || subjectsOf(first) != "match 8|match 7|match 6|match 5" {
		t.Fatalf("first page %q", subjectsOf(first))
	}
	// New commits land while paging: the query's start stays.
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "match new")
	second, more, _ := repo.SearchHistory(context.Background(), rq, 4, 4)
	third, last, _ := repo.SearchHistory(context.Background(), rq, 8, 4)
	if subjectsOf(second) != "match 4|match 3|match 2|match 1" || !more || subjectsOf(third) != "match 0" || last {
		t.Fatalf("pages %q %q", subjectsOf(second), subjectsOf(third))
	}
}

func TestSearchScopes(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.theirs, "switch", "-q", "-c", "topic")
	e.commitAs(rs.theirs, "t.txt", "T", "t@example.com", "Topic work")
	e.git(rs.theirs, "push", "-q", "origin", "topic")
	e.git(rs.mine, "fetch", "-q", "origin")
	e.git(rs.mine, "branch", "local-only")
	e.git(rs.mine, "switch", "-q", "local-only")
	e.commitAs(rs.mine, "l.txt", "L", "l@example.com", "Local work")
	e.git(rs.mine, "switch", "-q", "main")
	head := e.rev(rs.mine, "HEAD")
	repo, _ := e.open(rs.mine)
	scopes, err := repo.HistoryScopes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range scopes {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != ",local-only,main,origin/main,origin/topic" {
		t.Fatalf("scopes %q", names)
	}
	find := func(scope HistoryScope, text string) string {
		cs, _ := search(t, repo, HistoryQuery{Scopes: []HistoryScope{scope}, Field: FieldMessage, Text: text}, 0, 200)
		return subjectsOf(cs)
	}
	if find(scopes[0], "work") != "" || find(scopes[1], "work") != "Local work" || find(scopes[4], "work") != "Topic work" {
		t.Fatal("scope not searched")
	}
	// Browsing another line changes nothing.
	if e.rev(rs.mine, "HEAD") != head || strings.TrimSpace(e.git(rs.mine, "branch", "--show-current")) != "main" {
		t.Fatal("search switched branches")
	}
	e.git(rs.mine, "branch", "-D", "local-only")
	_, err = repo.ResolveQuery(context.Background(), HistoryQuery{Scopes: []HistoryScope{scopes[1]}})
	wantCode(t, err, CodeScopeMissing)
}

func TestSearchEmptyQueryAndUnborn(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	repo, _ := e.open(dir)
	if cs, more := search(t, repo, HistoryQuery{Field: FieldAuthor, Text: "x"}, 0, 200); len(cs) != 0 || more {
		t.Fatal("unborn history")
	}
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "One")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Two")
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldMessage, Text: "  "}, 0, 200); subjectsOf(cs) != "Two|One" {
		t.Fatalf("blank query %q", subjectsOf(cs))
	}
}

func TestSearchHashPrefix(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	// Fixed dates make the hashes the same in every run, so the history
	// below always has commits sharing a 4-digit prefix.
	env := e.Runner.Env
	e.Runner.Env = append(append([]string{}, env...), "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z")
	seen := map[string]bool{}
	shared := false
	for i := 0; i < 300 || !shared && i < 5000; i++ {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "C "+strconv.Itoa(i))
		p := e.rev(dir, "HEAD")[:4]
		shared = shared || seen[p]
		seen[p] = true
	}
	e.Runner.Env = env
	repo, _ := e.open(dir)
	// The oldest commit, far beyond the first page, by a short prefix.
	oldest := strings.TrimSpace(e.git(dir, "rev-list", "--max-parents=0", "HEAD"))
	cs, _ := search(t, repo, HistoryQuery{Field: FieldHash, Text: strings.ToUpper(oldest[:8])}, 0, 200)
	if subjectsOf(cs) != "C 0" {
		t.Fatalf("by prefix %q", subjectsOf(cs))
	}
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldHash, Text: oldest}, 0, 200); len(cs) != 1 {
		t.Fatal("full hash")
	}
	// Every commit sharing a 4-digit prefix is listed, newest first.
	all := strings.Fields(e.git(dir, "rev-list", "HEAD"))
	counts := map[string]int{}
	for _, h := range all {
		counts[h[:4]]++
	}
	prefix := ""
	for p, n := range counts {
		if n > 1 {
			prefix = p
			break
		}
	}
	if prefix == "" {
		t.Fatal("no shared 4-digit prefix: the fixed history changed")
	}
	cs, _ = search(t, repo, HistoryQuery{Field: FieldHash, Text: prefix}, 0, 200)
	if len(cs) != counts[prefix] {
		t.Fatalf("prefix %s: %d of %d", prefix, len(cs), counts[prefix])
	}
	// In history order: as rev-list lists them.
	var want []string
	for _, h := range all {
		if strings.HasPrefix(h, prefix) {
			want = append(want, h)
		}
	}
	for i, c := range cs {
		if c.Hash != want[i] {
			t.Fatalf("order: %d is %s, want %s", i, c.Hash, want[i])
		}
	}
}

func TestSearchHashOnlyInScope(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Base")
	e.git(dir, "switch", "-q", "-c", "side")
	e.git(dir, "commit", "-q", "--allow-empty", "-m", "Side only")
	side := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	e.git(dir, "switch", "-q", "main")
	repo, _ := e.open(dir)
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldHash, Text: side[:7]}, 0, 200); len(cs) != 0 {
		t.Fatal("found a commit outside HEAD")
	}
	scope := HistoryScope{Ref: "refs/heads/side", Name: "side"}
	if cs, _ := search(t, repo, HistoryQuery{Scopes: []HistoryScope{scope}, Field: FieldHash, Text: side[:7]}, 0, 200); subjectsOf(cs) != "Side only" {
		t.Fatalf("in side %q", subjectsOf(cs))
	}
	// A tree or blob with the prefix is not a commit.
	tree := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD^{tree}"))
	if cs, _ := search(t, repo, HistoryQuery{Field: FieldHash, Text: tree[:10]}, 0, 200); len(cs) != 0 {
		t.Fatal("tree listed")
	}
	for _, bad := range []string{"abc", "xyz123", "12 34", "-all", strings.Repeat("a", 65)} {
		_, err := repo.ResolveQuery(context.Background(), HistoryQuery{Field: FieldHash, Text: bad})
		wantCode(t, err, CodeInvalidHash)
	}
}

func TestSearchPath(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.commitAs(dir, "src/app/main.go", "A", "a@x", "Main")
	e.commitAs(dir, "src/app.go", "A", "a@x", "Sibling")
	e.commitAs(dir, "docs/*.md", "A", "a@x", "Star file")
	e.commitAs(dir, "docs/readme.md", "A", "a@x", "Readme")
	e.commitAs(dir, "dir with space/-x :y.txt", "A", "a@x", "Odd name")
	for i := range 210 {
		e.git(dir, "commit", "-q", "--allow-empty", "-m", "Filler "+strconv.Itoa(i))
	}
	e.commitAs(dir, "src/app/util.go", "A", "a@x", "Util")
	repo, _ := e.open(dir)
	find := func(p string) string {
		cs, _ := search(t, repo, HistoryQuery{Field: FieldPath, Text: p}, 0, 200)
		return subjectsOf(cs)
	}
	if got := find("src/app"); got != "Util|Main" {
		t.Fatalf("folder %q", got)
	}
	if got := find("src/app/"); got != "Util|Main" {
		t.Fatalf("folder with slash %q", got)
	}
	if got := find("docs/*.md"); got != "Star file" {
		t.Fatalf("literal star %q", got)
	}
	if got := find("dir with space/-x :y.txt"); got != "Odd name" {
		t.Fatalf("odd name %q", got)
	}
	if got := find(filepath.Join(dir, "src", "app.go")); got != "Sibling" {
		t.Fatalf("absolute %q", got)
	}
	if got := find("./src/../docs/readme.md"); got != "Readme" {
		t.Fatalf("dotted %q", got)
	}
	if got := find("HEAD"); got != "" {
		t.Fatalf("revision-like path %q", got)
	}
	for _, bad := range []string{"../outside", "/etc/passwd"} {
		_, err := repo.ResolveQuery(context.Background(), HistoryQuery{Field: FieldPath, Text: bad})
		wantCode(t, err, CodeInvalidPath)
	}
	// The detail of a commit found by path still lists all its files.
	e.git(dir, "rm", "-q", "docs/readme.md")
	write(t, filepath.Join(dir, "src", "app", "main.go"), "both\n")
	e.git(dir, "add", "-A")
	e.git(dir, "commit", "-q", "-m", "Both")
	cs, _ := search(t, repo, HistoryQuery{Field: FieldPath, Text: "src/app/main.go"}, 0, 200)
	d, err := repo.Detail(context.Background(), cs[0])
	if err != nil || len(d.Files) != 2 {
		t.Fatalf("detail %+v %v", d.Files, err)
	}
}
