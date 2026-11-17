package workflow

import (
	"context"
	"strings"
	"testing"

	"gitowl/internal/gittest"
)

type env struct {
	*gittest.Env
	t   *testing.T
	app *Application
}

func newEnv(t *testing.T) *env {
	t.Helper()
	g := gittest.New(t)
	return &env{Env: g, t: t, app: New(g.Runner)}
}

func (e *env) repo() string                          { e.t.Helper(); return e.Repo() }
func (e *env) git(dir string, args ...string) string { e.t.Helper(); return e.Git(dir, args...) }
func (e *env) commit(dir, message string)            { e.t.Helper(); e.Commit(dir, message) }

func (e *env) open(path string) (*Repository, Snapshot) {
	e.t.Helper()
	r, snap, err := e.app.Open(context.Background(), path)
	if err != nil {
		e.t.Fatalf("open %s: %v", path, err)
	}
	return r, snap
}

func write(t *testing.T, path, content string) { t.Helper(); gittest.Write(t, path, content) }

func paths(cs []Change) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Path)
	}
	return out
}

func wantCode(t *testing.T, err error, code Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got success", code)
	}
	if got := CodeOf(err); got != code {
		t.Fatalf("want %s, got %s (%v)", code, got, err)
	}
}

func join(s []string) string { return strings.Join(s, ",") }
