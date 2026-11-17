package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareRefsListsDifferingPaths(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "shared.txt"), "same\n")
	write(t, filepath.Join(dir, "a.txt"), "main\n")
	e.commit(dir, "On main")
	e.git(dir, "switch", "-q", "-c", "feature")
	write(t, filepath.Join(dir, "a.txt"), "feature\n")
	write(t, filepath.Join(dir, "b.txt"), "added\n")
	e.commit(dir, "On feature")
	e.git(dir, "switch", "-q", "main")
	r, _ := e.open(dir)

	cmp, err := r.CompareRefs(context.Background(), "main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if join(paths(cmp.Files)) != "a.txt,b.txt" {
		t.Fatalf("paths %s", join(paths(cmp.Files)))
	}
	d, err := r.CompareFileDiff(context.Background(), cmp, cmp.Files[0])
	if err != nil || !strings.Contains(d.Patch, "-main\n") || !strings.Contains(d.Patch, "+feature\n") {
		t.Fatalf("file diff %v\n%s", err, d.Patch)
	}
}

func TestCompareRefsToWorkingDir(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	e.commit(dir, "First")
	write(t, filepath.Join(dir, "a.txt"), "two\n")
	r, snap := e.open(dir)

	cmp, err := r.CompareRefs(context.Background(), snap.Head, "")
	if err != nil {
		t.Fatal(err)
	}
	if join(paths(cmp.Files)) != "a.txt" {
		t.Fatalf("paths %s", join(paths(cmp.Files)))
	}
}
