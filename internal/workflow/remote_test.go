package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// remotes is a bare remote, the clone GitOwl opens and another clone that
// stands for a colleague pushing to the same remote.
type remotes struct {
	bare, mine, theirs string
}

func cloned(t *testing.T, e *env) remotes {
	t.Helper()
	seed := e.repo()
	write(t, filepath.Join(seed, "a.txt"), "a\n")
	e.commit(seed, "Initial")
	base := canonical(t.TempDir())
	r := remotes{bare: filepath.Join(base, "remote.git"), mine: filepath.Join(base, "mine"), theirs: filepath.Join(base, "theirs")}
	e.git(base, "clone", "-q", "--bare", seed, r.bare)
	e.git(base, "clone", "-q", r.bare, r.mine)
	e.git(base, "clone", "-q", r.bare, r.theirs)
	return r
}

// theirCommit commits and pushes a change from the other clone.
func (r remotes) theirCommit(t *testing.T, e *env, file, content string) string {
	t.Helper()
	write(t, filepath.Join(r.theirs, file), content)
	e.commit(r.theirs, "Their "+file)
	e.git(r.theirs, "push", "-q", "origin", "HEAD")
	return strings.TrimSpace(e.git(r.theirs, "rev-parse", "HEAD"))
}

func TestFetchUpdatesRemoteBranchesOnly(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	// Work in progress: a staged and an unstaged change.
	write(t, filepath.Join(rs.mine, "a.txt"), "staged\n")
	e.git(rs.mine, "add", "a.txt")
	write(t, filepath.Join(rs.mine, "a.txt"), "unstaged\n")
	index := e.git(rs.mine, "ls-files", "-s")
	head := e.git(rs.mine, "rev-parse", "HEAD")

	repo, snap := e.open(rs.mine)
	if snap.Behind != 0 {
		t.Fatalf("behind before fetch = %d", snap.Behind)
	}
	info, err := repo.Remotes(context.Background())
	if err != nil || info.DefaultFetch() != "origin" || info.UpstreamRemote != "origin" || info.UpstreamBranch != "main" || !info.FetchedAt.IsZero() {
		t.Fatalf("remotes = %+v, %v", info, err)
	}
	snap, err = repo.Fetch(context.Background(), "origin")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.HasAheadBehind || snap.Behind != 1 || snap.Ahead != 0 {
		t.Fatalf("after fetch: %+v", snap)
	}
	if got := strings.TrimSpace(e.git(rs.mine, "rev-parse", "origin/main")); got != theirs {
		t.Fatalf("origin/main = %s, want %s", got, theirs)
	}
	if e.git(rs.mine, "rev-parse", "HEAD") != head || e.git(rs.mine, "ls-files", "-s") != index || readFile(t, filepath.Join(rs.mine, "a.txt")) != "unstaged\n" {
		t.Fatal("fetch changed HEAD, the index or a file")
	}
	if _, err := os.Stat(filepath.Join(rs.mine, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("fetch wrote their file into the working tree")
	}
	info, _ = repo.Remotes(context.Background())
	if info.FetchedAt.IsZero() {
		t.Fatal("successful fetch not recorded")
	}
}

func TestFetchKeepsOtherWorktreeIndex(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	linked := filepath.Join(canonical(t.TempDir()), "linked")
	e.git(rs.mine, "worktree", "add", "-q", "-b", "side", linked)
	write(t, filepath.Join(linked, "a.txt"), "linked staged\n")
	e.git(linked, "add", "a.txt")
	index := e.git(linked, "ls-files", "-s")
	rs.theirCommit(t, e, "b.txt", "b\n")

	repo, _ := e.open(rs.mine)
	if _, err := repo.Fetch(context.Background(), "origin"); err != nil {
		t.Fatal(err)
	}
	if e.git(linked, "ls-files", "-s") != index {
		t.Fatal("fetch changed another worktree's index")
	}
	// The record of the fetch belongs to the repository, so it shows in
	// every worktree.
	other, _ := e.open(linked)
	if info, _ := other.Remotes(context.Background()); info.FetchedAt.IsZero() {
		t.Fatal("fetch time not shared between worktrees")
	}
}

func TestFetchDetachedAndUnborn(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	e.git(rs.mine, "switch", "-q", "--detach")
	theirs := rs.theirCommit(t, e, "b.txt", "b\n")
	repo, _ := e.open(rs.mine)
	if info, _ := repo.Remotes(context.Background()); info.UpstreamRemote != "" || info.DefaultFetch() != "origin" {
		t.Fatalf("detached remotes = %+v", info)
	}
	snap, err := repo.Fetch(context.Background(), "origin")
	if err != nil || !snap.Detached {
		t.Fatalf("detached fetch: %+v, %v", snap, err)
	}
	if got := strings.TrimSpace(e.git(rs.mine, "rev-parse", "origin/main")); got != theirs {
		t.Fatalf("origin/main = %s", got)
	}

	unborn := e.repo()
	e.git(unborn, "remote", "add", "origin", rs.bare)
	repo, _ = e.open(unborn)
	snap, err = repo.Fetch(context.Background(), "origin")
	if err != nil || !snap.Unborn() {
		t.Fatalf("unborn fetch: %+v, %v", snap, err)
	}
	if got := strings.TrimSpace(e.git(unborn, "rev-parse", "origin/main")); got != theirs {
		t.Fatalf("unborn origin/main = %s", got)
	}
}

func TestFetchRemoteChoice(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	repo, _ := e.open(dir)
	_, err := repo.Fetch(context.Background(), "")
	wantCode(t, err, CodeNoRemote)

	e.git(dir, "remote", "add", "upstream", dir)
	e.git(dir, "remote", "add", "fork", dir)
	info, _ := repo.Remotes(context.Background())
	if len(info.Remotes) != 2 || info.DefaultFetch() != "" {
		t.Fatalf("two remotes without origin: %+v", info)
	}
	_, err = repo.Fetch(context.Background(), "")
	wantCode(t, err, CodeChooseRemote)
	_, err = repo.Fetch(context.Background(), "--upload-pack=touch /tmp/x")
	wantCode(t, err, CodeRemoteNotFound)
	if _, err := repo.Fetch(context.Background(), "fork"); err != nil {
		t.Fatal(err)
	}
	e.git(dir, "remote", "add", "origin", dir)
	if info, _ := repo.Remotes(context.Background()); info.DefaultFetch() != "origin" {
		t.Fatalf("origin not preferred: %+v", info)
	}
}

func TestRemoteURLsAreRedacted(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	e.git(dir, "remote", "add", "origin", "https://user:s3cret@example.invalid/r.git")
	repo, _ := e.open(dir)
	info, err := repo.Remotes(context.Background())
	if err != nil || len(info.Remotes) != 1 || strings.Contains(info.Remotes[0].URL, "s3cret") || strings.Contains(info.Remotes[0].URL, "user") {
		t.Fatalf("remotes = %+v, %v", info, err)
	}
}

// fakeSSH puts an ssh on PATH that records its arguments and runs body.
func fakeSSH(t *testing.T, e *env, body string) (argsFile string) {
	t.Helper()
	bin := t.TempDir()
	argsFile = filepath.Join(bin, "args")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	e.Runner.Env = append(e.Runner.Env, "PATH="+bin+":"+os.Getenv("PATH"))
	return argsFile
}

func sshRepo(t *testing.T, e *env) *Repository {
	t.Helper()
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	e.git(dir, "remote", "add", "origin", "ssh://git@example.invalid/r.git")
	repo, _ := e.open(dir)
	return repo
}

func TestFetchSSHFailures(t *testing.T) {
	for _, tc := range []struct {
		name, stderr string
		code         Code
	}{
		{"host key", "Host key verification failed.", CodeHostKey},
		{"auth", "git@example.invalid: Permission denied (publickey).", CodeAuthFailed},
		{"network", "ssh: Could not resolve hostname example.invalid: nodename nor servname provided", CodeNetwork},
		{"missing", "ERROR: Repository not found.", CodeRemoteRepositoryMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			args := fakeSSH(t, e, "echo '"+tc.stderr+"' >&2\nexit 255")
			repo := sshRepo(t, e)
			snap, err := repo.Fetch(context.Background(), "origin")
			wantCode(t, err, tc.code)
			if snap.Root == "" {
				t.Fatal("no state read after a failed fetch")
			}
			if got := readFile(t, args); !strings.Contains(got, "BatchMode=yes") {
				t.Fatalf("ssh ran without BatchMode: %q", got)
			}
		})
	}
}

