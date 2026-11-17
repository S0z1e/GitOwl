package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lines numbers lines 1..n, with line k replaced by edits[k].
func lines(n int, edits map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if s, ok := edits[i]; ok {
			b.WriteString(s)
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// indexBlob returns the staged content of path.
func (e *env) indexBlob(dir, path string) string {
	e.t.Helper()
	return e.git(dir, "show", ":"+path)
}

func diffOf(t *testing.T, repo *Repository, snap Snapshot, area Area, path string) Diff {
	t.Helper()
	for _, ch := range append(append([]Change{}, snap.Staged...), snap.Unstaged...) {
		if ch.Area == area && ch.Path == path {
			d, err := repo.FileDiff(context.Background(), ch)
			if err != nil {
				t.Fatal(err)
			}
			return d
		}
	}
	t.Fatalf("%s not in area %d of %+v", path, area, snap)
	return Diff{}
}

func TestStageAndUnstageOneHunk(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), lines(30, nil))
	write(t, filepath.Join(dir, "other.txt"), "o\n")
	e.commit(dir, "Initial")
	edited := lines(30, map[int]string{2: "two\n", 25: "twenty-five\n"})
	write(t, filepath.Join(dir, "f.txt"), edited)
	write(t, filepath.Join(dir, "other.txt"), "o2\n")
	repo, snap := e.open(dir)
	d := diffOf(t, repo, snap, Unstaged, "f.txt")
	if d.NoHunks != "" || d.HunkCount() != 2 {
		t.Fatalf("hunks %d, %q", d.HunkCount(), d.NoHunks)
	}
	snap, err := repo.StageHunk(context.Background(), d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.indexBlob(dir, "f.txt"); got != lines(30, map[int]string{25: "twenty-five\n"}) {
		t.Fatalf("index:\n%s", got)
	}
	if readFile(t, filepath.Join(dir, "f.txt")) != edited || e.indexBlob(dir, "other.txt") != "o\n" {
		t.Fatal("staging a hunk changed a file or another index entry")
	}
	// The file is now partly staged: one hunk on each side.
	staged, unstaged := diffOf(t, repo, snap, Staged, "f.txt"), diffOf(t, repo, snap, Unstaged, "f.txt")
	if staged.HunkCount() != 1 || unstaged.HunkCount() != 1 || !strings.Contains(unstaged.Patch, "+two") {
		t.Fatalf("after: staged %q unstaged %q", staged.Patch, unstaged.Patch)
	}
	snap, err = repo.UnstageHunk(context.Background(), staged, 0)
	if err != nil {
		t.Fatal(err)
	}
	if e.indexBlob(dir, "f.txt") != lines(30, nil) || len(snap.Staged) != 0 {
		t.Fatalf("unstage left %q", e.indexBlob(dir, "f.txt"))
	}
	if readFile(t, filepath.Join(dir, "f.txt")) != edited {
		t.Fatal("unstaging changed the file")
	}
}

func TestHunksInOneSection(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), lines(12, nil))
	e.commit(dir, "Initial")
	// Changes two lines apart share one @@ section: three hunks there.
	edited := lines(12, map[int]string{3: "", 5: "five\nextra\n", 7: "seven\n"})
	write(t, filepath.Join(dir, "f.txt"), edited)
	repo, snap := e.open(dir)
	d := diffOf(t, repo, snap, Unstaged, "f.txt")
	if strings.Count(d.Patch, "\n@@ ") != 1 || d.HunkCount() != 3 {
		t.Fatalf("patch:\n%s", d.Patch)
	}
	snap, err := repo.StageHunk(context.Background(), d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.indexBlob(dir, "f.txt"); got != lines(12, map[int]string{5: "five\nextra\n"}) {
		t.Fatalf("index:\n%s", got)
	}
	// Unstage it again from the staged diff, then stage the deletion.
	staged := diffOf(t, repo, snap, Staged, "f.txt")
	snap, err = repo.UnstageHunk(context.Background(), staged, 0)
	if err != nil || len(snap.Staged) != 0 {
		t.Fatalf("unstage: %v %+v", err, snap.Staged)
	}
	d = diffOf(t, repo, snap, Unstaged, "f.txt")
	if _, err := repo.StageHunk(context.Background(), d, 0); err != nil {
		t.Fatal(err)
	}
	if got := e.indexBlob(dir, "f.txt"); got != lines(12, map[int]string{3: ""}) {
		t.Fatalf("index:\n%s", got)
	}
	// Staged and unstaged parts are separate diffs, each with its own hunks.
	snap, _ = repo.Status(context.Background())
	staged, unstaged := diffOf(t, repo, snap, Staged, "f.txt"), diffOf(t, repo, snap, Unstaged, "f.txt")
	if staged.HunkCount() != 1 || unstaged.HunkCount() != 2 {
		t.Fatalf("staged %d unstaged %d", staged.HunkCount(), unstaged.HunkCount())
	}
	if readFile(t, filepath.Join(dir, "f.txt")) != edited {
		t.Fatal("file changed")
	}
}

