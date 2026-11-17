package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenameBranchDropsOldName(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	r, _ := e.open(dir)
	ctx := context.Background()
	if _, _, err := r.CreateBranch(ctx, "topic", false); err != nil {
		t.Fatal(err)
	}

	snap, b, err := r.RenameBranch(ctx, "topic", "renamed")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.local("topic"); ok {
		t.Fatalf("old name still listed: %s", names(b))
	}
	if _, ok := b.local("renamed"); !ok {
		t.Fatalf("new name missing: %s", names(b))
	}
	if snap.Branch != "main" {
		t.Fatalf("HEAD moved by rename: %+v", snap)
	}

	snap, b, err = r.RenameBranch(ctx, "main", "trunk")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "trunk" {
		t.Fatalf("current branch not renamed: %+v", snap)
	}
	if _, ok := b.local("main"); ok {
		t.Fatalf("old current name still listed: %s", names(b))
	}
}

func TestTagsListsNames(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	e.git(dir, "tag", "v1")
	e.git(dir, "tag", "release/beta")
	r, _ := e.open(dir)

	tags, err := r.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(tags))
	for i, tag := range tags {
		got[i] = tag.Name
	}
	if strings.Join(got, " ") != "release/beta v1" && strings.Join(got, " ") != "v1 release/beta" {
		t.Fatalf("tags %q", got)
	}
}

func TestCreateTagIsLightweight(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Initial")
	head := strings.TrimSpace(e.git(dir, "rev-parse", "HEAD"))
	r, _ := e.open(dir)

	snap, err := r.CreateTag(context.Background(), "v1", head)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Head != head {
		t.Fatalf("HEAD moved by tag: %+v", snap)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "refs/tags/v1")); got != head {
		t.Fatalf("tag points at %s, want %s", got, head)
	}
	if kind := strings.TrimSpace(e.git(dir, "for-each-ref", "--format=%(objecttype)", "refs/tags/v1")); kind != "commit" {
		t.Fatalf("tag is %q, want a lightweight tag (commit)", kind)
	}

	if _, err := r.CreateTag(context.Background(), "from-branch", "main"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(e.git(dir, "rev-parse", "refs/tags/from-branch")); got != head {
		t.Fatalf("branch target %s, want %s", got, head)
	}
}
