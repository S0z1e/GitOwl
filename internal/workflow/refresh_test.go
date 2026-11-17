package workflow

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRefreshShowsExternalChanges(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	r, snap := e.open(dir)
	if len(snap.Untracked) != 0 {
		t.Fatal(snap.Untracked)
	}
	write(t, filepath.Join(dir, "outside.txt"), "made outside GitOwl\n")
	snap, err := r.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := join(paths(snap.Untracked)); got != "outside.txt" {
		t.Fatalf("untracked after refresh: %s", got)
	}
}

func TestSwitchingRepositoriesClosesThePrevious(t *testing.T) {
	e := newEnv(t)
	a, b := e.repo(), e.repo()
	write(t, filepath.Join(b, "only-in-b.txt"), "b\n")
	ra, _ := e.open(a)
	rb, snap := e.open(b)
	if ra.Current() || !rb.Current() || e.app.Current() != rb {
		t.Fatal("current repository did not follow the latest open")
	}
	if snap.Root != b || join(paths(snap.Untracked)) != "only-in-b.txt" {
		t.Fatalf("snapshot of b: %+v", snap)
	}
	// Reopening the same path gives a new handle; the old one is closed.
	rb2, _ := e.open(b)
	if rb.Current() || !rb2.Current() {
		t.Fatal("reopening did not replace the handle")
	}
}