func TestFetchKeepsUsersSSHCommand(t *testing.T) {
	e := newEnv(t)
	args := fakeSSH(t, e, "echo 'Host key verification failed.' >&2\nexit 255")
	repo := sshRepo(t, e)
	e.git(repo.Root(), "config", "core.sshCommand", "ssh -i /dev/null")
	_, err := repo.Fetch(context.Background(), "origin")
	wantCode(t, err, CodeHostKey)
	if got := readFile(t, args); strings.Contains(got, "BatchMode") || !strings.Contains(got, "-i /dev/null") {
		t.Fatalf("user's ssh command replaced: %q", got)
	}
}

func TestFetchTimeoutAndCancel(t *testing.T) {
	e := newEnv(t)
	fakeSSH(t, e, "sleep 30")
	repo := sshRepo(t, e)
	e.app.SetNetworkTimeout(300 * time.Millisecond)
	start := time.Now()
	snap, err := repo.Fetch(context.Background(), "origin")
	wantCode(t, err, CodeTimedOut)
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout took %v: ssh not stopped", time.Since(start))
	}
	if snap.Root == "" {
		t.Fatal("no state read after a timeout")
	}

	e.app.SetNetworkTimeout(0)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start = time.Now()
	snap, err = repo.Fetch(ctx, "origin")
	wantCode(t, err, CodeCancelled)
	if time.Since(start) > 5*time.Second || snap.Root == "" {
		t.Fatalf("cancel took %v, state %+v", time.Since(start), snap)
	}
	if info, _ := repo.Remotes(context.Background()); !info.FetchedAt.IsZero() {
		t.Fatal("failed fetch recorded as successful")
	}
}

func TestFetchHTTPAuthRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()
	e := newEnv(t)
	dir := e.repo()
	url := strings.Replace(srv.URL, "http://", "http://alice:t0ken@", 1) + "/r.git"
	e.git(dir, "remote", "add", "origin", url)
	repo, _ := e.open(dir)
	_, err := repo.Fetch(context.Background(), "origin")
	wantCode(t, err, CodeAuthFailed)
	if strings.Contains(err.Error(), "t0ken") {
		t.Fatalf("credential in error: %v", err)
	}
}

func TestFetchClosedRepository(t *testing.T) {
	e := newEnv(t)
	rs := cloned(t, e)
	old, _ := e.open(rs.mine)
	e.open(rs.theirs)
	_, err := old.Fetch(context.Background(), "origin")
	wantCode(t, err, CodeRepositoryClosed)
}

func TestAddRemoteAppearsInRemotes(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	fork := filepath.Join(canonical(t.TempDir()), "fork.git")
	e.git(dir, "clone", "-q", "--bare", dir, fork)
	repo, _ := e.open(dir)
	info, err := repo.AddRemote(context.Background(), "fork", fork)
	if err != nil || !info.Has("fork") {
		t.Fatalf("add remote: %+v, %v", info, err)
	}
	got, err := repo.Remotes(context.Background())
	if err != nil || !got.Has("fork") {
		t.Fatalf("remotes after add: %+v, %v", got, err)
	}
	var url string
	for _, rem := range got.Remotes {
		if rem.Name == "fork" {
			url = rem.URL
		}
	}
	if url == "" {
		t.Fatal("fork remote has no URL")
	}
}

func TestAddRemoteRedactsCredentials(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	repo, _ := e.open(dir)
	info, err := repo.AddRemote(context.Background(), "origin", "https://user:s3cret@example.invalid/r.git")
	if err != nil || len(info.Remotes) != 1 {
		t.Fatalf("add remote: %+v, %v", info, err)
	}
	if strings.Contains(info.Remotes[0].URL, "s3cret") || strings.Contains(info.Remotes[0].URL, "user") {
		t.Fatalf("credential in remotes: %+v", info)
	}
	stored := e.config(dir, "^remote\\.origin\\.url$")
	if !strings.Contains(stored, "s3cret") {
		t.Fatalf("URL not stored in git config: %q", stored)
	}
}

func TestAddRemoteRejectsDuplicateAndBadName(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	repo, _ := e.open(dir)
	if _, err := repo.AddRemote(context.Background(), "origin", dir); err != nil {
		t.Fatal(err)
	}
	_, err := repo.AddRemote(context.Background(), "origin", dir)
	wantCode(t, err, CodeRemoteExists)
	_, err = repo.AddRemote(context.Background(), "bad..name", dir)
	wantCode(t, err, CodeInvalidRemoteName)
	_, err = repo.AddRemote(context.Background(), "ok", "")
	wantCode(t, err, CodeInvalidRemoteURL)
}
