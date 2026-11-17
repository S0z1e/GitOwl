package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func find(t *testing.T, cs []Change, path string) Change {
	t.Helper()
	for _, c := range cs {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("%s not in %v", path, paths(cs))
	return Change{}
}

func fileDiff(t *testing.T, r *Repository, c Change) Diff {
	t.Helper()
	d, err := r.FileDiff(context.Background(), c)
	if err != nil {
		t.Fatalf("diff %s: %v", c.Path, err)
	}
	return d
}

func TestDiffSeparatesStagedAndUnstagedParts(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "mixed.txt"), "one\ntwo\nthree\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "mixed.txt"), "one\nTWO staged\nthree\n")
	e.git(dir, "add", "mixed.txt")
	write(t, filepath.Join(dir, "mixed.txt"), "one\nTWO staged\nthree\nfour unstaged\n")
	r, snap := e.open(dir)

	staged := fileDiff(t, r, find(t, snap.Staged, "mixed.txt"))
	if !staged.HasText() || !strings.Contains(staged.Patch, "-two\n+TWO staged\n") || strings.Contains(staged.Patch, "four unstaged") {
		t.Fatalf("staged patch:\n%s", staged.Patch)
	}
	unstaged := fileDiff(t, r, find(t, snap.Unstaged, "mixed.txt"))
	if !strings.Contains(unstaged.Patch, "+four unstaged\n") || strings.Contains(unstaged.Patch, "-two") {
		t.Fatalf("unstaged patch:\n%s", unstaged.Patch)
	}
	// The staged diff is what the index holds.
	if got := e.git(dir, "show", ":mixed.txt"); got != "one\nTWO staged\nthree\n" {
		t.Fatalf("index content %q", got)
	}
}

func TestDiffAddedDeletedUntrackedAndUnusualNames(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "gone.txt"), "bye\n")
	write(t, filepath.Join(dir, "*.txt"), "a literal star\n")
	write(t, filepath.Join(dir, "match.txt"), "matched by a glob\n")
	e.commit(dir, "Initial")
	os.Remove(filepath.Join(dir, "gone.txt"))
	write(t, filepath.Join(dir, "*.txt"), "a literal star, changed\n")
	write(t, filepath.Join(dir, "match.txt"), "changed too\n")
	write(t, filepath.Join(dir, "新 文件.txt"), "你好\n")
	write(t, filepath.Join(dir, "staged new.txt"), "fresh\n")
	e.git(dir, "add", "staged new.txt")
	r, snap := e.open(dir)

	deleted := fileDiff(t, r, find(t, snap.Unstaged, "gone.txt"))
	if !strings.Contains(deleted.Patch, "deleted file mode") || !strings.Contains(deleted.Patch, "-bye\n") {
		t.Fatalf("deleted:\n%s", deleted.Patch)
	}
	added := fileDiff(t, r, find(t, snap.Staged, "staged new.txt"))
	if !strings.Contains(added.Patch, "new file mode") || !strings.Contains(added.Patch, "+fresh\n") {
		t.Fatalf("added:\n%s", added.Patch)
	}
	untracked := fileDiff(t, r, find(t, snap.Untracked, "新 文件.txt"))
	if !untracked.HasText() || !strings.Contains(untracked.Patch, "+你好\n") {
		t.Fatalf("untracked:\n%s", untracked.Patch)
	}
	// A path with glob characters names only itself.
	star := fileDiff(t, r, find(t, snap.Unstaged, "*.txt"))
	if strings.Contains(star.Patch, "match.txt") || !strings.Contains(star.Patch, "+a literal star, changed") {
		t.Fatalf("literal path:\n%s", star.Patch)
	}
}

func TestDiffExplainsContentWithoutText(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "image.bin"), "\x00\x01\x02binary")
	write(t, filepath.Join(dir, "script.sh"), "echo\n")
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "image.bin"), "\x00\x01\x03binary changed")
	os.Chmod(filepath.Join(dir, "script.sh"), 0o755)
	write(t, filepath.Join(dir, "new.bin"), "\x00new")
	write(t, filepath.Join(dir, "empty.txt"), "")
	r, snap := e.open(dir)

	if d := fileDiff(t, r, find(t, snap.Unstaged, "image.bin")); !d.Binary || d.HasText() {
		t.Fatalf("modified binary %+v", d)
	}
	if d := fileDiff(t, r, find(t, snap.Untracked, "new.bin")); !d.Binary {
		t.Fatalf("untracked binary %+v", d)
	}
	if d := fileDiff(t, r, find(t, snap.Unstaged, "script.sh")); !d.NoTextChange || !strings.Contains(d.Patch, "new mode 100755") {
		t.Fatalf("mode change %+v", d)
	}
	if d := fileDiff(t, r, find(t, snap.Untracked, "empty.txt")); !d.NoTextChange {
		t.Fatalf("empty file %+v", d)
	}
}

func TestDiffInRepositoryWithoutCommits(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "first.txt"), "first line\n")
	e.git(dir, "add", "first.txt")
	write(t, filepath.Join(dir, "first.txt"), "first line\nsecond line\n")
	r, snap := e.open(dir)
	staged := fileDiff(t, r, find(t, snap.Staged, "first.txt"))
	if !strings.Contains(staged.Patch, "new file mode") || strings.Contains(staged.Patch, "second line") {
		t.Fatalf("staged in unborn repository:\n%s", staged.Patch)
	}
	unstaged := fileDiff(t, r, find(t, snap.Unstaged, "first.txt"))
	if !strings.Contains(unstaged.Patch, "+second line") {
		t.Fatalf("unstaged in unborn repository:\n%s", unstaged.Patch)
	}
}

func TestDiffStagedRename(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "before.txt"), "a\nb\nc\nd\ne\n")
	e.commit(dir, "Initial")
	e.git(dir, "mv", "before.txt", "after.txt")
	r, snap := e.open(dir)
	d := fileDiff(t, r, find(t, snap.Staged, "after.txt"))
	if !strings.Contains(d.Patch, "rename from before.txt") || !strings.Contains(d.Patch, "rename to after.txt") {
		t.Fatalf("rename:\n%s", d.Patch)
	}
}
