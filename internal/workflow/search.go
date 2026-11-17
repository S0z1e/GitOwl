package workflow

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"gitowl/internal/git"
)

const (
	// CodeScopeMissing: the branch a history search looks in no longer
	// exists.
	CodeScopeMissing Code = "ScopeMissing"
	// CodeInvalidHash: a hash search is not 4 to 64 hexadecimal digits.
	CodeInvalidHash Code = "InvalidHash"
	// CodeInvalidPath: a path search leaves the repository.
	CodeInvalidPath Code = "InvalidPath"
)

// HistoryField is what a history search looks at.
type HistoryField uint8

const (
	// FieldMessage matches the subject and body of commit messages.
	FieldMessage HistoryField = iota
	// FieldAuthor matches author names and email addresses.
	FieldAuthor
	// FieldHash matches commit hashes by prefix.
	FieldHash
	// FieldPath matches commits that changed a file, or anything in a
	// folder. The History search does not offer it.
	FieldPath
)

// HistoryFields lists the criteria the History search offers, in the order
// the search shows them: commit message, commit hash, author.
var HistoryFields = []HistoryField{FieldMessage, FieldHash, FieldAuthor}

// HistoryScope is a line of history to look in: HEAD, a local branch or a
// remote-tracking branch as last fetched.
type HistoryScope struct {
	// Ref is the full ref name, "" for HEAD.
	Ref string
	// Name is the short name, as main or origin/main; "" for HEAD.
	Name   string
	Remote bool
}

// HistoryScopes lists HEAD, the local branches and the remote-tracking
// branches, without symbolic refs such as origin/HEAD.
func (r *Repository) HistoryScopes(ctx context.Context) ([]HistoryScope, error) {
	scopes := []HistoryScope{{}}
	b, err := r.Branches(ctx)
	if err != nil {
		return scopes, err
	}
	for _, l := range b.Local {
		scopes = append(scopes, HistoryScope{Ref: "refs/heads/" + l.Name, Name: l.Name})
	}
	for _, rb := range b.Remote {
		scopes = append(scopes, HistoryScope{Ref: "refs/remotes/" + rb.Name, Name: rb.Name, Remote: true})
	}
	return scopes, nil
}

// HistoryQuery is a search of history: text in one field, within the shown
// branches, in an order. Empty text lists every shown commit.
type HistoryQuery struct {
	// Scopes are the branches whose history is searched; HEAD when empty.
	Scopes []HistoryScope
	Order  HistoryOrder
	Field  HistoryField
	Text   string
}

// Searching reports a query with text to match.
func (q HistoryQuery) Searching() bool { return strings.TrimSpace(q.Text) != "" }

// Equal reports whether two queries show the same commits in the same order.
func (q HistoryQuery) Equal(o HistoryQuery) bool {
	return q.Field == o.Field && q.Text == o.Text && q.Order == o.Order && slices.Equal(q.Scopes, o.Scopes)
}

// ResolvedQuery is a query fixed to the commits its scopes pointed at when
// it was resolved, so its pages stay consistent while refs move.
type ResolvedQuery struct {
	HistoryQuery
	// Starts are the distinct commits the scopes pointed at, in scope
	// order; empty for a HEAD without commits.
	Starts []string
	// Refs are the scopes' ref names, "HEAD" for the checked-out branch.
	Refs []string
	// path is FieldPath's text as a path relative to the repository.
	path string
}

// ResolveQuery fixes q's scope to a commit and checks its text.
func (r *Repository) ResolveQuery(ctx context.Context, q HistoryQuery) (ResolvedQuery, error) {
	rq := ResolvedQuery{HistoryQuery: q}
	text := strings.TrimSpace(q.Text)
	rq.Text = text
	if text != "" {
		switch q.Field {
		case FieldHash:
			if !validHashPrefix(text) {
				return rq, &Error{Code: CodeInvalidHash, Detail: text}
			}
			rq.Text = strings.ToLower(text)
		case FieldPath:
			p, err := r.repoPath(text)
			if err != nil {
				return rq, err
			}
			rq.path = p
		}
	}
	starts, err := r.resolveScopes(ctx, q.Scopes)
	if err != nil {
		return rq, err
	}
	rq.Starts = starts
	for _, s := range q.Scopes {
		if s.Ref == "" {
			rq.Refs = append(rq.Refs, "HEAD")
		} else {
			rq.Refs = append(rq.Refs, s.Ref)
		}
	}
	return rq, nil
}

// resolveScopes returns the distinct commits the scopes point at, in scope
// order. No scopes means HEAD. HEAD without a commit (an unborn branch)
// resolves to none; a named branch that no longer exists is CodeScopeMissing.
func (r *Repository) resolveScopes(ctx context.Context, scopes []HistoryScope) ([]string, error) {
	if len(scopes) == 0 {
		scopes = []HistoryScope{{}}
	}
	// Every named branch is looked up in one read of the refs: a git call per
	// branch made a view of hundreds of branches take seconds.
	var refs map[string]string
	for _, s := range scopes {
		if s.Ref != "" {
			var err error
			if refs, err = r.refCommits(ctx); err != nil {
				return nil, err
			}
			break
		}
	}
	var starts []string
	seen := map[string]bool{}
	for _, s := range scopes {
		var tip string
		if s.Ref == "" {
			t, err := r.verifyRef(ctx, "HEAD")
			if err != nil {
				return nil, err
			}
			tip = t
		} else {
			if !strings.HasPrefix(s.Ref, "refs/") {
				return nil, &Error{Code: CodeScopeMissing, Detail: s.Ref}
			}
			tip = refs[s.Ref]
			if tip == "" {
				return nil, &Error{Code: CodeScopeMissing, Detail: s.Name}
			}
		}
		if tip == "" {
			continue // HEAD without a commit yet
		}
		if !seen[tip] {
			seen[tip] = true
			starts = append(starts, tip)
		}
	}
	return starts, nil
}