func TestHunkBytesKept(t *testing.T) {
	long := strings.Repeat("x", 20000)
	for _, tc := range []struct {
		name, path, before, after, want string
		hunk                            int
	}{
		{"crlf", "crlf.txt", "a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng\r\nh\r\ni\r\nj\r\n", "A\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng\r\nh\r\ni\r\nJ\r\n", "a\r\nb\r\nc\r\nd\r\ne\r\nf\r\ng\r\nh\r\ni\r\nJ\r\n", 1},
		{"no newline", "end.txt", lines(10, nil) + "last", lines(10, map[int]string{1: "first\n"}) + "LAST", lines(10, nil) + "LAST", 1},
		{"long line", "long.txt", lines(10, nil), lines(10, map[int]string{1: long + "\n", 10: "ten\n"}), lines(10, map[int]string{1: long + "\n"}), 0},
		{"whitespace", "ws.txt", lines(10, nil), lines(10, map[int]string{1: "line 1  \n", 10: "\tten\n"}), lines(10, map[int]string{1: "line 1  \n"}), 0},
		{"special path", "dir with space/-x \"q\" 名.txt", lines(10, nil), lines(10, map[int]string{1: "one\n", 10: "ten\n"}), lines(10, map[int]string{10: "ten\n"}), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			dir := e.repo()
			write(t, filepath.Join(dir, tc.path), tc.before)
			e.commit(dir, "Initial")
			write(t, filepath.Join(dir, tc.path), tc.after)
			repo, snap := e.open(dir)
			d := diffOf(t, repo, snap, Unstaged, tc.path)
			if d.HunkCount() != 2 {
				t.Fatalf("hunks %d:\n%s", d.HunkCount(), d.Patch)
			}
			if _, err := repo.StageHunk(context.Background(), d, tc.hunk); err != nil {
				t.Fatal(err)
			}
			if got := e.indexBlob(dir, tc.path); got != tc.want {
				t.Fatalf("index %q, want %q", got, tc.want)
			}
			if readFile(t, filepath.Join(dir, tc.path)) != tc.after {
				t.Fatal("file changed")
			}
		})
	}
}

func TestHunkRefusedWhenStale(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), lines(30, nil))
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), lines(30, map[int]string{2: "two\n", 25: "x\n"}))
	repo, snap := e.open(dir)
	d := diffOf(t, repo, snap, Unstaged, "f.txt")
	// Edited in another tool after the diff was shown.
	write(t, filepath.Join(dir, "f.txt"), lines(30, map[int]string{1: "new\n", 2: "two\n", 25: "x\n"}))
	snap, err := repo.StageHunk(context.Background(), d, 0)
	wantCode(t, err, CodeHunkStale)
	if snap.Root == "" || len(snap.Staged) != 0 || e.indexBlob(dir, "f.txt") != lines(30, nil) {
		t.Fatal("stale hunk staged, or no state returned")
	}
	// Staged elsewhere meanwhile: also stale.
	d = diffOf(t, repo, snap, Unstaged, "f.txt")
	e.git(dir, "add", "f.txt")
	_, err = repo.StageHunk(context.Background(), d, 0)
	wantCode(t, err, CodeHunkStale)
	_, err = repo.StageHunk(context.Background(), d, 9)
	wantCode(t, err, CodeHunkStale)
}

