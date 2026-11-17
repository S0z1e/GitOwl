package workflow

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"gitowl/internal/git"
)

const (
	// CodeNoRemote: the repository has no remote to fetch from.
	CodeNoRemote Code = "NoRemote"
	// CodeRemoteNotFound: the named remote is not configured (any more).
	CodeRemoteNotFound Code = "RemoteNotFound"
	// CodeRemoteExists: a remote with that name is already configured.
	CodeRemoteExists Code = "RemoteExists"
	// CodeInvalidRemoteName: the remote name is empty, padded, or Git rejects it.
	CodeInvalidRemoteName Code = "InvalidRemoteName"
	// CodeInvalidRemoteURL: the remote URL is empty.
	CodeInvalidRemoteURL Code = "InvalidRemoteURL"
	// CodeChooseRemote: there are several remotes and none is preferred.
	CodeChooseRemote Code = "ChooseRemote"
	// CodeAuthFailed: the remote refused the credentials, or none were
	// available without a prompt.
	CodeAuthFailed Code = "AuthFailed"
	// CodeHostKey: SSH could not verify the remote host's key.
	CodeHostKey Code = "HostKey"
	// CodeNetwork: the remote could not be reached.
	CodeNetwork Code = "Network"
	// CodeRemoteRepositoryMissing: the remote has no such repository, or
	// does not let this account see it.
	CodeRemoteRepositoryMissing Code = "RemoteRepositoryMissing"
	// CodeTimedOut: a remote operation took longer than its limit.
	CodeTimedOut Code = "TimedOut"
)

// DefaultNetworkTimeout bounds one fetch, push or pull.
const DefaultNetworkTimeout = 120 * time.Second

// Remote is a configured remote. URL has credentials removed.
type Remote struct {
	Name string
	URL  string
}

// RemoteInfo is what GitOwl knows about the remotes of the open worktree's
// repository and its current branch.
type RemoteInfo struct {
	Remotes []Remote
	// UpstreamRemote and UpstreamBranch are the current branch's
	// configured upstream, when it is on a configured remote.
	UpstreamRemote, UpstreamBranch string
	// FetchedAt is when GitOwl last fetched the repository successfully in
	// this run; zero when it has not.
	FetchedAt time.Time
	// Branch is the current branch, "" when HEAD is detached.
	Branch string
	// Push is where the current branch is pushed, nil when it has no
	// upstream on a configured remote and must be published first.
	Push *PushTarget
	// PushDefault is remote.pushDefault.
	PushDefault string
}

// Has reports a configured remote named name.
func (ri RemoteInfo) Has(name string) bool {
	return slices.ContainsFunc(ri.Remotes, func(r Remote) bool { return r.Name == name })
}

// DefaultFetch returns the remote Fetch uses without a choice: the
// upstream's, the only one, or origin. It returns "" when the user must
// choose, or when there is none.
func (ri RemoteInfo) DefaultFetch() string {
	switch {
	case ri.UpstreamRemote != "" && ri.Has(ri.UpstreamRemote):
		return ri.UpstreamRemote
	case len(ri.Remotes) == 1:
		return ri.Remotes[0].Name
	case ri.Has("origin"):
		return "origin"
	}
	return ""
}

// SetNetworkTimeout changes the limit of one remote operation; zero
// restores DefaultNetworkTimeout.
func (a *Application) SetNetworkTimeout(d time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.netTimeout = d
}

func (a *Application) networkTimeout() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.netTimeout > 0 {
		return a.netTimeout
	}
	return DefaultNetworkTimeout
}

func (a *Application) fetchedAt(common string) time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fetched[common]
}

func (a *Application) markFetched(common string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.fetched == nil {
		a.fetched = map[string]time.Time{}
	}
	a.fetched[common] = time.Now()
}

// Remotes reads the configured remotes and the current branch's upstream.
func (r *Repository) Remotes(ctx context.Context) (RemoteInfo, error) {
	info := RemoteInfo{FetchedAt: r.app.fetchedAt(r.common)}
	out, err := r.run(ctx, "remote")
	if err != nil {
		return info, err
	}
	urls := map[string]string{}
	if cfg, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, "config", "-z", "--get-regexp", `^remote\..*\.url$`); err == nil {
		for entry := range strings.SplitSeq(string(cfg), "\x00") {
			key, value, ok := strings.Cut(entry, "\n")
			if !ok {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
			if _, seen := urls[name]; !seen {
				urls[name] = git.Redact(value)
			}
		}
	}
	for name := range strings.Lines(string(out)) {
		name = strings.TrimSpace(name)
		if name != "" {
			info.Remotes = append(info.Remotes, Remote{Name: name, URL: urls[name]})
		}
	}
	branch, err := r.run(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if CodeOf(err) == CodeGitMissing || CodeOf(err) == CodeCancelled {
			return info, err
		}
		return info, nil // detached
	}
	b := strings.TrimSpace(string(branch))
	info.Branch = b
	info.PushDefault = r.config(ctx, "remote.pushDefault")
	remote := r.config(ctx, "branch."+b+".remote")
	merge := r.config(ctx, "branch."+b+".merge")
	if remote != "" && info.Has(remote) && strings.HasPrefix(merge, "refs/heads/") {
		info.UpstreamRemote, info.UpstreamBranch = remote, strings.TrimPrefix(merge, "refs/heads/")
	}
	r.pushTarget(ctx, &info)
	return info, nil
}