// refCommits maps the full name of every local and remote-tracking branch to
// the commit it points at, read with one git call.
func (r *Repository) refCommits(ctx context.Context) (map[string]string, error) {
	out, err := r.run(ctx, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for line := range strings.Lines(string(out)) {
		if f := strings.Split(strings.TrimSuffix(line, "\n"), "\x00"); len(f) == 2 {
			refs[f[0]] = f[1]
		}
	}
	return refs, nil
}

// validHashPrefix accepts 4 to 64 hexadecimal digits, the longest a
// SHA-256 repository's hashes are.
func validHashPrefix(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

// repoPath turns a path typed for a search into a path relative to the
// repository root: relative paths are already, absolute ones must be
// inside the worktree. Trailing slashes are dropped; a folder matches
// everything in it.
func (r *Repository) repoPath(text string) (string, error) {
	p := strings.ReplaceAll(text, "\\", "/")
	if strings.HasPrefix(p, "/") {
		root := r.root + "/"
		if !strings.HasPrefix(p+"/", root) {
			return "", &Error{Code: CodeInvalidPath, Detail: text}
		}
		p = strings.TrimPrefix(p+"/", root)
	}
	var parts []string
	for _, s := range strings.Split(p, "/") {
		switch s {
		case "", ".":
		case "..":
			if len(parts) == 0 {
				return "", &Error{Code: CodeInvalidPath, Detail: text}
			}
			parts = parts[:len(parts)-1]
		default:
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "", nil // the whole repository
	}
	return strings.Join(parts, "/"), nil
}

// SearchHistory returns up to limit commits of rq after skipping skip, in
// the order git log lists them from rq.Start, and whether more follow. It
// searches all of the history reachable from the start, not a page of it.
func (r *Repository) SearchHistory(ctx context.Context, rq ResolvedQuery, skip, limit int) ([]CommitInfo, bool, error) {
	if len(rq.Starts) == 0 {
		return nil, false, nil
	}
	if rq.Text != "" && rq.Field == FieldHash {
		return r.searchHash(ctx, rq, skip, limit)
	}
	args := []string{"log", "-z", "--no-color", "--no-notes", rq.Order.flag(), "--format=" + git.LogFormat,
		"--skip=" + strconv.Itoa(skip), "--max-count=" + strconv.Itoa(limit+1)}
	var paths []string
	if rq.Text != "" {
		switch rq.Field {
		case FieldMessage:
			args = append(args, "--regexp-ignore-case", "--fixed-strings", "--grep="+rq.Text)
		case FieldAuthor:
			args = append(args, "--regexp-ignore-case", "--fixed-strings", "--author="+rq.Text)
		case FieldPath:
			if rq.path != "" {
				args = append([]string{"--literal-pathspecs"}, args...)
				paths = []string{rq.path}
			}
		}
	}
	args = append(append(append(args, rq.Starts...), "--"), paths...)
	out, err := r.run(ctx, args...)
	if err != nil {
		return nil, false, err
	}
	return pageOf(out, limit)
}

// searchHash lists the commits of rq's scope whose hash starts with
// rq.Text: every one, in the order git rev-list walks them, when several
// do.
func (r *Repository) searchHash(ctx context.Context, rq ResolvedQuery, skip, limit int) ([]CommitInfo, bool, error) {
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{128}, "rev-parse", "--disambiguate="+rq.Text)
	if err != nil {
		return nil, false, gitFailure(err)
	}
	candidates := map[string]bool{}
	for line := range strings.Lines(string(out)) {
		if h := strings.TrimSpace(line); h != "" {
			candidates[h] = true
		}
	}
	if len(candidates) == 0 {
		return nil, false, nil
	}
	// Keep those reachable from the start, in history order. Trees and
	// blobs with the prefix are never in the walk.
	walk, err := r.run(ctx, append(append([]string{"rev-list"}, rq.Starts...), "--")...)
	if err != nil {
		return nil, false, err
	}
	var found []string
	for line := range strings.Lines(string(walk)) {
		if h := strings.TrimSpace(line); candidates[h] {
			found = append(found, h)
			if len(found) == len(candidates) {
				break
			}
		}
	}
	if skip >= len(found) {
		return nil, false, nil
	}
	args := append([]string{"log", "-z", "--no-color", "--no-notes", "--no-walk=unsorted", "--format=" + git.LogFormat}, found...)
	out, err = r.run(ctx, append(args, "--")...)
	if err != nil {
		return nil, false, err
	}
	commits, _, err := pageOf(out, len(found))
	if err != nil {
		return nil, false, err
	}
	commits = commits[min(skip, len(commits)):]
	more := len(commits) > limit
	if more {
		commits = commits[:limit]
	}
	return commits, more, nil
}

// pageOf parses git log output of up to limit+1 commits.
func pageOf(out []byte, limit int) ([]CommitInfo, bool, error) {
	entries, err := git.ParseLog(out)
	if err != nil {
		return nil, false, fail(CodeGitFailed, err)
	}
	more := len(entries) > limit
	if more {
		entries = entries[:limit]
	}
	commits := make([]CommitInfo, len(entries))
	for i, e := range entries {
		commits[i] = CommitInfo(e)
	}
	return commits, more, nil
}
