package workflow

import (
	"context"
	"path/filepath"
	"testing"
)

// numstat reads the change list's line counts for the open repository, from
// a fresh status snapshot, and fails the test on error.
func (e *env) numstat(r *Repository, snap Snapshot) Numstat {
	e.t.Helper()
	ns, err := r.Numstat(context.Background(), snap)
	if err != nil {
		e.t.Fatalf("numstat: %v", err)
	}
	return ns
}

// find returns the file stat for path in area, or fails the test.
func (ns Numstat) find(t *testing.T, area Area, path string) FileStat {
	t.Helper()
	for _, f := range ns.Files {
		if f.Area == area && f.Path == path {
			return f
		}
	}
	t.Fatalf("no %v stat for %q in %+v", area, path, ns.Files)
	return FileStat{}
}

// TestNumstatModify counts added and removed lines of a staged edit.
func TestNumstatModify(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\nthree\n")
	e.commit(repo, "add a")
	write(t, filepath.Join(repo, "a.txt"), "one\nTWO\nthree\nfour\n")
	e.git(repo, "add", "a.txt")
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	f := ns.find(t, Staged, "a.txt")
	if f.Added != 2 || f.Removed != 1 || f.Binary {
		t.Fatalf("staged a.txt = %+v, want +2 -1", f)
	}
	if ns.StagedAdded != 2 || ns.StagedRemoved != 1 {
		t.Fatalf("staged totals = +%d -%d, want +2 -1", ns.StagedAdded, ns.StagedRemoved)
	}
	if ns.Basis == "" {
		t.Fatal("empty basis")
	}
}

// TestNumstatTwoAreas counts a file changed in both the index and the
// working tree separately per area.
func TestNumstatTwoAreas(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\n")
	e.commit(repo, "add a")
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\nstaged\n")
	e.git(repo, "add", "a.txt")
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\nstaged\nworktree\n")
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	if s := ns.find(t, Staged, "a.txt"); s.Added != 1 || s.Removed != 0 {
		t.Fatalf("staged = %+v, want +1 -0", s)
	}
	if u := ns.find(t, Unstaged, "a.txt"); u.Added != 1 || u.Removed != 0 {
		t.Fatalf("unstaged = %+v, want +1 -0", u)
	}
}

// TestNumstatRename reports a staged rename under its new path.
func TestNumstatRename(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	write(t, filepath.Join(repo, "old.txt"), "one\ntwo\nthree\n")
	e.commit(repo, "add old")
	e.git(repo, "mv", "old.txt", "new.txt")
	write(t, filepath.Join(repo, "new.txt"), "one\ntwo\nthree\nfour\n")
	e.git(repo, "add", "new.txt")
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	f := ns.find(t, Staged, "new.txt")
	if f.Added != 1 || f.Removed != 0 || f.OrigPath != "old.txt" {
		t.Fatalf("rename = %+v, want +1 -0 from old.txt", f)
	}
}

// TestNumstatBinary marks a binary change as Binary, not zero lines.
func TestNumstatBinary(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	write(t, filepath.Join(repo, "b.bin"), "\x00\x01\x02")
	e.git(repo, "add", "b.bin")
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	f := ns.find(t, Staged, "b.bin")
	if !f.Binary || f.Added != 0 || f.Removed != 0 {
		t.Fatalf("binary = %+v, want Binary with 0 lines", f)
	}
}

// TestNumstatUntracked counts an untracked text file as all-added lines.
func TestNumstatUntracked(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	write(t, filepath.Join(repo, "seed.txt"), "seed\n")
	e.commit(repo, "seed")
	write(t, filepath.Join(repo, "u.txt"), "alpha\nbeta\ngamma\n")
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	f := ns.find(t, Untracked, "u.txt")
	if f.Added != 3 || f.Removed != 0 || f.Binary {
		t.Fatalf("untracked = %+v, want +3 -0", f)
	}
}

// TestNumstatEmptyRepo returns no files and no totals for a clean, unborn
// repository.
func TestNumstatEmptyRepo(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	if len(ns.Files) != 0 || ns.StagedAdded != 0 || ns.StagedRemoved != 0 {
		t.Fatalf("empty repo numstat = %+v, want no files", ns)
	}
}

// TestNumstatStaleBasis reports a result as stale once the staging area
// behind its basis has moved.
func TestNumstatStaleBasis(t *testing.T) {
	e := newEnv(t)
	repo := e.repo()
	write(t, filepath.Join(repo, "a.txt"), "one\n")
	e.commit(repo, "add a")
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\n")
	e.git(repo, "add", "a.txt")
	r, snap := e.open(repo)

	ns := e.numstat(r, snap)
	if ns.Stale(snap) {
		t.Fatal("fresh result reported stale")
	}
	// Staging more changes the basis the counts were read from.
	write(t, filepath.Join(repo, "a.txt"), "one\ntwo\nthree\n")
	e.git(repo, "add", "a.txt")
	later, err := r.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !ns.Stale(later) {
		t.Fatal("result not reported stale after the index moved")
	}
}
