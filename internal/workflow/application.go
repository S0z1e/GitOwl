// Package workflow is GitOwl's application workflow: the one boundary through
// which the interface reads and changes repositories and asks for commit
// message drafts. Every method may block on Git or the network and must be
// called off the UI thread.
package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitowl/internal/git"
)

// Application coordinates the open repository. Opening another repository
// closes the previous one: its handles report CodeRepositoryClosed for
// changes, and the interface discards their results.
type Application struct {
	git *git.Runner
	ai  AIConfig

	mu      sync.Mutex
	next    uint64
	current *Repository
	// closed is the last open request number when the repository was
	// closed: no request up to it may open a repository afterwards.
	closed uint64
	// locks serialize GitOwl's changes per worktree root, across every
	// opening of it: a handle opened again for the same directory waits for
	// the previous one's change to finish.
	locks map[string]*sync.Mutex
	// netTimeout bounds one remote operation; zero is the default.
	netTimeout time.Duration
	// fetched is when each repository, by shared Git directory, was last
	// fetched successfully in this run.
	fetched map[string]time.Time
}

// lock returns the mutex for key, creating it once.
func (a *Application) lock(key string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.locks == nil {
		a.locks = map[string]*sync.Mutex{}
	}
	m, ok := a.locks[key]
	if !ok {
		m = &sync.Mutex{}
		a.locks[key] = m
	}
	return m
}

// New returns a workflow running Git through r; a nil r reports
// CodeGitMissing for every repository.
func New(r *git.Runner) *Application { return &Application{git: r} }

// Repository is one opening of a repository. Each Open returns a new handle,
// even for the same path.
type Repository struct {
	app  *Application
	id   uint64
	root string
	// common is the Git directory the repository's worktrees share; it
	// identifies the repository across them.
	common string
}

// Common returns the shared Git directory, which is the same for every
// worktree of one repository.
func (r *Repository) Common() string { return r.common }

// lockRepository serializes a change to what the worktrees share, such as
// branches and worktree records, with GitOwl's other such changes, and
// with changes to this worktree. It returns the unlock function.
func (r *Repository) lockRepository() func() {
	shared := r.app.lock("repository:" + r.common)
	shared.Lock()
	unlock := r.lockWorktree()
	return func() {
		unlock()
		shared.Unlock()
	}
}

// lockWorktree serializes a change to this worktree's index or files with
// every other change GitOwl makes there, and returns the unlock function.
func (r *Repository) lockWorktree() func() {
	m := r.app.lock("worktree:" + r.root)
	m.Lock()
	return m.Unlock
}

// Root returns the repository's working tree root.
func (r *Repository) Root() string { return r.root }

// Name returns the root directory's base name.
func (r *Repository) Name() string { return filepath.Base(r.root) }

// Current reports whether r is still the open repository.
func (r *Repository) Current() bool {
	if r == nil {
		return false
	}
	r.app.mu.Lock()
	defer r.app.mu.Unlock()
	return r.app.current == r
}

// Current returns the open repository, or nil.
func (a *Application) Current() *Repository {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current
}

// Open opens the repository containing path and makes it the current one.
// When it fails, the previous repository stays open. When a later Open
// finishes first, this one reports CodeRepositoryClosed.
func (a *Application) Open(ctx context.Context, path string) (*Repository, Snapshot, error) {
	return a.OpenRequested(ctx, a.RequestOpen(), path)
}

// OpenRequest is an open asked for at one moment: a later Open that
// finishes first, or Close, makes it stale.
type OpenRequest uint64

// RequestOpen numbers an open request now, as the user asks for it, so a
// Close before the open runs makes it stale.
func (a *Application) RequestOpen() OpenRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.next++
	return OpenRequest(a.next)
}

