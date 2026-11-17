package workflow

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"

	"gitowl/internal/git"
)

// RecentKind is the outcome of reading a recent repository's status without
// opening it. It tells the welcome list whether the folder is still a
// readable repository and, if not, why.
type RecentKind uint8

const (
	// RecentOK means the folder is a Git working tree GitOwl could read.
	RecentOK RecentKind = iota
	// RecentMissing means the folder is gone.
	RecentMissing
	// RecentNotRepository means the folder exists but is not a Git working
	// tree (a plain folder, or a bare repository).
	RecentNotRepository
	// RecentPermissionDenied means the folder could not be read for lack of
	// permission.
	RecentPermissionDenied
	// RecentFailed means Git ran and failed, or its output did not parse.
	RecentFailed
)

// RecentStatus is a recent repository's state as read for the welcome list,
// without opening it or taking any lock. It reads only: it never fetches,
// nor changes refs, the index or files.
type RecentStatus struct {
	// Kind is the outcome; the other fields are meaningful only when it is
	// RecentOK.
	Kind RecentKind
	// Branch is the checked-out branch, empty when HEAD is detached or the
	// repository has no commits.
	Branch string
	// Detached reports a detached HEAD; Short is then its short commit hash.
	Detached bool
	Short    string
	// Unborn reports a repository without commits.
	Unborn bool
	// Modified is the number of distinct paths that differ from HEAD,
	// counting a path staged and unstaged once and including untracked
	// files.
	Modified int
}

// RecentStatus reads the status of the repository at path for the welcome
// list, without opening it, taking the open mutex or any worktree lock, so
// several calls may run at once. It is read only.
func (a *Application) RecentStatus(ctx context.Context, path string) RecentStatus {
	if path == "" {
		return RecentStatus{Kind: RecentMissing}
	}
	switch kind, ok := a.recentCheckPath(path); ok {
	case false:
		return RecentStatus{Kind: kind}
	}
	if a.git == nil {
		return RecentStatus{Kind: RecentFailed}
	}
	out, err := a.git.Run(ctx, path, nil, git.StatusArgs...)
	if err != nil {
		return recentGitError(err)
	}
	st, err := git.ParseStatus(out)
	if err != nil {
		return RecentStatus{Kind: RecentFailed}
	}
	return recentFromStatus(st)
}

// recentCheckPath reports the folder's own problems before Git runs: it
// returns ok=false with the kind when path is missing, unreadable or not a
// directory, and ok=true when Git should be asked.
func (a *Application) recentCheckPath(path string) (RecentKind, bool) {
	st, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return RecentMissing, false
	case errors.Is(err, fs.ErrPermission):
		return RecentPermissionDenied, false
	case err != nil:
		return RecentFailed, false
	case !st.IsDir():
		return RecentNotRepository, false
	}
	if _, err := os.ReadDir(path); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return RecentPermissionDenied, false
		}
		return RecentFailed, false
	}
	return RecentOK, true
}

// recentGitError classifies a failed git status for the welcome list.
func recentGitError(err error) RecentStatus {
	var g *git.Error
	if errors.As(err, &g) {
		msg := g.Message()
		switch {
		case strings.Contains(msg, "not a git repository"),
			strings.Contains(msg, "must be run in a work tree"),
			strings.Contains(msg, "this operation must be run in a work tree"):
			return RecentStatus{Kind: RecentNotRepository}
		case strings.Contains(msg, "Permission denied"),
			strings.Contains(msg, "permission denied"):
			return RecentStatus{Kind: RecentPermissionDenied}
		}
	}
	return RecentStatus{Kind: RecentFailed}
}

// recentFromStatus turns parsed status into a RecentStatus, counting each
// changed path once.
func recentFromStatus(st git.Status) RecentStatus {
	r := RecentStatus{Kind: RecentOK, Branch: st.Branch, Detached: st.Detached}
	if st.Unborn() {
		r.Unborn = true
	} else if st.Detached {
		r.Short = shortHash(st.Head)
	}
	seen := make(map[string]struct{}, len(st.Entries))
	for _, e := range st.Entries {
		seen[e.Path] = struct{}{}
	}
	r.Modified = len(seen)
	return r
}

// shortHash returns a fixed-length prefix of a full commit hash, matching
// Git's usual abbreviation for display.
func shortHash(head string) string {
	if len(head) > 7 {
		return head[:7]
	}
	return head
}
