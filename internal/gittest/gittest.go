// Package gittest creates isolated Git repositories for tests. Git runs with
// a temporary HOME and global configuration, never the developer's.
package gittest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitowl/internal/git"
)

// Env is an isolated Git configuration for one test.
type Env struct {
	T      testing.TB
	Home   string
	Config string
	Runner *git.Runner
}

// New returns an isolated environment, skipping the test without Git.
func New(t testing.TB) *Env {
	t.Helper()
	r, err := git.Find()
	if err != nil {
		t.Skip("git not available")
	}
	home := t.TempDir()
	cfg := filepath.Join(home, "gitconfig")
	Write(t, cfg, "[user]\n\tname = GitOwl Test\n\temail = test@gitowl.invalid\n[init]\n\tdefaultBranch = main\n[commit]\n\tgpgsign = false\n[core]\n\tautocrlf = false\n")
	r.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_CONFIG_GLOBAL=" + cfg, "GIT_CONFIG_NOSYSTEM=1"}
	return &Env{T: t, Home: home, Config: cfg, Runner: r}
}

// Repo creates an empty repository and returns its canonical root.
func (e *Env) Repo() string {
	e.T.Helper()
	dir := e.T.TempDir()
	e.Git(dir, "init", "-q")
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		e.T.Fatal(err)
	}
	return real
}

// Git runs a Git command and fails the test on error.
func (e *Env) Git(dir string, args ...string) string {
	e.T.Helper()
	out, err := e.Runner.Run(context.Background(), dir, nil, args...)
	if err != nil {
		e.T.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

// Commit stages everything and commits it.
func (e *Env) Commit(dir, message string) {
	e.T.Helper()
	e.Git(dir, "add", "-A")
	e.Git(dir, "commit", "-q", "-m", message)
}

// Write writes a file, creating its directories.
func Write(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