func TestHunkApplyFailureChangesNothing(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), lines(30, nil))
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), lines(30, map[int]string{2: "two\n", 25: "x\n"}))
	repo, snap := e.open(dir)
	d := diffOf(t, repo, snap, Unstaged, "f.txt")
	if _, err := repo.StageHunk(context.Background(), d, 7); CodeOf(err) != CodeHunkNotFound {
		t.Fatalf("out of range: %v", err)
	}
	// A locked index: Git refuses and nothing changes.
	lock := filepath.Join(dir, ".git", "index.lock")
	write(t, lock, "")
	defer os.Remove(lock)
	snap, err := repo.StageHunk(context.Background(), d, 0)
	wantCode(t, err, CodeHunkFailed)
	os.Remove(lock)
	if e.indexBlob(dir, "f.txt") != lines(30, nil) || snap.Root == "" {
		t.Fatal("failed apply changed the index")
	}
}

func TestHunksUnsupported(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "mod.sh"), lines(5, nil))
	write(t, filepath.Join(dir, "gone.txt"), "g\n")
	write(t, filepath.Join(dir, "old.txt"), lines(20, nil))
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "added.txt"), "a\n")
	e.git(dir, "add", "added.txt")
	write(t, filepath.Join(dir, "untracked.txt"), "u\n")
	os.Remove(filepath.Join(dir, "gone.txt"))
	if err := os.Chmod(filepath.Join(dir, "mod.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "mod.sh"), lines(5, map[int]string{1: "x\n"}))
	e.git(dir, "mv", "old.txt", "new.txt")
	repo, snap := e.open(dir)
	want := map[string]Code{"added.txt": HunksAdded, "gone.txt": HunksDeleted, "mod.sh": HunksMode, "new.txt": HunksRenamed, "untracked.txt": HunksUntracked}
	for _, ch := range append(append(append([]Change{}, snap.Staged...), snap.Unstaged...), snap.Untracked...) {
		d, err := repo.FileDiff(context.Background(), ch)
		if err != nil {
			t.Fatal(err)
		}
		if d.NoHunks != want[ch.Path] {
			t.Errorf("%s (%d): NoHunks %q, want %q", ch.Path, ch.Area, d.NoHunks, want[ch.Path])
		}
		if d.NoHunks != "" {
			if _, err := repo.StageHunk(context.Background(), d, 0); err == nil {
				t.Errorf("%s: staged an unsupported hunk", ch.Path)
			}
		}
	}

	// Symbolic links are never staged by hunk.
	link := e.repo()
	if err := os.Symlink("a", filepath.Join(link, "l")); err != nil {
		t.Fatal(err)
	}
	e.commit(link, "Link")
	os.Remove(filepath.Join(link, "l"))
	os.Symlink("b", filepath.Join(link, "l"))
	repo, snap = e.open(link)
	if d := diffOf(t, repo, snap, Unstaged, "l"); d.NoHunks != HunksSymlink {
		t.Fatalf("symlink NoHunks %q", d.NoHunks)
	}
}

func TestCommitTakesOnlyStagedHunk(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), lines(30, nil))
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), lines(30, map[int]string{2: "two\n", 25: "x\n"}))
	repo, snap := e.open(dir)
	snap, err := repo.StageHunk(context.Background(), diffOf(t, repo, snap, Unstaged, "f.txt"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.Commit(context.Background(), "Two", snap.StagedFingerprint); err != nil {
		t.Fatal(err)
	}
	if got := e.git(dir, "show", "HEAD:f.txt"); got != lines(30, map[int]string{2: "two\n"}) {
		t.Fatalf("committed:\n%s", got)
	}
	if got := e.git(dir, "diff", "--name-only"); got != "f.txt\n" {
		t.Fatalf("unstaged after commit: %q", got)
	}
}

func TestDraftSeesOnlyStagedHunk(t *testing.T) {
	e := newEnv(t)
	dir := e.repo()
	write(t, filepath.Join(dir, "f.txt"), lines(30, nil))
	e.commit(dir, "Initial")
	write(t, filepath.Join(dir, "f.txt"), lines(30, map[int]string{2: "staged-two\n", 25: "unstaged-x\n"}))
	repo, snap := e.open(dir)
	if _, err := repo.StageHunk(context.Background(), diffOf(t, repo, snap, Unstaged, "f.txt"), 0); err != nil {
		t.Fatal(err)
	}
	req, err := repo.PrepareDraft(context.Background(), DraftOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.Diff, "staged-two") || strings.Contains(req.Diff, "unstaged-x") {
		t.Fatalf("draft diff:\n%s", req.Diff)
	}
}