// config returns a configuration value, "" when unset or unreadable.
func (r *Repository) config(ctx context.Context, key string) string {
	out, _, err := r.app.git.RunStatus(ctx, r.root, nil, []int{1}, "config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// network runs a remote operation within the network time limit.
func (r *Repository) network(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.app.networkTimeout())
	defer cancel()
	out, err := r.app.git.RunNetwork(ctx, r.root, args...)
	return out, networkFailure(ctx, err)
}

// networkFailure explains a failed remote operation.
func networkFailure(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) && errors.Is(err, context.Canceled):
		return fail(CodeTimedOut, err)
	case errors.Is(err, context.Canceled):
		return fail(CodeCancelled, err)
	}
	var g *git.Error
	if !errors.As(err, &g) {
		return gitFailure(err)
	}
	msg := strings.ToLower(g.Message())
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(msg, p) {
				return true
			}
		}
		return false
	}
	switch {
	case has("host key verification failed", "remote host identification has changed", "no matching host key", "host key for"):
		return fail(CodeHostKey, err)
	case has("authentication failed", "could not read username", "could not read password", "terminal prompts disabled",
		"permission denied (publickey", "permission denied, please try again", "invalid username or password",
		"http basic: access denied", "the requested url returned error: 401", "the requested url returned error: 403",
		"permission to ", "access denied", "bad credentials"):
		return fail(CodeAuthFailed, err)
	case has("repository not found", "does not appear to be a git repository", "the requested url returned error: 404",
		"not found", "no such file or directory"):
		return fail(CodeRemoteRepositoryMissing, err)
	case has("could not resolve host", "could not resolve hostname", "connection refused", "connection timed out",
		"operation timed out", "network is unreachable", "failed to connect", "connection reset", "no route to host",
		"unable to access", "could not connect", "connection closed", "early eof", "ssl", "proxy"):
		return fail(CodeNetwork, err)
	}
	return fail(CodeGitFailed, err)
}

// checkRemote returns an error unless name is a configured remote.
func (r *Repository) checkRemote(ctx context.Context, name string) (RemoteInfo, error) {
	info, err := r.Remotes(ctx)
	if err == nil && ctx.Err() != nil {
		err = fail(CodeCancelled, ctx.Err())
	}
	if err != nil {
		return info, err
	}
	switch {
	case len(info.Remotes) == 0:
		return info, &Error{Code: CodeNoRemote}
	case name == "":
		return info, &Error{Code: CodeChooseRemote}
	case !info.Has(name) || strings.HasPrefix(name, "-"):
		return info, &Error{Code: CodeRemoteNotFound, Detail: name}
	}
	return info, nil
}

// afterNetwork reads the state after a remote operation, whatever its
// result, with a lifetime of its own: the operation's context may have
// been cancelled.
func (r *Repository) afterNetwork() (Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return r.read(ctx)
}

// AddRemote records a remote in Git's configuration. The URL is stored as
// given; Remotes redacts credentials when the interface reads it back.
func (r *Repository) AddRemote(ctx context.Context, name, url string) (RemoteInfo, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return RemoteInfo{}, &Error{Code: CodeRepositoryClosed}
	}
	if strings.TrimSpace(name) != name || name == "" || strings.HasPrefix(name, "-") {
		return RemoteInfo{}, &Error{Code: CodeInvalidRemoteName, Detail: name}
	}
	if _, err := r.run(ctx, "check-ref-format", "refs/remotes/"+name+"/x"); err != nil {
		return RemoteInfo{}, fail(CodeInvalidRemoteName, err)
	}
	if strings.TrimSpace(url) == "" {
		return RemoteInfo{}, &Error{Code: CodeInvalidRemoteURL, Detail: url}
	}
	info, err := r.Remotes(ctx)
	if err != nil {
		return info, err
	}
	if info.Has(name) {
		return info, &Error{Code: CodeRemoteExists, Detail: name}
	}
	if _, err := r.run(ctx, "remote", "add", "--", name, url); err != nil {
		return info, err
	}
	return r.Remotes(ctx)
}

// Fetch updates the remote-tracking branches of remote. It changes no
// files, no index and no local branch. The state read afterwards is
// returned even when the fetch failed, as it may have updated some refs.
func (r *Repository) Fetch(ctx context.Context, remote string) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	if _, err := r.checkRemote(ctx, remote); err != nil {
		return Snapshot{}, err
	}
	return r.fetchNetwork(ctx, "--", remote)
}

// FetchAll updates the remote-tracking branches of every configured remote,
// with the same limits as Fetch. It fails with CodeNoRemote when there is
// no remote at all.
func (r *Repository) FetchAll(ctx context.Context) (Snapshot, error) {
	defer r.lockRepository()()
	if !r.Current() {
		return Snapshot{}, &Error{Code: CodeRepositoryClosed}
	}
	info, err := r.Remotes(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if len(info.Remotes) == 0 {
		return Snapshot{}, &Error{Code: CodeNoRemote}
	}
	return r.fetchNetwork(ctx, "--all")
}

// fetchNetwork runs git fetch with args and reads the state afterwards. The
// caller holds the repository lock.
func (r *Repository) fetchNetwork(ctx context.Context, args ...string) (Snapshot, error) {
	_, err := r.network(ctx, append([]string{"fetch", "--no-recurse-submodules", "--no-write-fetch-head"}, args...)...)
	if err == nil {
		r.app.markFetched(r.common)
	}
	snap, rerr := r.afterNetwork()
	if err == nil {
		err = rerr
	}
	return snap, err
}