// OpenRequested is Open for a request numbered earlier with RequestOpen.
func (a *Application) OpenRequested(ctx context.Context, req OpenRequest, path string) (*Repository, Snapshot, error) {
	id := uint64(req)

	root, common, err := a.resolve(ctx, path)
	if err != nil {
		return nil, Snapshot{}, err
	}
	r := &Repository{app: a, id: id, root: root, common: common}
	snap, err := r.read(ctx)
	if err != nil {
		return nil, Snapshot{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current != nil && a.current.id > id || id <= a.closed {
		return nil, Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	a.current = r
	return r, snap, nil
}

// Close closes the open repository without touching it: its files, index,
// branches and worktrees stay as they are. Its handle reports
// CodeRepositoryClosed for changes from now on, and so do opens requested
// before Close that have not finished.
func (a *Application) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.current = nil
	a.closed = a.next
}

// resolve returns the worktree root containing path and the repository's
// shared Git directory, both with symbolic links resolved.
func (a *Application) resolve(ctx context.Context, path string) (string, string, error) {
	if path == "" {
		return "", "", &Error{Code: CodePathNotFound}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", fail(CodePathNotFound, err)
	}
	st, err := os.Stat(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", "", fail(CodePathNotFound, err)
	case errors.Is(err, fs.ErrPermission):
		return "", "", fail(CodeNoAccess, err)
	case err != nil:
		return "", "", fail(CodeNoAccess, err)
	case !st.IsDir():
		return "", "", &Error{Code: CodeNotDirectory, Detail: abs}
	}
	if _, err := os.ReadDir(abs); err != nil {
		return "", "", fail(CodeNoAccess, err)
	}
	if a.git == nil {
		return "", "", &Error{Code: CodeGitMissing}
	}
	out, err := a.git.Run(ctx, abs, nil, "rev-parse", "--is-bare-repository", "--show-toplevel", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		var g *git.Error
		if errors.As(err, &g) {
			msg := g.Message()
			switch {
			case strings.Contains(msg, "not a git repository"):
				return "", "", fail(CodeNotRepository, err)
			case strings.Contains(msg, "must be run in a work tree"):
				bare, _ := a.git.Run(ctx, abs, nil, "rev-parse", "--is-bare-repository")
				if strings.TrimSpace(string(bare)) == "true" {
					return "", "", fail(CodeBareRepository, err)
				}
				return "", "", fail(CodeNotRepository, err)
			}
		}
		return "", "", gitFailure(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 3 || lines[0] == "true" {
		return "", "", &Error{Code: CodeBareRepository, Detail: abs}
	}
	return canonical(lines[1]), canonical(lines[2]), nil
}

// canonical resolves symbolic links in an absolute path, as /var to
// /private/var on macOS, so one directory has one name.
func canonical(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

// Change is one file in one area of the working tree.
type Change struct {
	Path, OrigPath string
	Kind           git.Kind
	Area           Area
	Submodule      bool
	Conflicted     bool
	// Added and Removed are line counts when a commit detail has counted
	// them. Both stay 0 for a working-tree change that has not been counted.
	Added, Removed int
	Binary         bool
	// indexHash is the index's object when the change was read; a change
	// whose index entry moved since is stale.
	indexHash string
}

// Area is where a working tree change lives.
type Area uint8

const (
	Staged Area = iota
	Unstaged
	Untracked
)

// Snapshot is the state of a repository's working tree as GitOwl last read it.
type Snapshot struct {
	Root string
	// Branch is empty when HEAD is detached.
	Branch   string
	Detached bool
	// Head is the checked-out commit, empty before the first commit.
	Head string
	// Upstream, Ahead and Behind compare with the remote-tracking branch
	// as last fetched, not with the remote itself.
	Upstream       string
	Ahead, Behind  int
	HasAheadBehind bool

	Staged, Unstaged, Untracked []Change
	// StagedFingerprint identifies the staged content: it changes whenever
	// HEAD or the index differs from HEAD differently.
	StagedFingerprint string
}

// Unborn reports a repository without commits.
func (s Snapshot) Unborn() bool { return s.Head == "" }

// Clean reports a working tree without changes.
func (s Snapshot) Clean() bool {
	return len(s.Staged) == 0 && len(s.Unstaged) == 0 && len(s.Untracked) == 0
}

// Status reads the repository's current state.
func (r *Repository) Status(ctx context.Context) (Snapshot, error) { return r.read(ctx) }

func (r *Repository) run(ctx context.Context, args ...string) ([]byte, error) {
	out, err := r.app.git.Run(ctx, r.root, nil, args...)
	return out, gitFailure(err)
}

func (r *Repository) read(ctx context.Context) (Snapshot, error) {
	out, err := r.run(ctx, git.StatusArgs...)
	if err != nil {
		return Snapshot{}, err
	}
	st, err := git.ParseStatus(out)
	if err != nil {
		return Snapshot{}, fail(CodeGitFailed, err)
	}
	snap := Snapshot{
		Root: r.root, Branch: st.Branch, Detached: st.Detached, Head: st.Head,
		Upstream: st.Upstream, Ahead: st.Ahead, Behind: st.Behind, HasAheadBehind: st.HasAheadBehind,
	}
	for _, e := range st.Entries {
		switch {
		case e.Untracked:
			snap.Untracked = append(snap.Untracked, Change{Path: e.Path, Kind: git.Untracked, Area: Untracked})
		case e.Conflicted:
			snap.Unstaged = append(snap.Unstaged, Change{Path: e.Path, Kind: git.Conflicted, Area: Unstaged, Conflicted: true})
		default:
			if e.Index != '.' {
				snap.Staged = append(snap.Staged, Change{Path: e.Path, OrigPath: e.OrigPath, Kind: e.IndexKind(), Area: Staged, Submodule: e.Submodule, indexHash: e.IndexHash})
			}
			if e.Worktree != '.' {
				snap.Unstaged = append(snap.Unstaged, Change{Path: e.Path, Kind: e.WorktreeKind(), Area: Unstaged, Submodule: e.Submodule, indexHash: e.IndexHash})
			}
		}
	}
	snap.StagedFingerprint, err = r.stagedFingerprint(ctx, st.Head)
	if err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// stagedFingerprint hashes HEAD and the blobs the index stages against it.
func (r *Repository) stagedFingerprint(ctx context.Context, head string) (string, error) {
	out, err := r.run(ctx, "diff", "--cached", "--raw", "-z", "--no-abbrev", "--no-renames", "--no-ext-diff")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(head))
	h.Write([]byte{0})
	h.Write(out)
	return hex.EncodeToString(h.Sum(nil)), nil
}
