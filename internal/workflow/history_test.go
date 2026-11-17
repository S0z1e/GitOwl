package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryListsCommitsNewestFirst(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	r, _ := e.open(dir)
	ctx := context.Background()
	commits, more, err := r.History(ctx, 0, 50)
	if err != nil || len(commits) != 0 || more {
		t.Fatalf("empty history %v %v %v", commits, more, err)
	}
	for i, s := range []string{"First", "Second", "Third"} {
		write(t, filepath.Join(dir, "f.txt"), strings.Repeat("x\n", i+1))
		e.commit(dir, s)
	}
	commits, more, err = r.History(ctx, 0, 2)
	if err != nil || !more || len(commits) != 2 {
		t.Fatalf("page %v %v %v", commits, more, err)
	}
	if commits[0].Subject != "Third" || commits[1].Subject != "Second" {
		t.Fatalf("order %+v", commits)
	}
	c := commits[0]
	if c.Author != "GitOwl Test" || c.Email != "test@gitowl.invalid" || c.When.IsZero() || len(c.Hash) < 40 || len(c.Short()) != 7 {
		t.Fatalf("fields %+v", c)
	}
	rest, more, err := r.History(ctx, 2, 2)
	if err != nil || more || len(rest) != 1 || rest[0].Subject != "First" {
		t.Fatalf("second page %v %v %v", rest, more, err)
	}
}

func TestDetailComparesWithTheRightBase(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "a.txt"), "a\n")
	e.commit(dir, "Root\n\nWith a body.")
	write(t, filepath.Join(dir, "a.txt"), "a\nmain\n")
	e.commit(dir, "On main")
	e.git(dir, "switch", "-q", "-c", "topic", "HEAD~1")
	write(t, filepath.Join(dir, "b.txt"), "topic\n")
	e.commit(dir, "On topic")
	e.git(dir, "switch", "-q", "main")
	e.git(dir, "merge", "-q", "--no-ff", "-m", "Merge topic", "topic")
	r, _ := e.open(dir)
	ctx := context.Background()
	commits, _, err := r.History(ctx, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	bySubject := map[string]CommitInfo{}
	for _, c := range commits {
		bySubject[c.Subject] = c
	}

	root, err := r.Detail(ctx, bySubject["Root"])
	if err != nil {
		t.Fatal(err)
	}
	if root.Base != BaseEmptyTree || root.Message != "Root\n\nWith a body." || join(paths(root.Files)) != "a.txt" || root.Files[0].Kind != 1 {
		t.Fatalf("root %+v", root)
	}
	if d, err := r.CommitFileDiff(ctx, root, root.Files[0]); err != nil || !strings.Contains(d.Patch, "+a\n") || !strings.Contains(d.Patch, "new file mode") {
		t.Fatalf("root diff %v\n%s", err, d.Patch)
	}

	normal, _ := r.Detail(ctx, bySubject["On main"])
	if normal.Base != BaseParent || normal.BaseHash != bySubject["Root"].Hash {
		t.Fatalf("normal base %+v", normal)
	}
	if d, _ := r.CommitFileDiff(ctx, normal, normal.Files[0]); !strings.Contains(d.Patch, "+main\n") || strings.Contains(d.Patch, "+a\n") {
		t.Fatalf("normal diff\n%s", d.Patch)
	}

	merge, _ := r.Detail(ctx, bySubject["Merge topic"])
	if merge.Base != BaseFirstParent || merge.BaseHash != bySubject["On main"].Hash {
		t.Fatalf("merge base %+v", merge)
	}
	// Against the first parent the merge brings in topic's file only.
	if join(paths(merge.Files)) != "b.txt" {
		t.Fatalf("merge files %v", paths(merge.Files))
	}
}

func TestDetailOfRenameAndBinary(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "old name.txt"), "1\n2\n3\n4\n")
	write(t, filepath.Join(dir, "pic.bin"), "\x00\x01")
	e.commit(dir, "Initial")
	e.git(dir, "mv", "old name.txt", "new name.txt")
	write(t, filepath.Join(dir, "pic.bin"), "\x00\x02")
	e.commit(dir, "Rename and change binary")
	r, _ := e.open(dir)
	ctx := context.Background()
	commits, _, _ := r.History(ctx, 0, 1)
	d, err := r.Detail(ctx, commits[0])
	if err != nil {
		t.Fatal(err)
	}
	ren := find(t, d.Files, "new name.txt")
	if ren.OrigPath != "old name.txt" {
		t.Fatalf("rename %+v", ren)
	}
	if diff, _ := r.CommitFileDiff(ctx, d, ren); !strings.Contains(diff.Patch, "rename from old name.txt") {
		t.Fatalf("rename diff\n%s", diff.Patch)
	}
	if diff, _ := r.CommitFileDiff(ctx, d, find(t, d.Files, "pic.bin")); !diff.Binary {
		t.Fatalf("binary %+v", diff)
	}
}
